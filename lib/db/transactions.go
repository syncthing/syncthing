// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package db

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/syncthing/syncthing/lib/db/backend"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/protocol"
)

var (
	errEntryFromGlobalMissing = errors.New("device present in global list but missing as device/fileinfo entry")
	errEmptyGlobal            = errors.New("no versions in global list")
	errEmptyFileVersion       = errors.New("no devices in global file version")
)

// A readOnlyTransaction represents a database snapshot.
type readOnlyTransaction struct {
	backend.ReadTransaction
	keyer    keyer
	evLogger events.Logger
}

func (db *Lowlevel) newReadOnlyTransaction() (readOnlyTransaction, error) {
	tran, err := db.NewReadTransaction()
	if err != nil {
		return readOnlyTransaction{}, err
	}
	return readOnlyTransaction{
		ReadTransaction: tran,
		keyer:           db.keyer,
		evLogger:        db.evLogger,
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

func (t readOnlyTransaction) getFileTrunc(key []byte, trunc bool) (protocol.FileIntf, bool, error) {
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

func (t readOnlyTransaction) unmarshalTrunc(bs []byte, trunc bool) (protocol.FileIntf, error) {
	if trunc {
		var tf FileInfoTruncated
		err := tf.Unmarshal(bs)
		if err != nil {
			return nil, err
		}
		if err := t.fillTruncated(&tf); err != nil {
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

type blocksIndirectionError struct {
	err error
}

func (e *blocksIndirectionError) Error() string {
	return fmt.Sprintf("filling Blocks: %v", e.err)
}

func (e *blocksIndirectionError) Unwrap() error {
	return e.err
}

// fillFileInfo follows the (possible) indirection of blocks and version
// vector and fills it out.
func (t readOnlyTransaction) fillFileInfo(fi *protocol.FileInfo) error {
	var key []byte

	if len(fi.Blocks) == 0 && len(fi.BlocksHash) != 0 {
		// The blocks list is indirected and we need to load it.
		key = t.keyer.GenerateBlockListKey(key, fi.BlocksHash)
		bs, err := t.Get(key)
		if err != nil {
			return &blocksIndirectionError{err}
		}
		var bl BlockList
		if err := bl.Unmarshal(bs); err != nil {
			return err
		}
		fi.Blocks = bl.Blocks
	}

	if len(fi.VersionHash) != 0 {
		key = t.keyer.GenerateVersionKey(key, fi.VersionHash)
		bs, err := t.Get(key)
		if err != nil {
			return fmt.Errorf("filling Version: %w", err)
		}
		var v protocol.Vector
		if err := v.Unmarshal(bs); err != nil {
			return err
		}
		fi.Version = v
	}

	return nil
}

// fillTruncated follows the (possible) indirection of version vector and
// fills it.
func (t readOnlyTransaction) fillTruncated(fi *FileInfoTruncated) error {
	var key []byte

	if len(fi.VersionHash) == 0 {
		return nil
	}

	key = t.keyer.GenerateVersionKey(key, fi.VersionHash)
	bs, err := t.Get(key)
	if err != nil {
		return err
	}
	var v protocol.Vector
	if err := v.Unmarshal(bs); err != nil {
		return err
	}
	fi.Version = v
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

func (t readOnlyTransaction) getGlobal(keyBuf, folder, file []byte, truncate bool) ([]byte, protocol.FileIntf, bool, error) {
	vl, err := t.getGlobalVersions(keyBuf, folder, file)
	if backend.IsNotFound(err) {
		return keyBuf, nil, false, nil
	} else if err != nil {
		return nil, nil, false, err
	}
	var fi protocol.FileIntf
	keyBuf, fi, _, err = t.getGlobalFromVersionList(keyBuf, folder, file, truncate, vl)
	return keyBuf, fi, true, err
}

func (t readOnlyTransaction) getGlobalFromVersionList(keyBuf, folder, file []byte, truncate bool, vl VersionList) ([]byte, protocol.FileIntf, FileVersion, error) {
	fv, ok := vl.GetGlobal()
	if !ok {
		return keyBuf, nil, FileVersion{}, errEmptyGlobal
	}
	keyBuf, fi, err := t.getGlobalFromFileVersion(keyBuf, folder, file, truncate, fv)
	return keyBuf, fi, fv, err
}

func (t readOnlyTransaction) getGlobalFromFileVersion(keyBuf, folder, file []byte, truncate bool, fv FileVersion) ([]byte, protocol.FileIntf, error) {
	dev, ok := fv.FirstDevice()
	if !ok {
		return keyBuf, nil, errEmptyFileVersion
	}
	keyBuf, err := t.keyer.GenerateDeviceFileKey(keyBuf, folder, dev, file)
	if err != nil {
		return keyBuf, nil, err
	}
	fi, ok, err := t.getFileTrunc(keyBuf, truncate)
	if err != nil {
		return keyBuf, nil, err
	}
	if !ok {
		return keyBuf, nil, errEntryFromGlobalMissing
	}
	return keyBuf, fi, nil
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
		if f, ok, err := t.getFileTrunc(key, truncate); err != nil {
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
				l.Debugf("Sequence index corruption (folder %v, file %v): sequence %d != expected %d", string(folder), f.Name, f.Sequence, seq)
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

		var f protocol.FileIntf
		dk, f, _, err = t.getGlobalFromVersionList(dk, folder, name, truncate, vl)
		if err != nil {
			return err
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

func (t *readOnlyTransaction) withBlocksHash(folder, hash []byte, iterator Iterator) error {
	key, err := t.keyer.GenerateBlockListMapKey(nil, folder, hash, nil)
	if err != nil {
		return err
	}

	iter, err := t.NewPrefixIterator(key)
	if err != nil {
		return err
	}
	defer iter.Release()

	for iter.Next() {
		file := string(t.keyer.NameFromBlockListMapKey(iter.Key()))
		f, ok, err := t.getFile(folder, protocol.LocalDeviceID[:], []byte(osutil.NormalizedFilename(file)))
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		f.Name = osutil.NativeFilename(f.Name)

		if !bytes.Equal(f.BlocksHash, hash) {
			msg := "Mismatching block map list hashes"
			t.evLogger.Log(events.Failure, fmt.Sprintln(msg, "in withBlocksHash"))
			l.Warnf("%v: got %x expected %x", msg, f.BlocksHash, hash)
			continue
		}

		if f.IsDeleted() || f.IsInvalid() || f.IsDirectory() || f.IsSymlink() {
			msg := "Found something of unexpected type in block list map"
			t.evLogger.Log(events.Failure, fmt.Sprintln(msg, "in withBlocksHash"))
			l.Warnf("%v: %s", msg, f)
			continue
		}

		if !iterator(f) {
			break
		}
	}

	return iter.Error()
}

func (t *readOnlyTransaction) availability(folder, file []byte) ([]protocol.DeviceID, error) {
	vl, err := t.getGlobalVersions(nil, folder, file)
	if backend.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	fv, ok := vl.GetGlobal()
	if !ok {
		return nil, nil
	}
	devices := make([]protocol.DeviceID, len(fv.Devices))
	for i, dev := range fv.Devices {
		n, err := protocol.DeviceIDFromBytes(dev)
		if err != nil {
			return nil, err
		}
		devices[i] = n
	}

	return devices, nil
}

func (t *readOnlyTransaction) withNeed(folder, device []byte, truncate bool, fn Iterator) error {
	if bytes.Equal(device, protocol.LocalDeviceID[:]) {
		return t.withNeedLocal(folder, truncate, fn)
	}
	return t.withNeedIteratingGlobal(folder, device, truncate, fn)
}

func (t *readOnlyTransaction) withNeedIteratingGlobal(folder, device []byte, truncate bool, fn Iterator) error {
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
	devID, err := protocol.DeviceIDFromBytes(device)
	if err != nil {
		return err
	}
	for dbi.Next() {
		var vl VersionList
		if err := vl.Unmarshal(dbi.Value()); err != nil {
			return err
		}

		globalFV, ok := vl.GetGlobal()
		if !ok {
			return errEmptyGlobal
		}
		haveFV, have := vl.Get(device)

		if !Need(globalFV, have, haveFV.Version) {
			continue
		}

		name := t.keyer.NameFromGlobalVersionKey(dbi.Key())
		var gf protocol.FileIntf
		dk, gf, err = t.getGlobalFromFileVersion(dk, folder, name, truncate, globalFV)
		if err != nil {
			return err
		}

		if shouldDebug() {
			if globalDev, ok := globalFV.FirstDevice(); ok {
				globalID, _ := protocol.DeviceIDFromBytes(globalDev)
				l.Debugf("need folder=%q device=%v name=%q have=%v invalid=%v haveV=%v haveDeleted=%v globalV=%v globalDeleted=%v globalDev=%v", folder, devID, name, have, haveFV.IsInvalid(), haveFV.Version, haveFV.Deleted, gf.FileVersion(), globalFV.Deleted, globalID)
			}
		}
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
	var f protocol.FileIntf
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
	indirectionTracker
}

type indirectionTracker interface {
	recordIndirectionHashesForFile(f *protocol.FileInfo)
}

func (db *Lowlevel) newReadWriteTransaction(hooks ...backend.CommitHook) (readWriteTransaction, error) {
	tran, err := db.NewWriteTransaction(hooks...)
	if err != nil {
		return readWriteTransaction{}, err
	}
	return readWriteTransaction{
		WriteTransaction: tran,
		readOnlyTransaction: readOnlyTransaction{
			ReadTransaction: tran,
			keyer:           db.keyer,
		},
		indirectionTracker: db,
	}, nil
}

func (t readWriteTransaction) Commit() error {
	// The readOnlyTransaction must close after commit, because they may be
	// backed by the same actual lower level transaction.
	defer t.readOnlyTransaction.close()
	return t.WriteTransaction.Commit()
}

func (t readWriteTransaction) close() {
	t.readOnlyTransaction.close()
	t.WriteTransaction.Release()
}

// putFile stores a file in the database, taking care of indirected fields.
func (t readWriteTransaction) putFile(fkey []byte, fi protocol.FileInfo) error {
	var bkey []byte

	// Always set the blocks hash when there are blocks.
	if len(fi.Blocks) > 0 {
		fi.BlocksHash = protocol.BlocksHash(fi.Blocks)
	} else {
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

	// Indirect the version vector if it's large enough.
	if len(fi.Version.Counters) > versionIndirectionCutoff {
		fi.VersionHash = protocol.VectorHash(fi.Version)
		bkey = t.keyer.GenerateVersionKey(bkey, fi.VersionHash)
		if _, err := t.Get(bkey); backend.IsNotFound(err) {
			// Marshal the version vector and save it
			versionBs := mustMarshal(&fi.Version)
			if err := t.Put(bkey, versionBs); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		fi.Version = protocol.Vector{}
	} else {
		fi.VersionHash = nil
	}

	t.indirectionTracker.recordIndirectionHashesForFile(&fi)

	fiBs := mustMarshal(&fi)
	return t.Put(fkey, fiBs)
}

// updateGlobal adds this device+version to the version list for the given
// file. If the device is already present in the list, the version is updated.
// If the file does not have an entry in the global list, it is created.
func (t readWriteTransaction) updateGlobal(gk, keyBuf, folder, device []byte, file protocol.FileInfo, meta *metadataTracker) ([]byte, bool, error) {
	deviceID, err := protocol.DeviceIDFromBytes(device)
	if err != nil {
		return nil, false, err
	}

	l.Debugf("update global; folder=%q device=%v file=%q version=%v invalid=%v", folder, deviceID, file.Name, file.Version, file.IsInvalid())

	fl, err := t.getGlobalVersionsByKey(gk)
	if err != nil && !backend.IsNotFound(err) {
		return nil, false, err
	}

	globalFV, oldGlobalFV, removedFV, haveOldGlobal, haveRemoved, globalChanged, err := fl.update(folder, device, file, t.readOnlyTransaction)
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
	var global, oldGlobal protocol.FileIntf

	// Check the need of the device that was updated
	// Must happen before updating global meta: If this is the first
	// item from this device, it will be initialized with the global state.

	needBefore := haveOldGlobal && Need(oldGlobalFV, haveRemoved, removedFV.Version)
	needNow := Need(globalFV, true, file.Version)
	if needBefore {
		if keyBuf, oldGlobal, err = t.getGlobalFromFileVersion(keyBuf, folder, name, true, oldGlobalFV); err != nil {
			return nil, false, err
		}
		gotOldGlobal = true
		meta.removeNeeded(deviceID, oldGlobal)
		if !needNow && bytes.Equal(device, protocol.LocalDeviceID[:]) {
			if keyBuf, err = t.updateLocalNeed(keyBuf, folder, name, false); err != nil {
				return nil, false, err
			}
		}
	}
	if needNow {
		keyBuf, global, err = t.getGlobalFromFileVersion(keyBuf, folder, name, true, globalFV)
		if err != nil {
			return nil, false, err
		}
		gotGlobal = true
		meta.addNeeded(deviceID, global)
		if !needBefore && bytes.Equal(device, protocol.LocalDeviceID[:]) {
			if keyBuf, err = t.updateLocalNeed(keyBuf, folder, name, true); err != nil {
				return nil, false, err
			}
		}
	}

	// Update global size counter if necessary

	if !globalChanged {
		// Neither the global state nor the needs of any devices, except
		// the one updated, changed.
		return keyBuf, true, nil
	}

	// Remove the old global from the global size counter
	if haveOldGlobal {
		if !gotOldGlobal {
			if keyBuf, oldGlobal, err = t.getGlobalFromFileVersion(keyBuf, folder, name, true, oldGlobalFV); err != nil {
				return nil, false, err
			}
			gotOldGlobal = true
		}
		// Remove the old global from the global size counter
		meta.removeFile(protocol.GlobalDeviceID, oldGlobal)
	}

	// Add the new global to the global size counter
	if !gotGlobal {
		if globalFV.Version.Equal(file.Version) {
			// The inserted file is the global file
			global = file
		} else {
			keyBuf, global, err = t.getGlobalFromFileVersion(keyBuf, folder, name, true, globalFV)
		}
		gotGlobal = true
	}
	meta.addFile(protocol.GlobalDeviceID, global)

	// check for local (if not already done before)
	if !bytes.Equal(device, protocol.LocalDeviceID[:]) {
		localFV, haveLocal := fl.Get(protocol.LocalDeviceID[:])
		needBefore := haveOldGlobal && Need(oldGlobalFV, haveLocal, localFV.Version)
		needNow := Need(globalFV, haveLocal, localFV.Version)
		if needBefore {
			meta.removeNeeded(protocol.LocalDeviceID, oldGlobal)
			if !needNow {
				if keyBuf, err = t.updateLocalNeed(keyBuf, folder, name, false); err != nil {
					return nil, false, err
				}
			}
		}
		if needNow {
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
		if haveOldGlobal && Need(oldGlobalFV, have, fv.Version) {
			meta.removeNeeded(dev, oldGlobal)
		}
		if Need(globalFV, have, fv.Version) {
			meta.addNeeded(dev, global)
		}
	}

	return keyBuf, true, nil
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

func Need(global FileVersion, haveLocal bool, localVersion protocol.Vector) bool {
	// We never need an invalid file or a file without a valid version (just
	// another way of expressing "invalid", really, until we fix that
	// part...).
	if global.IsInvalid() || global.Version.IsEmpty() {
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
func (t readWriteTransaction) removeFromGlobal(gk, keyBuf, folder, device, file []byte, meta *metadataTracker) ([]byte, error) {
	deviceID, err := protocol.DeviceIDFromBytes(device)
	if err != nil {
		return nil, err
	}

	l.Debugf("remove from global; folder=%q device=%v file=%q", folder, deviceID, file)

	fl, err := t.getGlobalVersionsByKey(gk)
	if backend.IsNotFound(err) {
		// We might be called to "remove" a global version that doesn't exist
		// if the first update for the file is already marked invalid.
		return keyBuf, nil
	} else if err != nil {
		return nil, err
	}

	oldGlobalFV, haveOldGlobal := fl.GetGlobal()
	oldGlobalFV = oldGlobalFV.copy()

	if !haveOldGlobal {
		// Shouldn't ever happen, but doesn't hurt to handle.
		t.evLogger.Log(events.Failure, "encountered empty global while removing item")
		return keyBuf, t.Delete(gk)
	}

	removedFV, haveRemoved, globalChanged, err := fl.pop(device, file)
	if err != nil {
		return nil, err
	}
	if !haveRemoved {
		// There is no version for the given device
		return keyBuf, nil
	}

	var global protocol.FileIntf
	var gotGlobal bool

	globalFV, haveGlobal := fl.GetGlobal()
	// Add potential needs of the removed device
	if haveGlobal && !globalFV.IsInvalid() && Need(globalFV, false, protocol.Vector{}) && !Need(oldGlobalFV, haveRemoved, removedFV.Version) {
		keyBuf, global, _, err = t.getGlobalFromVersionList(keyBuf, folder, file, true, fl)
		if err != nil {
			return nil, err
		}
		gotGlobal = true
		meta.addNeeded(deviceID, global)
		if bytes.Equal(protocol.LocalDeviceID[:], device) {
			if keyBuf, err = t.updateLocalNeed(keyBuf, folder, file, true); err != nil {
				return nil, err
			}
		}
	}

	// Global hasn't changed, abort early
	if !globalChanged {
		l.Debugf("new global after remove: %v", fl)
		if err := t.Put(gk, mustMarshal(&fl)); err != nil {
			return nil, err
		}
		return keyBuf, nil
	}

	var oldGlobal protocol.FileIntf
	keyBuf, oldGlobal, err = t.getGlobalFromFileVersion(keyBuf, folder, file, true, oldGlobalFV)
	if err != nil {
		return nil, err
	}
	meta.removeFile(protocol.GlobalDeviceID, oldGlobal)

	// Remove potential device needs
	shouldRemoveNeed := func(dev protocol.DeviceID) bool {
		fv, have := fl.Get(dev[:])
		if !Need(oldGlobalFV, have, fv.Version) {
			return false // Didn't need it before
		}
		return !haveGlobal || !Need(globalFV, have, fv.Version)
	}
	if shouldRemoveNeed(protocol.LocalDeviceID) {
		meta.removeNeeded(protocol.LocalDeviceID, oldGlobal)
		if keyBuf, err = t.updateLocalNeed(keyBuf, folder, file, false); err != nil {
			return nil, err
		}
	}
	for _, dev := range meta.devices() {
		if bytes.Equal(dev[:], device) { // Was the previous global
			continue
		}
		if shouldRemoveNeed(dev) {
			meta.removeNeeded(dev, oldGlobal)
		}
	}

	// Nothing left, i.e. nothing to add to the global counter below.
	if fl.Empty() {
		if err := t.Delete(gk); err != nil {
			return nil, err
		}
		return keyBuf, nil
	}

	// Add to global
	if !gotGlobal {
		keyBuf, global, _, err = t.getGlobalFromVersionList(keyBuf, folder, file, true, fl)
		if err != nil {
			return nil, err
		}
	}
	meta.addFile(protocol.GlobalDeviceID, global)

	l.Debugf(`new global for "%s" after remove: %v`, file, fl)
	if err := t.Put(gk, mustMarshal(&fl)); err != nil {
		return nil, err
	}

	return keyBuf, nil
}

func (t readWriteTransaction) deleteKeyPrefix(prefix []byte) error {
	return t.deleteKeyPrefixMatching(prefix, func([]byte) bool { return true })
}

func (t readWriteTransaction) deleteKeyPrefixMatching(prefix []byte, match func(key []byte) bool) error {
	dbi, err := t.NewPrefixIterator(prefix)
	if err != nil {
		return err
	}
	defer dbi.Release()
	for dbi.Next() {
		if !match(dbi.Key()) {
			continue
		}
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
