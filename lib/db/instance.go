// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package db

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/util"
)

type instance struct {
	*Lowlevel
	keyer keyer
}

func newInstance(ll *Lowlevel) *instance {
	return &instance{
		Lowlevel: ll,
		keyer:    newDefaultKeyer(ll.folderIdx, ll.deviceIdx),
	}
}

// updateRemoteFiles adds a list of fileinfos to the database and updates the
// global versionlist and metadata.
func (db *instance) updateRemoteFiles(folder, device []byte, fs []protocol.FileInfo, meta *metadataTracker) {
	rw := db.newReadWriteTransaction()
	defer rw.close()
	updateRemoteFiles(rw, db.keyer, folder, device, fs, meta)
}

func updateRemoteFiles(rw readWriter, k keyer, folder, device []byte, fs []protocol.FileInfo, meta *metadataTracker) {
	var dk, gk, keyBuf []byte
	devID := protocol.DeviceIDFromBytes(device)
	for _, f := range fs {
		name := []byte(f.Name)
		dk = k.GenerateDeviceFileKey(rw, dk, folder, device, name)

		ef, ok := getFileTrunc(rw, dk, true)
		if ok && unchanged(f, ef) {
			continue
		}

		if ok {
			meta.removeFile(devID, ef)
		}
		meta.addFile(devID, f)

		// updateGlobal depends on being able to access the previous version
		// of the file in the database, so we need to do this before we Put
		// the new file
		gk = k.GenerateGlobalVersionKey(rw, gk, folder, name)
		keyBuf, _ = updateGlobal(rw, k, gk, keyBuf, folder, device, f, meta)

		l.Debugf("insert; folder=%q device=%v %v", folder, devID, f)
		rw.Put(dk, mustMarshal(&f), nil)
	}
}

// updateLocalFiles adds fileinfos to the db, and updates the global versionlist,
// metadata, sequence and blockmap buckets.
func (db *instance) updateLocalFiles(folder []byte, fs []protocol.FileInfo, meta *metadataTracker) {
	rw := db.newReadWriteTransaction()
	defer rw.close()
	updateLocalFiles(rw, db.keyer, folder, fs, meta)
}

func updateLocalFiles(rw readWriter, k keyer, folder []byte, fs []protocol.FileInfo, meta *metadataTracker) {
	var dk, gk, keyBuf []byte
	blockBuf := make([]byte, 4)
	for _, f := range fs {
		name := []byte(f.Name)
		dk = k.GenerateDeviceFileKey(rw, dk, folder, protocol.LocalDeviceID[:], name)

		ef, ok := getFileByKey(rw, dk)
		if ok && unchanged(f, ef) {
			continue
		}

		if ok {
			if !ef.IsDirectory() && !ef.IsDeleted() && !ef.IsInvalid() {
				for _, block := range ef.Blocks {
					keyBuf = k.GenerateBlockMapKey(rw, keyBuf, folder, block.Hash, name)
					rw.Delete(keyBuf, nil)
				}
			}

			keyBuf = k.GenerateSequenceKey(rw, keyBuf, folder, ef.SequenceNo())
			rw.Delete(keyBuf, nil)
			l.Debugf("removing sequence; folder=%q sequence=%v %v", folder, ef.SequenceNo(), ef.FileName())
		}

		f.Sequence = meta.nextLocalSeq()

		if ok {
			meta.removeFile(protocol.LocalDeviceID, ef)
		}
		meta.addFile(protocol.LocalDeviceID, f)

		// updateGlobal depends on being able to access the previous version
		// of the file in the database, so we need to do this before we Put
		// the new file
		gk = k.GenerateGlobalVersionKey(rw, gk, folder, []byte(f.Name))
		keyBuf, _ = updateGlobal(rw, k, gk, keyBuf, folder, protocol.LocalDeviceID[:], f, meta)

		l.Debugf("insert (local); folder=%q %v", folder, f)
		rw.Put(dk, mustMarshal(&f), nil)

		keyBuf = k.GenerateSequenceKey(rw, keyBuf, folder, f.Sequence)
		rw.Put(keyBuf, dk, nil)
		l.Debugf("adding sequence; folder=%q sequence=%v %v", folder, f.Sequence, f.Name)

		if !f.IsDirectory() && !f.IsDeleted() && !f.IsInvalid() {
			for i, block := range f.Blocks {
				binary.BigEndian.PutUint32(blockBuf, uint32(i))
				keyBuf = k.GenerateBlockMapKey(rw, keyBuf, folder, block.Hash, name)
				rw.Put(keyBuf, blockBuf, nil)
			}
		}
	}
}

func (db *instance) withHave(folder, device, prefix []byte, truncate bool, fn Iterator) {
	r := db.newReadOnlyTransaction()
	defer r.close()
	withHave(r, db.keyer, folder, device, prefix, truncate, fn)
}

func withHave(r reader, k keyer, folder, device, prefix []byte, truncate bool, fn Iterator) {
	if len(prefix) > 0 {
		unslashedPrefix := prefix
		if bytes.HasSuffix(prefix, []byte{'/'}) {
			unslashedPrefix = unslashedPrefix[:len(unslashedPrefix)-1]
		} else {
			prefix = append(prefix, '/')
		}

		if f, ok := getFileTrunc(r, k.GenerateDeviceFileKey(errorWriter{}, nil, folder, device, unslashedPrefix), true); ok && !fn(f) {
			return
		}
	}

	dbi := r.NewIterator(util.BytesPrefix(k.GenerateDeviceFileKey(errorWriter{}, nil, folder, device, prefix)), nil)
	defer dbi.Release()

	for dbi.Next() {
		name := k.NameFromDeviceFileKey(dbi.Key())
		if len(prefix) > 0 && !bytes.HasPrefix(name, prefix) {
			return
		}

		f, err := unmarshalTrunc(dbi.Value(), truncate)
		if err != nil {
			l.Debugln("unmarshal error:", err)
			continue
		}
		if !fn(f) {
			return
		}
	}
}

func (db *instance) withHaveSequence(folder []byte, startSeq int64, fn Iterator) {
	t := db.newReadOnlyTransaction()
	defer t.close()
	withHaveSequence(t, db.keyer, folder, startSeq, fn)
}

func withHaveSequence(r reader, k keyer, folder []byte, startSeq int64, fn Iterator) {
	dbi := r.NewIterator(&util.Range{Start: k.GenerateSequenceKey(errorWriter{}, nil, folder, startSeq), Limit: k.GenerateSequenceKey(errorWriter{}, nil, folder, maxInt64)}, nil)
	defer dbi.Release()

	for dbi.Next() {
		f, ok := getFileByKey(r, dbi.Value())
		if !ok {
			l.Debugln("missing file for sequence number", k.SequenceFromSequenceKey(dbi.Key()))
			continue
		}

		if shouldDebug() {
			if seq := k.SequenceFromSequenceKey(dbi.Key()); f.Sequence != seq {
				l.Warnf("Sequence index corruption (folder %v, file %v): sequence %d != expected %d", string(folder), f.Name, f.Sequence, seq)
				panic("sequence index corruption")
			}
		}
		if !fn(f) {
			return
		}
	}
}

func (db *instance) withAllFolderTruncated(folder []byte, fn func(device []byte, f FileInfoTruncated) bool) {
	t := db.newReadWriteTransaction()
	defer t.close()
	withAllFolderTruncated(t, db.keyer, folder, fn)
}

func withAllFolderTruncated(rw readWriter, k keyer, folder []byte, fn func(device []byte, f FileInfoTruncated) bool) {
	dbi := rw.NewIterator(util.BytesPrefix(k.GenerateDeviceFileKey(rw, nil, folder, nil, nil).WithoutNameAndDevice()), nil)
	defer dbi.Release()

	var gk, keyBuf []byte
	for dbi.Next() {
		device, ok := k.DeviceFromDeviceFileKey(dbi.Key())
		if !ok {
			// Not having the device in the index is bad. Clear it.
			rw.Delete(dbi.Key(), nil)
			continue
		}
		var f FileInfoTruncated
		// The iterator function may keep a reference to the unmarshalled
		// struct, which in turn references the buffer it was unmarshalled
		// from. dbi.Value() just returns an internal slice that it reuses, so
		// we need to copy it.
		err := f.Unmarshal(append([]byte{}, dbi.Value()...))
		if err != nil {
			l.Debugln("unmarshal error:", err)
			continue
		}

		switch f.Name {
		case "", ".", "..", "/": // A few obviously invalid filenames
			l.Infof("Dropping invalid filename %q from database", f.Name)
			name := []byte(f.Name)
			gk = k.GenerateGlobalVersionKey(rw, gk, folder, name)
			keyBuf = removeFromGlobal(rw, k, gk, keyBuf, folder, device, name, nil)
			rw.Delete(dbi.Key(), nil)
			continue
		}

		if !fn(device, f) {
			return
		}
	}
}

func (db *instance) withGlobal(folder, prefix []byte, truncate bool, fn Iterator) {
	t := db.newReadOnlyTransaction()
	defer t.close()
	withGlobal(t, db.keyer, folder, prefix, truncate, fn)
}

func withGlobal(r reader, k keyer, folder, prefix []byte, truncate bool, fn Iterator) {
	if len(prefix) > 0 {
		unslashedPrefix := prefix
		if bytes.HasSuffix(prefix, []byte{'/'}) {
			unslashedPrefix = unslashedPrefix[:len(unslashedPrefix)-1]
		} else {
			prefix = append(prefix, '/')
		}

		if _, f, ok := getGlobal(r, k, nil, folder, unslashedPrefix, truncate); ok && !fn(f) {
			return
		}
	}

	gk, ok := k.GenerateGlobalVersionKeyRO(nil, folder, prefix)
	if !ok {
		return
	}
	dbi := r.NewIterator(util.BytesPrefix(gk), nil)
	defer dbi.Release()

	var dk []byte
	for dbi.Next() {
		name := k.NameFromGlobalVersionKey(dbi.Key())
		if len(prefix) > 0 && !bytes.HasPrefix(name, prefix) {
			return
		}

		vl, ok := unmarshalVersionList(dbi.Value())
		if !ok {
			continue
		}

		dk = k.GenerateDeviceFileKey(errorWriter{}, dk, folder, vl.Versions[0].Device, name)

		f, ok := getFileTrunc(r, dk, truncate)
		if !ok {
			continue
		}

		if !fn(f) {
			return
		}
	}
}

func (db *instance) availability(folder, file []byte) []protocol.DeviceID {
	k := db.keyer.GenerateGlobalVersionKey(db, nil, folder, file)
	bs, err := db.Get(k, nil)
	if err == leveldb.ErrNotFound {
		return nil
	}
	if err != nil {
		l.Debugln("surprise error:", err)
		return nil
	}

	vl, ok := unmarshalVersionList(bs)
	if !ok {
		return nil
	}

	var devices []protocol.DeviceID
	for _, v := range vl.Versions {
		if !v.Version.Equal(vl.Versions[0].Version) {
			break
		}
		if v.Invalid {
			continue
		}
		n := protocol.DeviceIDFromBytes(v.Device)
		devices = append(devices, n)
	}

	return devices
}

func (db *instance) withNeed(folder, device []byte, truncate bool, fn Iterator) {
	r := db.newReadOnlyTransaction()
	defer r.close()
	withNeed(r, db.keyer, folder, device, truncate, fn)
}

func withNeed(r reader, k keyer, folder, device []byte, truncate bool, fn Iterator) {
	if bytes.Equal(device, protocol.LocalDeviceID[:]) {
		withNeedLocal(r, k, folder, truncate, fn)
		return
	}

	dbi := r.NewIterator(util.BytesPrefix(k.GenerateGlobalVersionKey(errorWriter{}, nil, folder, nil).WithoutName()), nil)
	defer dbi.Release()

	var dk []byte
	devID := protocol.DeviceIDFromBytes(device)
	for dbi.Next() {
		vl, ok := unmarshalVersionList(dbi.Value())
		if !ok {
			continue
		}

		haveFV, have := vl.Get(device)
		// XXX: This marks Concurrent (i.e. conflicting) changes as
		// needs. Maybe we should do that, but it needs special
		// handling in the puller.
		if have && haveFV.Version.GreaterEqual(vl.Versions[0].Version) {
			continue
		}

		name := k.NameFromGlobalVersionKey(dbi.Key())
		needVersion := vl.Versions[0].Version
		needDevice := protocol.DeviceIDFromBytes(vl.Versions[0].Device)

		for i := range vl.Versions {
			if !vl.Versions[i].Version.Equal(needVersion) {
				// We haven't found a valid copy of the file with the needed version.
				break
			}

			if vl.Versions[i].Invalid {
				// The file is marked invalid, don't use it.
				continue
			}

			dk = k.GenerateDeviceFileKey(errorWriter{}, dk, folder, vl.Versions[i].Device, name)
			gf, ok := getFileTrunc(r, dk, truncate)
			if !ok {
				continue
			}

			if gf.IsDeleted() && !have {
				// We don't need deleted files that we don't have
				break
			}

			l.Debugf("need folder=%q device=%v name=%q have=%v invalid=%v haveV=%v globalV=%v globalDev=%v", folder, devID, name, have, haveFV.Invalid, haveFV.Version, needVersion, needDevice)

			if !fn(gf) {
				return
			}

			// This file is handled, no need to look further in the version list
			break
		}
	}
}

func (db *instance) withNeedLocal(folder, device []byte, truncate bool, fn Iterator) {
	r := db.newReadOnlyTransaction()
	defer r.close()
	withNeedLocal(r, db.keyer, folder, truncate, fn)
}

func withNeedLocal(r reader, k keyer, folder []byte, truncate bool, fn Iterator) {
	dbi := r.NewIterator(util.BytesPrefix(k.GenerateNeedFileKey(errorWriter{}, nil, folder, nil).WithoutName()), nil)
	defer dbi.Release()

	var keyBuf []byte
	var f FileIntf
	var ok bool
	for dbi.Next() {
		keyBuf, f, ok = getGlobal(r, k, keyBuf, folder, k.NameFromGlobalVersionKey(dbi.Key()), truncate)
		if !ok {
			continue
		}
		if !fn(f) {
			return
		}
	}
}

func (db *instance) dropFolder(folder []byte) {
	rw := db.newReadWriteTransaction()
	defer rw.close()
	dropFolder(rw, db.keyer, folder)
}

func dropFolder(rw readWriter, k keyer, folder []byte) {
	for _, key := range [][]byte{
		// Remove all items related to the given folder from the device->file bucket
		k.GenerateDeviceFileKey(rw, nil, folder, nil, nil).WithoutNameAndDevice(),
		// Remove all sequences related to the folder
		k.GenerateSequenceKey(rw, nil, []byte(folder), 0).WithoutSequence(),
		// Remove all items related to the given folder from the global bucket
		k.GenerateGlobalVersionKey(rw, nil, folder, nil).WithoutName(),
		// Remove all needs related to the folder
		k.GenerateNeedFileKey(rw, nil, folder, nil).WithoutName(),
		// Remove the blockmap of the folder
		k.GenerateBlockMapKey(rw, nil, folder, nil, nil).WithoutHashAndName(),
	} {
		deleteKeyPrefix(rw, key)
	}
}

func (db *instance) dropDeviceFolder(device, folder []byte, meta *metadataTracker) {
	rw := db.newReadWriteTransaction()
	defer rw.close()
	dropDeviceFolder(rw, db.keyer, device, folder, meta)
}

func dropDeviceFolder(rw readWriter, k keyer, device, folder []byte, meta *metadataTracker) {
	dbi := rw.NewIterator(util.BytesPrefix(k.GenerateDeviceFileKey(rw, nil, folder, device, nil)), nil)
	defer dbi.Release()

	var gk, keyBuf []byte
	for dbi.Next() {
		name := k.NameFromDeviceFileKey(dbi.Key())
		gk = k.GenerateGlobalVersionKey(rw, gk, folder, name)
		keyBuf = removeFromGlobal(rw, k, gk, keyBuf, folder, device, name, meta)
		rw.Delete(dbi.Key(), nil)
	}
	if bytes.Equal(device, protocol.LocalDeviceID[:]) {
		deleteKeyPrefix(rw, k.GenerateBlockMapKey(nil, nil, folder, nil, nil).WithoutHashAndName())
	}
}

func (db *instance) checkGlobals(folder []byte, meta *metadataTracker) {
	rw := db.newReadWriteTransaction()
	defer rw.close()
	checkGlobals(rw, db.keyer, folder, meta)
}

func checkGlobals(rw readWriter, k keyer, folder []byte, meta *metadataTracker) {
	dbi := rw.NewIterator(util.BytesPrefix(k.GenerateGlobalVersionKey(rw, nil, folder, nil).WithoutName()), nil)
	defer dbi.Release()

	var dk []byte
	for dbi.Next() {
		vl, ok := unmarshalVersionList(dbi.Value())
		if !ok {
			continue
		}

		// Check the global version list for consistency. An issue in previous
		// versions of goleveldb could result in reordered writes so that
		// there are global entries pointing to no longer existing files. Here
		// we find those and clear them out.

		name := k.NameFromGlobalVersionKey(dbi.Key())
		var newVL VersionList
		for i, version := range vl.Versions {
			dk = k.GenerateDeviceFileKey(rw, dk, folder, version.Device, name)
			_, err := rw.Get(dk, nil)
			if err == leveldb.ErrNotFound {
				continue
			}
			if err != nil {
				l.Debugln("surprise error:", err)
				return
			}
			newVL.Versions = append(newVL.Versions, version)

			if i == 0 {
				if fi, ok := getFileByKey(rw, dk); ok {
					meta.addFile(protocol.GlobalDeviceID, fi)
				}
			}
		}

		if len(newVL.Versions) != len(vl.Versions) {
			rw.Put(dbi.Key(), mustMarshal(&newVL), nil)
		}
	}
	l.Debugf("db check completed for %q", folder)
}

func (db *instance) getIndexID(device, folder []byte) protocol.IndexID {
	cur, err := db.Get(db.keyer.GenerateIndexIDKey(db, nil, device, folder), nil)
	if err != nil {
		return 0
	}

	var id protocol.IndexID
	if err := id.Unmarshal(cur); err != nil {
		return 0
	}

	return id
}

func (db *instance) setIndexID(device, folder []byte, id protocol.IndexID) {
	bs, _ := id.Marshal() // marshalling can't fail
	if err := db.Put(db.keyer.GenerateIndexIDKey(db, nil, device, folder), bs, nil); err != nil && err != leveldb.ErrClosed {
		panic("storing index ID: " + err.Error())
	}
}

func (db *instance) dropMtimes(folder []byte) {
	t := db.newReadWriteTransaction()
	defer t.close()
	deleteKeyPrefix(t, db.keyer.GenerateMtimesKey(t, nil, folder))
}

func (db *instance) dropFolderMeta(folder []byte) {
	t := db.newReadWriteTransaction()
	defer t.close()
	deleteKeyPrefix(t, db.keyer.GenerateFolderMetaKey(t, nil, folder))
}

func (db *instance) dropPrefix(prefix []byte) {
	t := db.newReadWriteTransaction()
	defer t.close()
	deleteKeyPrefix(t, prefix)
}

func unmarshalTrunc(bs []byte, truncate bool) (FileIntf, error) {
	if truncate {
		var tf FileInfoTruncated
		err := tf.Unmarshal(bs)
		return tf, err
	}

	var tf protocol.FileInfo
	err := tf.Unmarshal(bs)
	return tf, err
}

func unmarshalVersionList(data []byte) (VersionList, bool) {
	var vl VersionList
	if err := vl.Unmarshal(data); err != nil {
		l.Debugln("unmarshal error:", err)
		return VersionList{}, false
	}
	if len(vl.Versions) == 0 {
		l.Debugln("empty version list")
		return VersionList{}, false
	}
	return vl, true
}

type errorSuggestion struct {
	inner      error
	suggestion string
}

func (e errorSuggestion) Error() string {
	return fmt.Sprintf("%s (%s)", e.inner.Error(), e.suggestion)
}

// unchanged checks if two files are the same and thus don't need to be updated.
// Local flags or the invalid bit might change without the version
// being bumped. The IsInvalid() method handles both.
func unchanged(nf, ef FileIntf) bool {
	return ef.FileVersion().Equal(nf.FileVersion()) && ef.IsInvalid() == nf.IsInvalid()
}
