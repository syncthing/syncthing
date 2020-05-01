// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package db

import (
	"bytes"
	"errors"

	"github.com/syncthing/syncthing/lib/db/backend"
	"github.com/syncthing/syncthing/lib/protocol"
)

var errEntryFromGlobalMissing = errors.New("device present in global list but missing as device/fileinfo entry")

// A readOnlyTransaction represents a database snapshot.
type readOnlyTransaction struct {
	backend.ReadTransaction
	keyer keyer
}

func (db *Lowlevel) newReadOnlyTransaction() (readOnlyTransaction, error) {
	tran, err := db.NewReadTransaction()
	if err != nil {
		return readOnlyTransaction{}, err
	}
	return readOnlyTransaction{
		ReadTransaction: tran,
		keyer:           db.keyer,
	}, nil
}

func (t readOnlyTransaction) close() {
	t.Release()
}

func (t readOnlyTransaction) getFile(folder, device, file []byte) (protocol.FileInfo, bool, error) {
	key, err := t.keyer.GenerateDeviceFileKey(nil, folder, device, file)
	if err != nil {
		return protocol.FileInfo{}, false, err
	}
	return t.getFileByKey(key)
}

func (t readOnlyTransaction) getFileByKey(key []byte) (protocol.FileInfo, bool, error) {
	f, ok, err := t.getFileTrunc(key, false)
	if err != nil || !ok {
		return protocol.FileInfo{}, false, err
	}
	return f.(protocol.FileInfo), true, nil
}

func (t readOnlyTransaction) getFileTrunc(key []byte, trunc bool) (FileIntf, bool, error) {
	bs, err := t.Get(key)
	if backend.IsNotFound(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	f, err := t.unmarshalTrunc(bs, trunc)
	if backend.IsNotFound(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return f, true, nil
}

func (t readOnlyTransaction) unmarshalTrunc(bs []byte, trunc bool) (FileIntf, error) {
	if trunc {
		var tf FileInfoTruncated
		err := tf.Unmarshal(bs)
		if err != nil {
			return nil, err
		}
		return tf, nil
	}

	var fi protocol.FileInfo
	if err := fi.Unmarshal(bs); err != nil {
		return nil, err
	}
	if err := t.fillFileInfo(&fi); err != nil {
		return nil, err
	}
	return fi, nil
}

// fillFileInfo follows the (possible) indirection of blocks and fills it out.
func (t readOnlyTransaction) fillFileInfo(fi *protocol.FileInfo) error {
	var key []byte

	if len(fi.Blocks) == 0 && len(fi.BlocksHash) != 0 {
		// The blocks list is indirected and we need to load it.
		key = t.keyer.GenerateBlockListKey(key, fi.BlocksHash)
		bs, err := t.Get(key)
		if err != nil {
			return err
		}
		var bl BlockList
		if err := bl.Unmarshal(bs); err != nil {
			return err
		}
		fi.Blocks = bl.Blocks
	}

	return nil
}

func (t readOnlyTransaction) getGlobalVersions(keyBuf, folder, file []byte) (VersionList, error) {
	var err error
	keyBuf, err = t.keyer.GenerateGlobalVersionKey(keyBuf, folder, file)
	if err != nil {
		return VersionList{}, err
	}
	return t.getGlobalVersionsByKey(keyBuf)
}

func (t readOnlyTransaction) getGlobalVersionsByKey(key []byte) (VersionList, error) {
	bs, err := t.Get(key)
	if err != nil {
		return VersionList{}, err
	}

	var vl VersionList
	if err := vl.Unmarshal(bs); err != nil {
		return VersionList{}, err
	}

	return vl, nil
}

func (t readOnlyTransaction) getGlobal(keyBuf, folder, file []byte, truncate bool) ([]byte, FileIntf, bool, error) {
	vl, err := t.getGlobalVersions(keyBuf, folder, file)
	if backend.IsNotFound(err) {
		return keyBuf, nil, false, nil
	} else if err != nil {
		return nil, nil, false, err
	}
	if len(vl.Versions) == 0 {
		return nil, nil, false, nil
	}

	keyBuf, err = t.keyer.GenerateDeviceFileKey(keyBuf, folder, vl.Versions[0].Device, file)
	if err != nil {
		return nil, nil, false, err
	}
	fi, ok, err := t.getFileTrunc(keyBuf, truncate)
	if err != nil || !ok {
		return keyBuf, nil, false, err
	}
	return keyBuf, fi, true, nil
}

func (t *readOnlyTransaction) withHave(folder, device, prefix []byte, truncate bool, fn Iterator) error {
	if len(prefix) > 0 {
		unslashedPrefix := prefix
		if bytes.HasSuffix(prefix, []byte{'/'}) {
			unslashedPrefix = unslashedPrefix[:len(unslashedPrefix)-1]
		} else {
			prefix = append(prefix, '/')
		}

		key, err := t.keyer.GenerateDeviceFileKey(nil, folder, device, unslashedPrefix)
		if err != nil {
			return err
		}
		if f, ok, err := t.getFileTrunc(key, true); err != nil {
			return err
		} else if ok && !fn(f) {
			return nil
		}
	}

	key, err := t.keyer.GenerateDeviceFileKey(nil, folder, device, prefix)
	if err != nil {
		return err
	}
	dbi, err := t.NewPrefixIterator(key)
	if err != nil {
		return err
	}
	defer dbi.Release()

	for dbi.Next() {
		name := t.keyer.NameFromDeviceFileKey(dbi.Key())
		if len(prefix) > 0 && !bytes.HasPrefix(name, prefix) {
			return nil
		}

		f, err := t.unmarshalTrunc(dbi.Value(), truncate)
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

func (t *readOnlyTransaction) withHaveSequence(folder []byte, startSeq int64, fn Iterator) error {
	first, err := t.keyer.GenerateSequenceKey(nil, folder, startSeq)
	if err != nil {
		return err
	}
	last, err := t.keyer.GenerateSequenceKey(nil, folder, maxInt64)
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
			l.Debugln("missing file for sequence number", t.keyer.SequenceFromSequenceKey(dbi.Key()))
			continue
		}

		if shouldDebug() {
			if seq := t.keyer.SequenceFromSequenceKey(dbi.Key()); f.Sequence != seq {
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

func (t *readOnlyTransaction) withGlobal(folder, prefix []byte, truncate bool, fn Iterator) error {
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

	key, err := t.keyer.GenerateGlobalVersionKey(nil, folder, prefix)
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
		name := t.keyer.NameFromGlobalVersionKey(dbi.Key())
		if len(prefix) > 0 && !bytes.HasPrefix(name, prefix) {
			return nil
		}

		var vl VersionList
		if err := vl.Unmarshal(dbi.Value()); err != nil {
			return err
		}

		dk, err = t.keyer.GenerateDeviceFileKey(dk, folder, vl.Versions[0].Device, name)
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

func (t *readOnlyTransaction) availability(folder, file []byte) ([]protocol.DeviceID, error) {
	vl, err := t.getGlobalVersions(nil, folder, file)
	if backend.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
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

func (t *readOnlyTransaction) withNeed(folder, device []byte, truncate bool, fn Iterator) error {
	if bytes.Equal(device, protocol.LocalDeviceID[:]) {
		return t.withNeedLocal(folder, truncate, fn)
	}

	key, err := t.keyer.GenerateGlobalVersionKey(nil, folder, nil)
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
		var vl VersionList
		if err := vl.Unmarshal(dbi.Value()); err != nil {
			return err
		}

		globalFV := vl.Versions[0]
		haveFV, have := vl.Get(device)

		if !need(globalFV, have, haveFV.Version) {
			continue
		}
		name := t.keyer.NameFromGlobalVersionKey(dbi.Key())
		dk, err = t.keyer.GenerateDeviceFileKey(dk, folder, globalFV.Device, name)
		if err != nil {
			return err
		}
		gf, ok, err := t.getFileTrunc(dk, truncate)
		if err != nil {
			return err
		}
		if !ok {
			return errEntryFromGlobalMissing
		}
		l.Debugf("need folder=%q device=%v name=%q have=%v invalid=%v haveV=%v globalV=%v globalDev=%v", folder, devID, name, have, haveFV.Invalid, haveFV.Version, globalFV.Version, globalFV.Device)
		if !fn(gf) {
			return dbi.Error()
		}
	}
	return dbi.Error()
}

func (t *readOnlyTransaction) withNeedLocal(folder []byte, truncate bool, fn Iterator) error {
	key, err := t.keyer.GenerateNeedFileKey(nil, folder, nil)
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
		keyBuf, f, ok, err = t.getGlobal(keyBuf, folder, t.keyer.NameFromGlobalVersionKey(dbi.Key()), truncate)
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

// A readWriteTransaction is a readOnlyTransaction plus a batch for writes.
// The batch will be committed on close() or by checkFlush() if it exceeds the
// batch size.
type readWriteTransaction struct {
	backend.WriteTransaction
	readOnlyTransaction
}

func (db *Lowlevel) newReadWriteTransaction() (readWriteTransaction, error) {
	tran, err := db.NewWriteTransaction()
	if err != nil {
		return readWriteTransaction{}, err
	}
	return readWriteTransaction{
		WriteTransaction: tran,
		readOnlyTransaction: readOnlyTransaction{
			ReadTransaction: tran,
			keyer:           db.keyer,
		},
	}, nil
}

func (t readWriteTransaction) Commit() error {
	t.readOnlyTransaction.close()
	return t.WriteTransaction.Commit()
}

func (t readWriteTransaction) close() {
	t.readOnlyTransaction.close()
	t.WriteTransaction.Release()
}

// putFile stores a file in the database, taking care of indirected fields.
// Set the truncated flag when putting a file that deliberatly can have an
// empty block list but a non-empty block list hash. This should normally be
// false.
func (t readWriteTransaction) putFile(fkey []byte, fi protocol.FileInfo, truncated bool) error {
	var bkey []byte

	// Always set the blocks hash when there are blocks. Leave the blocks
	// hash alone when there are no blocks and we might be putting a
	// "truncated" FileInfo (no blocks, but the hash reference is live).
	if len(fi.Blocks) > 0 {
		fi.BlocksHash = protocol.BlocksHash(fi.Blocks)
	} else if !truncated {
		fi.BlocksHash = nil
	}

	// Indirect the blocks if the block list is large enough.
	if len(fi.Blocks) > blocksIndirectionCutoff {
		bkey = t.keyer.GenerateBlockListKey(bkey, fi.BlocksHash)
		if _, err := t.Get(bkey); backend.IsNotFound(err) {
			// Marshal the block list and save it
			blocksBs := mustMarshal(&BlockList{Blocks: fi.Blocks})
			if err := t.Put(bkey, blocksBs); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		fi.Blocks = nil
	}

	fiBs := mustMarshal(&fi)
	return t.Put(fkey, fiBs)
}

// updateGlobal adds this device+version to the version list for the given
// file. If the device is already present in the list, the version is updated.
// If the file does not have an entry in the global list, it is created.
func (t readWriteTransaction) updateGlobal(gk, keyBuf, folder, device []byte, file protocol.FileInfo, meta *metadataTracker) ([]byte, bool, error) {
	deviceID := protocol.DeviceIDFromBytes(device)

	l.Debugf("update global; folder=%q device=%v file=%q version=%v invalid=%v", folder, deviceID, file.Name, file.Version, file.IsInvalid())

	fl, err := t.getGlobalVersionsByKey(gk)
	if err != nil && !backend.IsNotFound(err) {
		return nil, false, err
	}

	fl, removedFV, removedAt, insertedAt, err := fl.update(folder, device, file, t.readOnlyTransaction)
	if err != nil {
		return nil, false, err
	}

	name := []byte(file.Name)

	l.Debugf(`new global for "%v" after update: %v`, file.Name, fl)
	if err := t.Put(gk, mustMarshal(&fl)); err != nil {
		return nil, false, err
	}

	// Only load those from db if actually needed

	var gotGlobal, gotOldGlobal bool
	var global, oldGlobal FileIntf

	globalFV := fl.Versions[0]
	var oldGlobalFV FileVersion
	haveOldGlobal := false

	globalUnaffected := removedAt != 0 && insertedAt != 0
	if globalUnaffected {
		oldGlobalFV = globalFV
		haveOldGlobal = true
	} else {
		if removedAt == 0 {
			oldGlobalFV = removedFV
			haveOldGlobal = true
		} else if len(fl.Versions) > 1 {
			// The previous newest version is now at index 1
			oldGlobalFV = fl.Versions[1]
			haveOldGlobal = true
		}
	}

	// Check the need of the device that was updated
	// Must happen before updating global meta: If this is the first
	// item from this device, it will be initialized with the global state.

	needBefore := false
	if haveOldGlobal {
		needBefore = need(oldGlobalFV, removedAt >= 0, removedFV.Version)
	}
	needNow := need(globalFV, true, fl.Versions[insertedAt].Version)
	if needBefore {
		if !gotOldGlobal {
			if oldGlobal, err = t.updateGlobalGetOldGlobal(keyBuf, folder, name, oldGlobalFV); err != nil {
				return nil, false, err
			}
			gotOldGlobal = true
		}
		meta.removeNeeded(deviceID, oldGlobal)
		if !needNow && bytes.Equal(device, protocol.LocalDeviceID[:]) {
			if keyBuf, err = t.updateLocalNeed(keyBuf, folder, name, false); err != nil {
				return nil, false, err
			}
		}
	}
	if needNow {
		if !gotGlobal {
			if global, err = t.updateGlobalGetGlobal(keyBuf, folder, name, file, insertedAt, fl); err != nil {
				return nil, false, err
			}
			gotGlobal = true
		}
		meta.addNeeded(deviceID, global)
		if !needBefore && bytes.Equal(device, protocol.LocalDeviceID[:]) {
			if keyBuf, err = t.updateLocalNeed(keyBuf, folder, name, true); err != nil {
				return nil, false, err
			}
		}
	}

	// Update global size counter if necessary
	// Necessary here means the first item in the global list was changed,
	// even if both new and old are invalid, due to potential change in
	// LocalFlags.

	if !globalUnaffected {
		if global, err = t.updateGlobalGetGlobal(keyBuf, folder, name, file, insertedAt, fl); err != nil {
			return nil, false, err
		}
		gotGlobal = true
		if haveOldGlobal {
			if oldGlobal, err = t.updateGlobalGetOldGlobal(keyBuf, folder, name, oldGlobalFV); err != nil {
				return nil, false, err
			}
			gotOldGlobal = true
			// Remove the old global from the global size counter
			meta.removeFile(protocol.GlobalDeviceID, oldGlobal)
		}

		// Add the new global to the global size counter
		meta.addFile(protocol.GlobalDeviceID, global)
	}

	if globalUnaffected {
		// Neither the global state nor the needs of any devices, except
		// the one updated, changed.
		return keyBuf, true, nil
	}

	// If global changed, but both the new and old are invalid, noone needed
	// the file before and now -> nothing to do.
	if global.IsInvalid() && (!haveOldGlobal || oldGlobal.IsInvalid()) {
		return keyBuf, true, nil
	}

	// check for local (if not already done before)
	if !bytes.Equal(device, protocol.LocalDeviceID[:]) {
		localFV, haveLocal := fl.Get(protocol.LocalDeviceID[:])
		needBefore := false
		if haveOldGlobal {
			needBefore = need(oldGlobalFV, haveLocal, localFV.Version)
		}
		needNow := need(globalFV, haveLocal, localFV.Version)
		if needBefore {
			meta.removeNeeded(protocol.LocalDeviceID, oldGlobal)
			if !needNow {
				if keyBuf, err = t.updateLocalNeed(keyBuf, folder, name, false); err != nil {
					return nil, false, err
				}
			}
		}
		if need(globalFV, haveLocal, localFV.Version) {
			meta.addNeeded(protocol.LocalDeviceID, global)
			if !needBefore {
				if keyBuf, err = t.updateLocalNeed(keyBuf, folder, name, true); err != nil {
					return nil, false, err
				}
			}
		}
	}

	for _, dev := range meta.devices() {
		if bytes.Equal(dev[:], device) {
			// Already handled above
			continue
		}
		fv, have := fl.Get(dev[:])
		if haveOldGlobal && need(oldGlobalFV, have, fv.Version) {
			meta.removeNeeded(dev, oldGlobal)
		}
		if need(globalFV, have, fv.Version) {
			meta.addNeeded(dev, global)
		}
	}

	return keyBuf, true, nil
}

func (t readWriteTransaction) updateGlobalGetGlobal(keyBuf, folder, name []byte, file protocol.FileInfo, insertedAt int, fl VersionList) (FileIntf, error) {
	if insertedAt == 0 {
		// Inserted a new newest version
		return file, nil
	}
	var err error
	keyBuf, err = t.keyer.GenerateDeviceFileKey(keyBuf, folder, fl.Versions[0].Device, name)
	if err != nil {
		return nil, err
	}
	global, ok, err := t.getFileTrunc(keyBuf, true)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errEntryFromGlobalMissing
	}
	return global, nil
}

func (t readWriteTransaction) updateGlobalGetOldGlobal(keyBuf, folder, name []byte, oldGlobalFV FileVersion) (FileIntf, error) {
	var err error
	keyBuf, err = t.keyer.GenerateDeviceFileKey(keyBuf, folder, oldGlobalFV.Device, name)
	if err != nil {
		return nil, err
	}
	oldGlobal, ok, err := t.getFileTrunc(keyBuf, true)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errEntryFromGlobalMissing
	}
	return oldGlobal, nil
}

func (t readWriteTransaction) updateLocalNeed(keyBuf, folder, name []byte, add bool) ([]byte, error) {
	var err error
	keyBuf, err = t.keyer.GenerateNeedFileKey(keyBuf, folder, name)
	if err != nil {
		return nil, err
	}
	if add {
		l.Debugf("local need insert; folder=%q, name=%q", folder, name)
		err = t.Put(keyBuf, nil)
	} else {
		l.Debugf("local need delete; folder=%q, name=%q", folder, name)
		err = t.Delete(keyBuf)
	}
	return keyBuf, err
}

func need(global FileVersion, haveLocal bool, localVersion protocol.Vector) bool {
	// We never need an invalid file.
	if global.Invalid {
		return false
	}
	// We don't need a deleted file if we don't have it.
	if global.Deleted && !haveLocal {
		return false
	}
	// We don't need the global file if we already have the same version.
	if haveLocal && localVersion.GreaterEqual(global.Version) {
		return false
	}
	return true
}

// removeFromGlobal removes the device from the global version list for the
// given file. If the version list is empty after this, the file entry is
// removed entirely.
func (t readWriteTransaction) removeFromGlobal(gk, keyBuf, folder, device []byte, file []byte, meta *metadataTracker) ([]byte, error) {
	deviceID := protocol.DeviceIDFromBytes(device)

	l.Debugf("remove from global; folder=%q device=%v file=%q", folder, deviceID, file)

	fl, err := t.getGlobalVersionsByKey(gk)
	if backend.IsNotFound(err) {
		// We might be called to "remove" a global version that doesn't exist
		// if the first update for the file is already marked invalid.
		return keyBuf, nil
	} else if err != nil {
		return nil, err
	}

	fl, removedFV, removedAt := fl.pop(device)
	if removedAt == -1 {
		// There is no version for the given device
		return keyBuf, nil
	}

	if removedAt != 0 {
		l.Debugf("new global after remove: %v", fl)
		if err := t.Put(gk, mustMarshal(&fl)); err != nil {
			return nil, err
		}
	}

	keyBuf, err = t.keyer.GenerateDeviceFileKey(keyBuf, folder, device, file)
	if err != nil {
		return nil, err
	}
	f, ok, err := t.getFileTrunc(keyBuf, true)
	if err != nil {
		return nil, err
	} else if !ok {
		return nil, errEntryFromGlobalMissing
	}
	meta.removeFile(protocol.GlobalDeviceID, f)

	if fv, have := fl.Get(protocol.LocalDeviceID[:]); need(removedFV, have, fv.Version) {
		meta.removeNeeded(protocol.LocalDeviceID, f)
		if keyBuf, err = t.updateLocalNeed(keyBuf, folder, file, false); err != nil {
			return nil, err
		}
	}
	for _, dev := range meta.devices() {
		if bytes.Equal(dev[:], device) {
			continue
		}
		if fv, have := fl.Get(dev[:]); need(removedFV, have, fv.Version) {
			meta.removeNeeded(deviceID, f)
		}
	}

	if len(fl.Versions) == 0 {
		if err := t.Delete(gk); err != nil {
			return nil, err
		}
		return keyBuf, nil
	}

	globalFV := fl.Versions[0]
	keyBuf, err = t.keyer.GenerateDeviceFileKey(keyBuf, folder, globalFV.Device, file)
	if err != nil {
		return nil, err
	}
	global, ok, err := t.getFileTrunc(keyBuf, true)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errEntryFromGlobalMissing
	}
	meta.addFile(protocol.GlobalDeviceID, global)

	if !globalFV.Invalid {
		if fv, have := fl.Get(protocol.LocalDeviceID[:]); need(globalFV, have, fv.Version) {
			meta.addNeeded(deviceID, global)
			if keyBuf, err = t.updateLocalNeed(keyBuf, folder, file, true); err != nil {
				return nil, err
			}
		}
		for _, dev := range meta.devices() {
			if fv, have := fl.Get(dev[:]); need(globalFV, have, fv.Version) {
				meta.addNeeded(deviceID, global)
			}
		}
	}

	l.Debugf("new global after remove: %v", fl)
	if err := t.Put(gk, mustMarshal(&fl)); err != nil {
		return nil, err
	}

	return keyBuf, nil
}

func (t readWriteTransaction) deleteKeyPrefix(prefix []byte) error {
	dbi, err := t.NewPrefixIterator(prefix)
	if err != nil {
		return err
	}
	defer dbi.Release()
	for dbi.Next() {
		if err := t.Delete(dbi.Key()); err != nil {
			return err
		}
	}
	return dbi.Error()
}

func (t *readWriteTransaction) withAllFolderTruncated(folder []byte, fn func(device []byte, f FileInfoTruncated) bool) error {
	key, err := t.keyer.GenerateDeviceFileKey(nil, folder, nil, nil)
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
		device, ok := t.keyer.DeviceFromDeviceFileKey(dbi.Key())
		if !ok {
			// Not having the device in the index is bad. Clear it.
			if err := t.Delete(dbi.Key()); err != nil {
				return err
			}
			continue
		}

		intf, err := t.unmarshalTrunc(dbi.Value(), true)
		if err != nil {
			return err
		}
		f := intf.(FileInfoTruncated)

		switch f.Name {
		case "", ".", "..", "/": // A few obviously invalid filenames
			l.Infof("Dropping invalid filename %q from database", f.Name)
			name := []byte(f.Name)
			gk, err = t.keyer.GenerateGlobalVersionKey(gk, folder, name)
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
	return dbi.Error()
}

type marshaller interface {
	Marshal() ([]byte, error)
}

func mustMarshal(f marshaller) []byte {
	bs, err := f.Marshal()
	if err != nil {
		panic(err)
	}
	return bs
}
