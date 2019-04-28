// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package db

import (
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/util"
)

// A readOnlyTransaction represents a database snapshot.
type readOnlyTransaction struct {
	snapshot
	keyer keyer
}

func (db *instance) newReadOnlyTransaction() readOnlyTransaction {
	return readOnlyTransaction{
		snapshot: db.GetSnapshot(),
		keyer:    db.keyer,
	}
}

func (t readOnlyTransaction) close() {
	t.Release()
}

func (t readOnlyTransaction) getFile(folder, device, file []byte) (protocol.FileInfo, bool) {
	return t.getFileByKey(t.keyer.GenerateDeviceFileKey(nil, folder, device, file))
}

func (t readOnlyTransaction) getFileByKey(key []byte) (protocol.FileInfo, bool) {
	if f, ok := t.getFileTrunc(key, false); ok {
		return f.(protocol.FileInfo), true
	}
	return protocol.FileInfo{}, false
}

func (t readOnlyTransaction) getFileTrunc(key []byte, trunc bool) (FileIntf, bool) {
	bs, err := t.Get(key, nil)
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

func (t readOnlyTransaction) getGlobal(keyBuf, folder, file []byte, truncate bool) ([]byte, FileIntf, bool) {
	keyBuf = t.keyer.GenerateGlobalVersionKey(keyBuf, folder, file)

	bs, err := t.Get(keyBuf, nil)
	if err != nil {
		return keyBuf, nil, false
	}

	vl, ok := unmarshalVersionList(bs)
	if !ok {
		return keyBuf, nil, false
	}

	keyBuf = t.keyer.GenerateDeviceFileKey(keyBuf, folder, vl.Versions[0].Device, file)
	if fi, ok := t.getFileTrunc(keyBuf, truncate); ok {
		return keyBuf, fi, true
	}

	return keyBuf, nil, false
}

// A readWriteTransaction is a readOnlyTransaction plus a batch for writes.
// The batch will be committed on close() or by checkFlush() if it exceeds the
// batch size.
type readWriteTransaction struct {
	readOnlyTransaction
	*batch
}

func (db *instance) newReadWriteTransaction() readWriteTransaction {
	return readWriteTransaction{
		readOnlyTransaction: db.newReadOnlyTransaction(),
		batch:               db.newBatch(),
	}
}

func (t readWriteTransaction) close() {
	t.flush()
	t.readOnlyTransaction.close()
}

// updateGlobal adds this device+version to the version list for the given
// file. If the device is already present in the list, the version is updated.
// If the file does not have an entry in the global list, it is created.
func (t readWriteTransaction) updateGlobal(gk, keyBuf, folder, device []byte, file protocol.FileInfo, meta *metadataTracker) ([]byte, bool) {
	l.Debugf("update global; folder=%q device=%v file=%q version=%v invalid=%v", folder, protocol.DeviceIDFromBytes(device), file.Name, file.Version, file.IsInvalid())

	var fl VersionList
	if svl, err := t.Get(gk, nil); err == nil {
		fl.Unmarshal(svl) // Ignore error, continue with empty fl
	}
	fl, removedFV, removedAt, insertedAt := fl.update(folder, device, file, t.readOnlyTransaction)
	if insertedAt == -1 {
		l.Debugln("update global; same version, global unchanged")
		return keyBuf, false
	}

	name := []byte(file.Name)

	var global protocol.FileInfo
	if insertedAt == 0 {
		// Inserted a new newest version
		global = file
	} else {
		keyBuf = t.keyer.GenerateDeviceFileKey(keyBuf, folder, fl.Versions[0].Device, name)
		if new, ok := t.getFileByKey(keyBuf); ok {
			global = new
		} else {
			// This file must exist in the db, so this must be caused
			// by the db being closed - bail out.
			l.Debugln("File should exist:", name)
			return keyBuf, false
		}
	}

	// Fixup the list of files we need.
	keyBuf = t.updateLocalNeed(keyBuf, folder, name, fl, global)

	if removedAt != 0 && insertedAt != 0 {
		l.Debugf(`new global for "%v" after update: %v`, file.Name, fl)
		t.Put(gk, mustMarshal(&fl))
		return keyBuf, true
	}

	// Remove the old global from the global size counter
	var oldGlobalFV FileVersion
	if removedAt == 0 {
		oldGlobalFV = removedFV
	} else if len(fl.Versions) > 1 {
		// The previous newest version is now at index 1
		oldGlobalFV = fl.Versions[1]
	}
	keyBuf = t.keyer.GenerateDeviceFileKey(keyBuf, folder, oldGlobalFV.Device, name)
	if oldFile, ok := t.getFileByKey(keyBuf); ok {
		// A failure to get the file here is surprising and our
		// global size data will be incorrect until a restart...
		meta.removeFile(protocol.GlobalDeviceID, oldFile)
	}

	// Add the new global to the global size counter
	meta.addFile(protocol.GlobalDeviceID, global)

	l.Debugf(`new global for "%v" after update: %v`, file.Name, fl)
	t.Put(gk, mustMarshal(&fl))

	return keyBuf, true
}

// updateLocalNeed checks whether the given file is still needed on the local
// device according to the version list and global FileInfo given and updates
// the db accordingly.
func (t readWriteTransaction) updateLocalNeed(keyBuf, folder, name []byte, fl VersionList, global protocol.FileInfo) []byte {
	keyBuf = t.keyer.GenerateNeedFileKey(keyBuf, folder, name)
	hasNeeded, _ := t.Has(keyBuf, nil)
	if localFV, haveLocalFV := fl.Get(protocol.LocalDeviceID[:]); need(global, haveLocalFV, localFV.Version) {
		if !hasNeeded {
			l.Debugf("local need insert; folder=%q, name=%q", folder, name)
			t.Put(keyBuf, nil)
		}
	} else if hasNeeded {
		l.Debugf("local need delete; folder=%q, name=%q", folder, name)
		t.Delete(keyBuf)
	}
	return keyBuf
}

func need(global FileIntf, haveLocal bool, localVersion protocol.Vector) bool {
	// We never need an invalid file.
	if global.IsInvalid() {
		return false
	}
	// We don't need a deleted file if we don't have it.
	if global.IsDeleted() && !haveLocal {
		return false
	}
	// We don't need the global file if we already have the same version.
	if haveLocal && localVersion.GreaterEqual(global.FileVersion()) {
		return false
	}
	return true
}

// removeFromGlobal removes the device from the global version list for the
// given file. If the version list is empty after this, the file entry is
// removed entirely.
func (t readWriteTransaction) removeFromGlobal(gk, keyBuf, folder, device []byte, file []byte, meta *metadataTracker) []byte {
	l.Debugf("remove from global; folder=%q device=%v file=%q", folder, protocol.DeviceIDFromBytes(device), file)

	svl, err := t.Get(gk, nil)
	if err != nil {
		// We might be called to "remove" a global version that doesn't exist
		// if the first update for the file is already marked invalid.
		return keyBuf
	}

	var fl VersionList
	err = fl.Unmarshal(svl)
	if err != nil {
		l.Debugln("unmarshal error:", err)
		return keyBuf
	}

	fl, _, removedAt := fl.pop(device)
	if removedAt == -1 {
		// There is no version for the given device
		return keyBuf
	}

	if removedAt == 0 {
		// A failure to get the file here is surprising and our
		// global size data will be incorrect until a restart...
		keyBuf = t.keyer.GenerateDeviceFileKey(keyBuf, folder, device, file)
		if f, ok := t.getFileByKey(keyBuf); ok {
			meta.removeFile(protocol.GlobalDeviceID, f)
		}
	}

	if len(fl.Versions) == 0 {
		keyBuf = t.keyer.GenerateNeedFileKey(keyBuf, folder, file)
		t.Delete(keyBuf)
		t.Delete(gk)
		return keyBuf
	}

	if removedAt == 0 {
		keyBuf = t.keyer.GenerateDeviceFileKey(keyBuf, folder, fl.Versions[0].Device, file)
		global, ok := t.getFileByKey(keyBuf)
		if !ok {
			// This file must exist in the db, so this must be caused
			// by the db being closed - bail out.
			l.Debugln("File should exist:", file)
			return keyBuf
		}
		keyBuf = t.updateLocalNeed(keyBuf, folder, file, fl, global)
		meta.addFile(protocol.GlobalDeviceID, global)
	}

	l.Debugf("new global after remove: %v", fl)
	t.Put(gk, mustMarshal(&fl))

	return keyBuf
}

func (t readWriteTransaction) deleteKeyPrefix(prefix []byte) {
	dbi := t.NewIterator(util.BytesPrefix(prefix), nil)
	for dbi.Next() {
		t.Delete(dbi.Key())
		t.checkFlush()
	}
	dbi.Release()
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
