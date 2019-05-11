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
	t := db.newReadWriteTransaction()
	defer t.close()

	var dk, gk, keyBuf []byte
	devID := protocol.DeviceIDFromBytes(device)
	for _, f := range fs {
		name := []byte(f.Name)
		dk = db.keyer.GenerateDeviceFileKey(dk, folder, device, name)

		ef, ok := t.getFileTrunc(dk, true)
		if ok && unchanged(f, ef) {
			continue
		}

		if ok {
			meta.removeFile(devID, ef)
		}
		meta.addFile(devID, f)

		l.Debugf("insert; folder=%q device=%v %v", folder, devID, f)
		t.Put(dk, mustMarshal(&f))

		gk = db.keyer.GenerateGlobalVersionKey(gk, folder, name)
		keyBuf, _ = t.updateGlobal(gk, keyBuf, folder, device, f, meta)

		t.checkFlush()
	}
}

// updateLocalFiles adds fileinfos to the db, and updates the global versionlist,
// metadata, sequence and blockmap buckets.
func (db *instance) updateLocalFiles(folder []byte, fs []protocol.FileInfo, meta *metadataTracker) {
	t := db.newReadWriteTransaction()
	defer t.close()

	var dk, gk, keyBuf []byte
	blockBuf := make([]byte, 4)
	for _, f := range fs {
		name := []byte(f.Name)
		dk = db.keyer.GenerateDeviceFileKey(dk, folder, protocol.LocalDeviceID[:], name)

		ef, ok := t.getFileByKey(dk)
		if ok && unchanged(f, ef) {
			continue
		}

		if ok {
			if !ef.IsDirectory() && !ef.IsDeleted() && !ef.IsInvalid() {
				for _, block := range ef.Blocks {
					keyBuf = db.keyer.GenerateBlockMapKey(keyBuf, folder, block.Hash, name)
					t.Delete(keyBuf)
				}
			}

			keyBuf = db.keyer.GenerateSequenceKey(keyBuf, folder, ef.SequenceNo())
			t.Delete(keyBuf)
			l.Debugf("removing sequence; folder=%q sequence=%v %v", folder, ef.SequenceNo(), ef.FileName())
		}

		f.Sequence = meta.nextLocalSeq()

		if ok {
			meta.removeFile(protocol.LocalDeviceID, ef)
		}
		meta.addFile(protocol.LocalDeviceID, f)

		l.Debugf("insert (local); folder=%q %v", folder, f)
		t.Put(dk, mustMarshal(&f))

		gk = db.keyer.GenerateGlobalVersionKey(gk, folder, []byte(f.Name))
		keyBuf, _ = t.updateGlobal(gk, keyBuf, folder, protocol.LocalDeviceID[:], f, meta)

		keyBuf = db.keyer.GenerateSequenceKey(keyBuf, folder, f.Sequence)
		t.Put(keyBuf, dk)
		l.Debugf("adding sequence; folder=%q sequence=%v %v", folder, f.Sequence, f.Name)

		if !f.IsDirectory() && !f.IsDeleted() && !f.IsInvalid() {
			for i, block := range f.Blocks {
				binary.BigEndian.PutUint32(blockBuf, uint32(i))
				keyBuf = db.keyer.GenerateBlockMapKey(keyBuf, folder, block.Hash, name)
				t.Put(keyBuf, blockBuf)
			}
		}

		t.checkFlush()
	}
}

func (db *instance) withHave(folder, device, prefix []byte, truncate bool, fn Iterator) {
	t := db.newReadOnlyTransaction()
	defer t.close()

	if len(prefix) > 0 {
		unslashedPrefix := prefix
		if bytes.HasSuffix(prefix, []byte{'/'}) {
			unslashedPrefix = unslashedPrefix[:len(unslashedPrefix)-1]
		} else {
			prefix = append(prefix, '/')
		}

		if f, ok := t.getFileTrunc(db.keyer.GenerateDeviceFileKey(nil, folder, device, unslashedPrefix), true); ok && !fn(f) {
			return
		}
	}

	dbi := t.NewIterator(util.BytesPrefix(db.keyer.GenerateDeviceFileKey(nil, folder, device, prefix)), nil)
	defer dbi.Release()

	for dbi.Next() {
		name := db.keyer.NameFromDeviceFileKey(dbi.Key())
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

	dbi := t.NewIterator(&util.Range{Start: db.keyer.GenerateSequenceKey(nil, folder, startSeq), Limit: db.keyer.GenerateSequenceKey(nil, folder, maxInt64)}, nil)
	defer dbi.Release()

	for dbi.Next() {
		f, ok := t.getFileByKey(dbi.Value())
		if !ok {
			l.Debugln("missing file for sequence number", db.keyer.SequenceFromSequenceKey(dbi.Key()))
			continue
		}

		if shouldDebug() {
			if seq := db.keyer.SequenceFromSequenceKey(dbi.Key()); f.Sequence != seq {
				panic(fmt.Sprintf("sequence index corruption (folder %v, file %v): sequence %d != expected %d", string(folder), f.Name, f.Sequence, seq))
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

	dbi := t.NewIterator(util.BytesPrefix(db.keyer.GenerateDeviceFileKey(nil, folder, nil, nil).WithoutNameAndDevice()), nil)
	defer dbi.Release()

	var gk, keyBuf []byte
	for dbi.Next() {
		device, ok := db.keyer.DeviceFromDeviceFileKey(dbi.Key())
		if !ok {
			// Not having the device in the index is bad. Clear it.
			t.Delete(dbi.Key())
			t.checkFlush()
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
			gk = db.keyer.GenerateGlobalVersionKey(gk, folder, name)
			keyBuf = t.removeFromGlobal(gk, keyBuf, folder, device, name, nil)
			t.Delete(dbi.Key())
			t.checkFlush()
			continue
		}

		if !fn(device, f) {
			return
		}
	}
}

func (db *instance) getFileDirty(folder, device, file []byte) (protocol.FileInfo, bool) {
	t := db.newReadOnlyTransaction()
	defer t.close()
	return t.getFile(folder, device, file)
}

func (db *instance) getGlobalDirty(folder, file []byte, truncate bool) (FileIntf, bool) {
	t := db.newReadOnlyTransaction()
	defer t.close()
	_, f, ok := t.getGlobal(nil, folder, file, truncate)
	return f, ok
}

func (db *instance) withGlobal(folder, prefix []byte, truncate bool, fn Iterator) {
	t := db.newReadOnlyTransaction()
	defer t.close()

	if len(prefix) > 0 {
		unslashedPrefix := prefix
		if bytes.HasSuffix(prefix, []byte{'/'}) {
			unslashedPrefix = unslashedPrefix[:len(unslashedPrefix)-1]
		} else {
			prefix = append(prefix, '/')
		}

		if _, f, ok := t.getGlobal(nil, folder, unslashedPrefix, truncate); ok && !fn(f) {
			return
		}
	}

	dbi := t.NewIterator(util.BytesPrefix(db.keyer.GenerateGlobalVersionKey(nil, folder, prefix)), nil)
	defer dbi.Release()

	var dk []byte
	for dbi.Next() {
		name := db.keyer.NameFromGlobalVersionKey(dbi.Key())
		if len(prefix) > 0 && !bytes.HasPrefix(name, prefix) {
			return
		}

		vl, ok := unmarshalVersionList(dbi.Value())
		if !ok {
			continue
		}

		dk = db.keyer.GenerateDeviceFileKey(dk, folder, vl.Versions[0].Device, name)

		f, ok := t.getFileTrunc(dk, truncate)
		if !ok {
			continue
		}

		if !fn(f) {
			return
		}
	}
}

func (db *instance) availability(folder, file []byte) []protocol.DeviceID {
	k := db.keyer.GenerateGlobalVersionKey(nil, folder, file)
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
	if bytes.Equal(device, protocol.LocalDeviceID[:]) {
		db.withNeedLocal(folder, truncate, fn)
		return
	}

	t := db.newReadOnlyTransaction()
	defer t.close()

	dbi := t.NewIterator(util.BytesPrefix(db.keyer.GenerateGlobalVersionKey(nil, folder, nil).WithoutName()), nil)
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

		name := db.keyer.NameFromGlobalVersionKey(dbi.Key())
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

			dk = db.keyer.GenerateDeviceFileKey(dk, folder, vl.Versions[i].Device, name)
			gf, ok := t.getFileTrunc(dk, truncate)
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

func (db *instance) withNeedLocal(folder []byte, truncate bool, fn Iterator) {
	t := db.newReadOnlyTransaction()
	defer t.close()

	dbi := t.NewIterator(util.BytesPrefix(db.keyer.GenerateNeedFileKey(nil, folder, nil).WithoutName()), nil)
	defer dbi.Release()

	var keyBuf []byte
	var f FileIntf
	var ok bool
	for dbi.Next() {
		keyBuf, f, ok = t.getGlobal(keyBuf, folder, db.keyer.NameFromGlobalVersionKey(dbi.Key()), truncate)
		if !ok {
			continue
		}
		if !fn(f) {
			return
		}
	}
}

func (db *instance) dropFolder(folder []byte) {
	t := db.newReadWriteTransaction()
	defer t.close()

	for _, key := range [][]byte{
		// Remove all items related to the given folder from the device->file bucket
		db.keyer.GenerateDeviceFileKey(nil, folder, nil, nil).WithoutNameAndDevice(),
		// Remove all sequences related to the folder
		db.keyer.GenerateSequenceKey(nil, []byte(folder), 0).WithoutSequence(),
		// Remove all items related to the given folder from the global bucket
		db.keyer.GenerateGlobalVersionKey(nil, folder, nil).WithoutName(),
		// Remove all needs related to the folder
		db.keyer.GenerateNeedFileKey(nil, folder, nil).WithoutName(),
		// Remove the blockmap of the folder
		db.keyer.GenerateBlockMapKey(nil, folder, nil, nil).WithoutHashAndName(),
	} {
		t.deleteKeyPrefix(key)
	}
}

func (db *instance) dropDeviceFolder(device, folder []byte, meta *metadataTracker) {
	t := db.newReadWriteTransaction()
	defer t.close()

	dbi := t.NewIterator(util.BytesPrefix(db.keyer.GenerateDeviceFileKey(nil, folder, device, nil)), nil)
	defer dbi.Release()

	var gk, keyBuf []byte
	for dbi.Next() {
		name := db.keyer.NameFromDeviceFileKey(dbi.Key())
		gk = db.keyer.GenerateGlobalVersionKey(gk, folder, name)
		keyBuf = t.removeFromGlobal(gk, keyBuf, folder, device, name, meta)
		t.Delete(dbi.Key())
		t.checkFlush()
	}
	if bytes.Equal(device, protocol.LocalDeviceID[:]) {
		t.deleteKeyPrefix(db.keyer.GenerateBlockMapKey(nil, folder, nil, nil).WithoutHashAndName())
	}
}

func (db *instance) checkGlobals(folder []byte, meta *metadataTracker) {
	t := db.newReadWriteTransaction()
	defer t.close()

	dbi := t.NewIterator(util.BytesPrefix(db.keyer.GenerateGlobalVersionKey(nil, folder, nil).WithoutName()), nil)
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

		name := db.keyer.NameFromGlobalVersionKey(dbi.Key())
		var newVL VersionList
		for i, version := range vl.Versions {
			dk = db.keyer.GenerateDeviceFileKey(dk, folder, version.Device, name)
			_, err := t.Get(dk, nil)
			if err == leveldb.ErrNotFound {
				continue
			}
			if err != nil {
				l.Debugln("surprise error:", err)
				return
			}
			newVL.Versions = append(newVL.Versions, version)

			if i == 0 {
				if fi, ok := t.getFileByKey(dk); ok {
					meta.addFile(protocol.GlobalDeviceID, fi)
				}
			}
		}

		if len(newVL.Versions) != len(vl.Versions) {
			t.Put(dbi.Key(), mustMarshal(&newVL))
			t.checkFlush()
		}
	}
	l.Debugf("db check completed for %q", folder)
}

func (db *instance) getIndexID(device, folder []byte) protocol.IndexID {
	cur, err := db.Get(db.keyer.GenerateIndexIDKey(nil, device, folder), nil)
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
	if err := db.Put(db.keyer.GenerateIndexIDKey(nil, device, folder), bs, nil); err != nil && err != leveldb.ErrClosed {
		panic("storing index ID: " + err.Error())
	}
}

func (db *instance) dropMtimes(folder []byte) {
	db.dropPrefix(db.keyer.GenerateMtimesKey(nil, folder))
}

func (db *instance) dropFolderMeta(folder []byte) {
	db.dropPrefix(db.keyer.GenerateFolderMetaKey(nil, folder))
}

func (db *instance) dropPrefix(prefix []byte) {
	t := db.newReadWriteTransaction()
	defer t.close()

	t.deleteKeyPrefix(prefix)
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
