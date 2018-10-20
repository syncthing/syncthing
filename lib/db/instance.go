// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package db

import (
	"bytes"
	"fmt"

	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/iterator"
	"github.com/syndtr/goleveldb/leveldb/util"
)

type deletionHandler func(t readWriteTransaction, folder, device, name []byte, dbi iterator.Iterator)

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

func (db *instance) updateFiles(folder, device []byte, fs []protocol.FileInfo, meta *metadataTracker) {
	t := db.newReadWriteTransaction()
	defer t.close()

	var fk []byte
	var gk []byte
	for _, f := range fs {
		name := []byte(f.Name)
		fk = db.keyer.GenerateDeviceFileKey(fk, folder, device, name)

		// Get and unmarshal the file entry. If it doesn't exist or can't be
		// unmarshalled we'll add it as a new entry.
		bs, err := t.Get(fk, nil)
		var ef FileInfoTruncated
		if err == nil {
			err = ef.Unmarshal(bs)
		}

		// Local flags or the invalid bit might change without the version
		// being bumped. The IsInvalid() method handles both.
		if err == nil && ef.Version.Equal(f.Version) && ef.IsInvalid() == f.IsInvalid() {
			continue
		}

		devID := protocol.DeviceIDFromBytes(device)
		if err == nil {
			meta.removeFile(devID, ef)
		}
		meta.addFile(devID, f)

		t.insertFile(fk, folder, device, f)

		gk = db.keyer.GenerateGlobalVersionKey(gk, folder, name)
		t.updateGlobal(gk, folder, device, f, meta)

		// Write out and reuse the batch every few records, to avoid the batch
		// growing too large and thus allocating unnecessarily much memory.
		t.checkFlush()
	}
}

func (db *instance) addSequences(folder []byte, fs []protocol.FileInfo) {
	t := db.newReadWriteTransaction()
	defer t.close()

	var sk []byte
	var dk []byte
	for _, f := range fs {
		sk = db.keyer.GenerateSequenceKey(sk, folder, f.Sequence)
		dk = db.keyer.GenerateDeviceFileKey(dk, folder, protocol.LocalDeviceID[:], []byte(f.Name))
		t.Put(sk, dk)
		l.Debugf("adding sequence; folder=%q sequence=%v %v", folder, f.Sequence, f.Name)
		t.checkFlush()
	}
}

func (db *instance) removeSequences(folder []byte, fs []protocol.FileInfo) {
	t := db.newReadWriteTransaction()
	defer t.close()

	var sk []byte
	for _, f := range fs {
		t.Delete(db.keyer.GenerateSequenceKey(sk, folder, f.Sequence))
		l.Debugf("removing sequence; folder=%q sequence=%v %v", folder, f.Sequence, f.Name)
		t.checkFlush()
	}
}

func (db *instance) withHave(folder, device, prefix []byte, truncate bool, fn Iterator) {
	if len(prefix) > 0 {
		unslashedPrefix := prefix
		if bytes.HasSuffix(prefix, []byte{'/'}) {
			unslashedPrefix = unslashedPrefix[:len(unslashedPrefix)-1]
		} else {
			prefix = append(prefix, '/')
		}

		if f, ok := db.getFileTrunc(db.keyer.GenerateDeviceFileKey(nil, folder, device, unslashedPrefix), true); ok && !fn(f) {
			return
		}
	}

	t := db.newReadOnlyTransaction()
	defer t.close()

	dbi := t.NewIterator(util.BytesPrefix(db.keyer.GenerateDeviceFileKey(nil, folder, device, prefix)), nil)
	defer dbi.Release()

	for dbi.Next() {
		name := db.keyer.NameFromDeviceFileKey(dbi.Key())
		if len(prefix) > 0 && !bytes.HasPrefix(name, prefix) {
			return
		}

		// The iterator function may keep a reference to the unmarshalled
		// struct, which in turn references the buffer it was unmarshalled
		// from. dbi.Value() just returns an internal slice that it reuses, so
		// we need to copy it.
		f, err := unmarshalTrunc(append([]byte{}, dbi.Value()...), truncate)
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
		f, ok := db.getFile(dbi.Value())
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

	var gk []byte

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
			t.removeFromGlobal(gk, folder, device, name, nil)
			t.Delete(dbi.Key())
			t.checkFlush()
			continue
		}

		if !fn(device, f) {
			return
		}
	}
}

func (db *instance) getFile(key []byte) (protocol.FileInfo, bool) {
	if f, ok := db.getFileTrunc(key, false); ok {
		return f.(protocol.FileInfo), true
	}
	return protocol.FileInfo{}, false
}

func (db *instance) getFileTrunc(key []byte, trunc bool) (FileIntf, bool) {
	bs, err := db.Get(key, nil)
	if err == leveldb.ErrNotFound {
		return nil, false
	}
	if err != nil {
		l.Debugln("surprise error:", err)
		return nil, false
	}

	f, err := unmarshalTrunc(bs, trunc)
	if err != nil {
		l.Debugln("unmarshal error:", err)
		return nil, false
	}
	return f, true
}

func (db *instance) getGlobal(folder, file []byte, truncate bool) (FileIntf, bool) {
	t := db.newReadOnlyTransaction()
	defer t.close()

	_, _, f, ok := db.getGlobalInto(t, nil, nil, folder, file, truncate)
	return f, ok
}

func (db *instance) getGlobalInto(t readOnlyTransaction, gk, dk, folder, file []byte, truncate bool) ([]byte, []byte, FileIntf, bool) {
	gk = db.keyer.GenerateGlobalVersionKey(gk, folder, file)

	bs, err := t.Get(gk, nil)
	if err != nil {
		return gk, dk, nil, false
	}

	vl, ok := unmarshalVersionList(bs)
	if !ok {
		return gk, dk, nil, false
	}

	dk = db.keyer.GenerateDeviceFileKey(dk, folder, vl.Versions[0].Device, file)
	if fi, ok := db.getFileTrunc(dk, truncate); ok {
		return gk, dk, fi, true
	}

	return gk, dk, nil, false
}

func (db *instance) withGlobal(folder, prefix []byte, truncate bool, fn Iterator) {
	if len(prefix) > 0 {
		unslashedPrefix := prefix
		if bytes.HasSuffix(prefix, []byte{'/'}) {
			unslashedPrefix = unslashedPrefix[:len(unslashedPrefix)-1]
		} else {
			prefix = append(prefix, '/')
		}

		if f, ok := db.getGlobal(folder, unslashedPrefix, truncate); ok && !fn(f) {
			return
		}
	}

	t := db.newReadOnlyTransaction()
	defer t.close()

	dbi := t.NewIterator(util.BytesPrefix(db.keyer.GenerateGlobalVersionKey(nil, folder, prefix)), nil)
	defer dbi.Release()

	var fk []byte
	for dbi.Next() {
		name := db.keyer.NameFromGlobalVersionKey(dbi.Key())
		if len(prefix) > 0 && !bytes.HasPrefix(name, prefix) {
			return
		}

		vl, ok := unmarshalVersionList(dbi.Value())
		if !ok {
			continue
		}

		fk = db.keyer.GenerateDeviceFileKey(fk, folder, vl.Versions[0].Device, name)

		f, ok := db.getFileTrunc(fk, truncate)
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

	var fk []byte
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

			fk = db.keyer.GenerateDeviceFileKey(fk, folder, vl.Versions[i].Device, name)
			bs, err := t.Get(fk, nil)
			if err != nil {
				l.Debugln("surprise error:", err)
				continue
			}

			gf, err := unmarshalTrunc(bs, truncate)
			if err != nil {
				l.Debugln("unmarshal error:", err)
				continue
			}

			if gf.IsDeleted() && !have {
				// We don't need deleted files that we don't have
				break
			}

			l.Debugf("need folder=%q device=%v name=%q have=%v invalid=%v haveV=%v globalV=%v globalDev=%v", folder, protocol.DeviceIDFromBytes(device), name, have, haveFV.Invalid, haveFV.Version, needVersion, needDevice)

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

	var dk []byte
	var gk []byte
	var f FileIntf
	var ok bool
	for dbi.Next() {
		gk, dk, f, ok = db.getGlobalInto(t, gk, dk, folder, db.keyer.NameFromGlobalVersionKey(dbi.Key()), truncate)
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
	} {
		t.deleteKeyPrefix(key)
	}
}

func (db *instance) dropDeviceFolder(device, folder []byte, meta *metadataTracker) {
	t := db.newReadWriteTransaction()
	defer t.close()

	dbi := t.NewIterator(util.BytesPrefix(db.keyer.GenerateDeviceFileKey(nil, folder, device, nil)), nil)
	defer dbi.Release()

	var gk []byte

	for dbi.Next() {
		key := dbi.Key()
		name := db.keyer.NameFromDeviceFileKey(key)
		gk = db.keyer.GenerateGlobalVersionKey(gk, folder, name)
		t.removeFromGlobal(gk, folder, device, name, meta)
		t.Delete(key)
		t.checkFlush()
	}
}

func (db *instance) checkGlobals(folder []byte, meta *metadataTracker) {
	t := db.newReadWriteTransaction()
	defer t.close()

	dbi := t.NewIterator(util.BytesPrefix(db.keyer.GenerateGlobalVersionKey(nil, folder, nil).WithoutName()), nil)
	defer dbi.Release()

	var fk []byte
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
			fk = db.keyer.GenerateDeviceFileKey(fk, folder, version.Device, name)
			_, err := t.Get(fk, nil)
			if err == leveldb.ErrNotFound {
				continue
			}
			if err != nil {
				l.Debugln("surprise error:", err)
				return
			}
			newVL.Versions = append(newVL.Versions, version)

			if i == 0 {
				if fi, ok := db.getFile(fk); ok {
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
	key := db.keyer.GenerateIndexIDKey(nil, device, folder)
	cur, err := db.Get(key, nil)
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
	key := db.keyer.GenerateIndexIDKey(nil, device, folder)
	bs, _ := id.Marshal() // marshalling can't fail
	if err := db.Put(key, bs, nil); err != nil {
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

	dbi := t.NewIterator(util.BytesPrefix(prefix), nil)
	defer dbi.Release()

	for dbi.Next() {
		t.Delete(dbi.Key())
	}
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
