// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package db

import (
	"bytes"
	"encoding/binary"

	"github.com/syncthing/syncthing/lib/db/backend"
	"github.com/syncthing/syncthing/lib/protocol"
)

// Lowlevel is the lowest level database interface. It has a very simple
// purpose: hold the actual backend database, and the in-memory state
// that belong to that database. In the same way that a single on disk
// database can only be opened once, there should be only one Lowlevel for
// any given backend.
type Lowlevel struct {
	backend.Backend
	folderIdx *smallIndex
	deviceIdx *smallIndex
	keyer     keyer
}

func NewLowlevel(backend backend.Backend) *Lowlevel {
	db := &Lowlevel{
		Backend:   backend,
		folderIdx: newSmallIndex(backend, []byte{KeyTypeFolderIdx}),
		deviceIdx: newSmallIndex(backend, []byte{KeyTypeDeviceIdx}),
	}
	db.keyer = newDefaultKeyer(db.folderIdx, db.deviceIdx)
	return db
}

// ListFolders returns the list of folders currently in the database
func (db *Lowlevel) ListFolders() []string {
	return db.folderIdx.Values()
}

// updateRemoteFiles adds a list of fileinfos to the database and updates the
// global versionlist and metadata.
func (db *Lowlevel) updateRemoteFiles(folder, device []byte, fs []protocol.FileInfo, meta *metadataTracker) error {
	t, err := db.newReadWriteTransaction()
	if err != nil {
		return err
	}
	defer t.close()

	var dk, gk, keyBuf []byte
	devID := protocol.DeviceIDFromBytes(device)
	for _, f := range fs {
		name := []byte(f.Name)
		dk, err = db.keyer.GenerateDeviceFileKey(dk, folder, device, name)
		if err != nil {
			return err
		}

		ef, ok, err := t.getFileTrunc(dk, true)
		if err != nil {
			return err
		}
		if ok && unchanged(f, ef) {
			continue
		}

		if ok {
			meta.removeFile(devID, ef)
		}
		meta.addFile(devID, f)

		l.Debugf("insert; folder=%q device=%v %v", folder, devID, f)
		if err := t.Put(dk, mustMarshal(&f)); err != nil {
			return err
		}

		gk, err = db.keyer.GenerateGlobalVersionKey(gk, folder, name)
		if err != nil {
			return err
		}
		keyBuf, _, err = t.updateGlobal(gk, keyBuf, folder, device, f, meta)
		if err != nil {
			return err
		}

		if err := t.Checkpoint(); err != nil {
			return err
		}
	}

	return t.commit()
}

// updateLocalFiles adds fileinfos to the db, and updates the global versionlist,
// metadata, sequence and blockmap buckets.
func (db *Lowlevel) updateLocalFiles(folder []byte, fs []protocol.FileInfo, meta *metadataTracker) error {
	t, err := db.newReadWriteTransaction()
	if err != nil {
		return err
	}
	defer t.close()

	var dk, gk, keyBuf []byte
	blockBuf := make([]byte, 4)
	for _, f := range fs {
		name := []byte(f.Name)
		dk, err = db.keyer.GenerateDeviceFileKey(dk, folder, protocol.LocalDeviceID[:], name)
		if err != nil {
			return err
		}

		ef, ok, err := t.getFileByKey(dk)
		if err != nil {
			return err
		}
		if ok && unchanged(f, ef) {
			continue
		}

		if ok {
			if !ef.IsDirectory() && !ef.IsDeleted() && !ef.IsInvalid() {
				for _, block := range ef.Blocks {
					keyBuf, err = db.keyer.GenerateBlockMapKey(keyBuf, folder, block.Hash, name)
					if err != nil {
						return err
					}
					if err := t.Delete(keyBuf); err != nil {
						return err
					}
				}
			}

			keyBuf, err = db.keyer.GenerateSequenceKey(keyBuf, folder, ef.SequenceNo())
			if err != nil {
				return err
			}
			if err := t.Delete(keyBuf); err != nil {
				return err
			}
			l.Debugf("removing sequence; folder=%q sequence=%v %v", folder, ef.SequenceNo(), ef.FileName())
		}

		f.Sequence = meta.nextLocalSeq()

		if ok {
			meta.removeFile(protocol.LocalDeviceID, ef)
		}
		meta.addFile(protocol.LocalDeviceID, f)

		l.Debugf("insert (local); folder=%q %v", folder, f)
		if err := t.Put(dk, mustMarshal(&f)); err != nil {
			return err
		}

		gk, err = db.keyer.GenerateGlobalVersionKey(gk, folder, []byte(f.Name))
		if err != nil {
			return err
		}
		keyBuf, _, err = t.updateGlobal(gk, keyBuf, folder, protocol.LocalDeviceID[:], f, meta)
		if err != nil {
			return err
		}

		keyBuf, err = db.keyer.GenerateSequenceKey(keyBuf, folder, f.Sequence)
		if err != nil {
			return err
		}
		if err := t.Put(keyBuf, dk); err != nil {
			return err
		}
		l.Debugf("adding sequence; folder=%q sequence=%v %v", folder, f.Sequence, f.Name)

		if !f.IsDirectory() && !f.IsDeleted() && !f.IsInvalid() {
			for i, block := range f.Blocks {
				binary.BigEndian.PutUint32(blockBuf, uint32(i))
				keyBuf, err = db.keyer.GenerateBlockMapKey(keyBuf, folder, block.Hash, name)
				if err != nil {
					return err
				}
				if err := t.Put(keyBuf, blockBuf); err != nil {
					return err
				}
			}
		}

		if err := t.Checkpoint(); err != nil {
			return err
		}
	}

	return t.commit()
}

func (db *Lowlevel) withHave(folder, device, prefix []byte, truncate bool, fn Iterator) error {
	t, err := db.newReadOnlyTransaction()
	if err != nil {
		return err
	}
	defer t.close()

	if len(prefix) > 0 {
		unslashedPrefix := prefix
		if bytes.HasSuffix(prefix, []byte{'/'}) {
			unslashedPrefix = unslashedPrefix[:len(unslashedPrefix)-1]
		} else {
			prefix = append(prefix, '/')
		}

		key, err := db.keyer.GenerateDeviceFileKey(nil, folder, device, unslashedPrefix)
		if err != nil {
			return err
		}
		if f, ok, err := t.getFileTrunc(key, true); err != nil {
			return err
		} else if ok && !fn(f) {
			return nil
		}
	}

	key, err := db.keyer.GenerateDeviceFileKey(nil, folder, device, prefix)
	if err != nil {
		return err
	}
	dbi, err := t.NewPrefixIterator(key)
	if err != nil {
		return err
	}
	defer dbi.Release()

	for dbi.Next() {
		name := db.keyer.NameFromDeviceFileKey(dbi.Key())
		if len(prefix) > 0 && !bytes.HasPrefix(name, prefix) {
			return nil
		}

		f, err := unmarshalTrunc(dbi.Value(), truncate)
		if err != nil {
			l.Debugln("unmarshal error:", err)
			continue
		}
		if !fn(f) {
			return nil
		}
	}
	return dbi.Error()
}

func (db *Lowlevel) withHaveSequence(folder []byte, startSeq int64, fn Iterator) error {
	t, err := db.newReadOnlyTransaction()
	if err != nil {
		return err
	}
	defer t.close()

	first, err := db.keyer.GenerateSequenceKey(nil, folder, startSeq)
	if err != nil {
		return err
	}
	last, err := db.keyer.GenerateSequenceKey(nil, folder, maxInt64)
	if err != nil {
		return err
	}
	dbi, err := t.NewRangeIterator(first, last)
	if err != nil {
		return err
	}
	defer dbi.Release()

	for dbi.Next() {
		f, ok, err := t.getFileByKey(dbi.Value())
		if err != nil {
			return err
		}
		if !ok {
			l.Debugln("missing file for sequence number", db.keyer.SequenceFromSequenceKey(dbi.Key()))
			continue
		}

		if shouldDebug() {
			if seq := db.keyer.SequenceFromSequenceKey(dbi.Key()); f.Sequence != seq {
				l.Warnf("Sequence index corruption (folder %v, file %v): sequence %d != expected %d", string(folder), f.Name, f.Sequence, seq)
				panic("sequence index corruption")
			}
		}
		if !fn(f) {
			return nil
		}
	}
	return dbi.Error()
}

func (db *Lowlevel) withAllFolderTruncated(folder []byte, fn func(device []byte, f FileInfoTruncated) bool) error {
	t, err := db.newReadWriteTransaction()
	if err != nil {
		return err
	}
	defer t.close()

	key, err := db.keyer.GenerateDeviceFileKey(nil, folder, nil, nil)
	if err != nil {
		return err
	}
	dbi, err := t.NewPrefixIterator(key.WithoutNameAndDevice())
	if err != nil {
		return err
	}
	defer dbi.Release()

	var gk, keyBuf []byte
	for dbi.Next() {
		device, ok := db.keyer.DeviceFromDeviceFileKey(dbi.Key())
		if !ok {
			// Not having the device in the index is bad. Clear it.
			if err := t.Delete(dbi.Key()); err != nil {
				return err
			}
			continue
		}
		var f FileInfoTruncated
		// The iterator function may keep a reference to the unmarshalled
		// struct, which in turn references the buffer it was unmarshalled
		// from. dbi.Value() just returns an internal slice that it reuses, so
		// we need to copy it.
		err := f.Unmarshal(append([]byte{}, dbi.Value()...))
		if err != nil {
			return err
		}

		switch f.Name {
		case "", ".", "..", "/": // A few obviously invalid filenames
			l.Infof("Dropping invalid filename %q from database", f.Name)
			name := []byte(f.Name)
			gk, err = db.keyer.GenerateGlobalVersionKey(gk, folder, name)
			if err != nil {
				return err
			}
			keyBuf, err = t.removeFromGlobal(gk, keyBuf, folder, device, name, nil)
			if err != nil {
				return err
			}
			if err := t.Delete(dbi.Key()); err != nil {
				return err
			}
			continue
		}

		if !fn(device, f) {
			return nil
		}
	}
	if err := dbi.Error(); err != nil {
		return err
	}
	return t.commit()
}

func (db *Lowlevel) getFileDirty(folder, device, file []byte) (protocol.FileInfo, bool, error) {
	t, err := db.newReadOnlyTransaction()
	if err != nil {
		return protocol.FileInfo{}, false, err
	}
	defer t.close()
	return t.getFile(folder, device, file)
}

func (db *Lowlevel) getGlobalDirty(folder, file []byte, truncate bool) (FileIntf, bool, error) {
	t, err := db.newReadOnlyTransaction()
	if err != nil {
		return nil, false, err
	}
	defer t.close()
	_, f, ok, err := t.getGlobal(nil, folder, file, truncate)
	return f, ok, err
}

func (db *Lowlevel) withGlobal(folder, prefix []byte, truncate bool, fn Iterator) error {
	t, err := db.newReadOnlyTransaction()
	if err != nil {
		return err
	}
	defer t.close()

	if len(prefix) > 0 {
		unslashedPrefix := prefix
		if bytes.HasSuffix(prefix, []byte{'/'}) {
			unslashedPrefix = unslashedPrefix[:len(unslashedPrefix)-1]
		} else {
			prefix = append(prefix, '/')
		}

		if _, f, ok, err := t.getGlobal(nil, folder, unslashedPrefix, truncate); err != nil {
			return err
		} else if ok && !fn(f) {
			return nil
		}
	}

	key, err := db.keyer.GenerateGlobalVersionKey(nil, folder, prefix)
	if err != nil {
		return err
	}
	dbi, err := t.NewPrefixIterator(key)
	if err != nil {
		return err
	}
	defer dbi.Release()

	var dk []byte
	for dbi.Next() {
		name := db.keyer.NameFromGlobalVersionKey(dbi.Key())
		if len(prefix) > 0 && !bytes.HasPrefix(name, prefix) {
			return nil
		}

		vl, ok := unmarshalVersionList(dbi.Value())
		if !ok {
			continue
		}

		dk, err = db.keyer.GenerateDeviceFileKey(dk, folder, vl.Versions[0].Device, name)
		if err != nil {
			return err
		}

		f, ok, err := t.getFileTrunc(dk, truncate)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}

		if !fn(f) {
			return nil
		}
	}
	if err != nil {
		return err
	}
	return dbi.Error()
}

func (db *Lowlevel) availability(folder, file []byte) ([]protocol.DeviceID, error) {
	k, err := db.keyer.GenerateGlobalVersionKey(nil, folder, file)
	if err != nil {
		return nil, err
	}
	bs, err := db.Get(k)
	if backend.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	vl, ok := unmarshalVersionList(bs)
	if !ok {
		return nil, nil
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

	return devices, nil
}

func (db *Lowlevel) withNeed(folder, device []byte, truncate bool, fn Iterator) error {
	if bytes.Equal(device, protocol.LocalDeviceID[:]) {
		return db.withNeedLocal(folder, truncate, fn)
	}

	t, err := db.newReadOnlyTransaction()
	if err != nil {
		return err
	}
	defer t.close()

	key, err := db.keyer.GenerateGlobalVersionKey(nil, folder, nil)
	if err != nil {
		return err
	}
	dbi, err := t.NewPrefixIterator(key.WithoutName())
	if err != nil {
		return err
	}
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

			dk, err = db.keyer.GenerateDeviceFileKey(dk, folder, vl.Versions[i].Device, name)
			if err != nil {
				return err
			}
			gf, ok, err := t.getFileTrunc(dk, truncate)
			if err != nil {
				return err
			}
			if !ok {
				continue
			}

			if gf.IsDeleted() && !have {
				// We don't need deleted files that we don't have
				break
			}

			l.Debugf("need folder=%q device=%v name=%q have=%v invalid=%v haveV=%v globalV=%v globalDev=%v", folder, devID, name, have, haveFV.Invalid, haveFV.Version, needVersion, needDevice)

			if !fn(gf) {
				return nil
			}

			// This file is handled, no need to look further in the version list
			break
		}
	}
	return dbi.Error()
}

func (db *Lowlevel) withNeedLocal(folder []byte, truncate bool, fn Iterator) error {
	t, err := db.newReadOnlyTransaction()
	if err != nil {
		return err
	}
	defer t.close()

	key, err := db.keyer.GenerateNeedFileKey(nil, folder, nil)
	if err != nil {
		return err
	}
	dbi, err := t.NewPrefixIterator(key.WithoutName())
	if err != nil {
		return err
	}
	defer dbi.Release()

	var keyBuf []byte
	var f FileIntf
	var ok bool
	for dbi.Next() {
		keyBuf, f, ok, err = t.getGlobal(keyBuf, folder, db.keyer.NameFromGlobalVersionKey(dbi.Key()), truncate)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		if !fn(f) {
			return nil
		}
	}
	return dbi.Error()
}

func (db *Lowlevel) dropFolder(folder []byte) error {
	t, err := db.newReadWriteTransaction()
	if err != nil {
		return err
	}
	defer t.close()

	// Remove all items related to the given folder from the device->file bucket
	k0, err := db.keyer.GenerateDeviceFileKey(nil, folder, nil, nil)
	if err != nil {
		return err
	}
	if err := t.deleteKeyPrefix(k0.WithoutNameAndDevice()); err != nil {
		return err
	}

	// Remove all sequences related to the folder
	k1, err := db.keyer.GenerateSequenceKey(nil, folder, 0)
	if err != nil {
		return err
	}
	if err := t.deleteKeyPrefix(k1.WithoutSequence()); err != nil {
		return err
	}

	// Remove all items related to the given folder from the global bucket
	k2, err := db.keyer.GenerateGlobalVersionKey(nil, folder, nil)
	if err != nil {
		return err
	}
	if err := t.deleteKeyPrefix(k2.WithoutName()); err != nil {
		return err
	}

	// Remove all needs related to the folder
	k3, err := db.keyer.GenerateNeedFileKey(nil, folder, nil)
	if err != nil {
		return err
	}
	if err := t.deleteKeyPrefix(k3.WithoutName()); err != nil {
		return err
	}

	// Remove the blockmap of the folder
	k4, err := db.keyer.GenerateBlockMapKey(nil, folder, nil, nil)
	if err != nil {
		return err
	}
	if err := t.deleteKeyPrefix(k4.WithoutHashAndName()); err != nil {
		return err
	}

	return t.commit()
}

func (db *Lowlevel) dropDeviceFolder(device, folder []byte, meta *metadataTracker) error {
	t, err := db.newReadWriteTransaction()
	if err != nil {
		return err
	}
	defer t.close()

	key, err := db.keyer.GenerateDeviceFileKey(nil, folder, device, nil)
	if err != nil {
		return err
	}
	dbi, err := t.NewPrefixIterator(key)
	if err != nil {
		return err
	}
	var gk, keyBuf []byte
	for dbi.Next() {
		name := db.keyer.NameFromDeviceFileKey(dbi.Key())
		gk, err = db.keyer.GenerateGlobalVersionKey(gk, folder, name)
		if err != nil {
			return err
		}
		keyBuf, err = t.removeFromGlobal(gk, keyBuf, folder, device, name, meta)
		if err != nil {
			return err
		}
		if err := t.Delete(dbi.Key()); err != nil {
			return err
		}
		if err := t.Checkpoint(); err != nil {
			return err
		}
	}
	if err := dbi.Error(); err != nil {
		return err
	}
	dbi.Release()

	if bytes.Equal(device, protocol.LocalDeviceID[:]) {
		key, err := db.keyer.GenerateBlockMapKey(nil, folder, nil, nil)
		if err != nil {
			return err
		}
		if err := t.deleteKeyPrefix(key.WithoutHashAndName()); err != nil {
			return err
		}
	}
	return t.commit()
}

func (db *Lowlevel) checkGlobals(folder []byte, meta *metadataTracker) error {
	t, err := db.newReadWriteTransaction()
	if err != nil {
		return err
	}
	defer t.close()

	key, err := db.keyer.GenerateGlobalVersionKey(nil, folder, nil)
	if err != nil {
		return err
	}
	dbi, err := t.NewPrefixIterator(key.WithoutName())
	if err != nil {
		return err
	}
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
			dk, err = db.keyer.GenerateDeviceFileKey(dk, folder, version.Device, name)
			if err != nil {
				return err
			}
			_, err := t.Get(dk)
			if backend.IsNotFound(err) {
				continue
			}
			if err != nil {
				return err
			}
			newVL.Versions = append(newVL.Versions, version)

			if i == 0 {
				if fi, ok, err := t.getFileByKey(dk); err != nil {
					return err
				} else if ok {
					meta.addFile(protocol.GlobalDeviceID, fi)
				}
			}
		}

		if len(newVL.Versions) != len(vl.Versions) {
			if err := t.Put(dbi.Key(), mustMarshal(&newVL)); err != nil {
				return err
			}
		}
	}
	if err := dbi.Error(); err != nil {
		return err
	}

	l.Debugf("db check completed for %q", folder)
	return t.commit()
}

func (db *Lowlevel) getIndexID(device, folder []byte) (protocol.IndexID, error) {
	key, err := db.keyer.GenerateIndexIDKey(nil, device, folder)
	if err != nil {
		return 0, err
	}
	cur, err := db.Get(key)
	if backend.IsNotFound(err) {
		return 0, nil
	} else if err != nil {
		return 0, err
	}

	var id protocol.IndexID
	if err := id.Unmarshal(cur); err != nil {
		return 0, nil
	}

	return id, nil
}

func (db *Lowlevel) setIndexID(device, folder []byte, id protocol.IndexID) error {
	bs, _ := id.Marshal() // marshalling can't fail
	key, err := db.keyer.GenerateIndexIDKey(nil, device, folder)
	if err != nil {
		return err
	}
	return db.Put(key, bs)
}

func (db *Lowlevel) dropMtimes(folder []byte) error {
	key, err := db.keyer.GenerateMtimesKey(nil, folder)
	if err != nil {
		return err
	}
	return db.dropPrefix(key)
}

func (db *Lowlevel) dropFolderMeta(folder []byte) error {
	key, err := db.keyer.GenerateFolderMetaKey(nil, folder)
	if err != nil {
		return err
	}
	return db.dropPrefix(key)
}

func (db *Lowlevel) dropPrefix(prefix []byte) error {
	t, err := db.newReadWriteTransaction()
	if err != nil {
		return err
	}
	defer t.close()

	if err := t.deleteKeyPrefix(prefix); err != nil {
		return err
	}
	return t.commit()
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

// unchanged checks if two files are the same and thus don't need to be updated.
// Local flags or the invalid bit might change without the version
// being bumped.
func unchanged(nf, ef FileIntf) bool {
	return ef.FileVersion().Equal(nf.FileVersion()) && ef.IsInvalid() == nf.IsInvalid() && ef.FileLocalFlags() == nf.FileLocalFlags()
}
