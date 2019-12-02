// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package db

import (
	"github.com/syncthing/syncthing/lib/db/backend"
	"github.com/syncthing/syncthing/lib/protocol"
)

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
	f, err := unmarshalTrunc(bs, trunc)
	if err != nil {
		return nil, false, err
	}
	return f, true, nil
}

func (t readOnlyTransaction) getGlobal(keyBuf, folder, file []byte, truncate bool) ([]byte, FileIntf, bool, error) {
	var err error
	keyBuf, err = t.keyer.GenerateGlobalVersionKey(keyBuf, folder, file)
	if err != nil {
		return nil, nil, false, err
	}

	bs, err := t.Get(keyBuf)
	if backend.IsNotFound(err) {
		return keyBuf, nil, false, nil
	}
	if err != nil {
		return nil, nil, false, err
	}

	vl, ok := unmarshalVersionList(bs)
	if !ok {
		return keyBuf, nil, false, nil
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

func (t readWriteTransaction) commit() error {
	t.readOnlyTransaction.close()
	return t.WriteTransaction.Commit()
}

func (t readWriteTransaction) close() {
	t.readOnlyTransaction.close()
	t.WriteTransaction.Release()
}

// updateGlobal adds this device+version to the version list for the given
// file. If the device is already present in the list, the version is updated.
// If the file does not have an entry in the global list, it is created.
func (t readWriteTransaction) updateGlobal(gk, keyBuf, folder, device []byte, file protocol.FileInfo, meta *metadataTracker) ([]byte, bool, error) {
	l.Debugf("update global; folder=%q device=%v file=%q version=%v invalid=%v", folder, protocol.DeviceIDFromBytes(device), file.Name, file.Version, file.IsInvalid())

	var fl VersionList
	svl, err := t.Get(gk)
	if err == nil {
		_ = fl.Unmarshal(svl) // Ignore error, continue with empty fl
	} else if !backend.IsNotFound(err) {
		return nil, false, err
	}

	fl, removedFV, removedAt, insertedAt, err := fl.update(folder, device, file, t.readOnlyTransaction)
	if err != nil {
		return nil, false, err
	}
	if insertedAt == -1 {
		l.Debugln("update global; same version, global unchanged")
		return keyBuf, false, nil
	}

	name := []byte(file.Name)

	var global protocol.FileInfo
	if insertedAt == 0 {
		// Inserted a new newest version
		global = file
	} else {
		keyBuf, err = t.keyer.GenerateDeviceFileKey(keyBuf, folder, fl.Versions[0].Device, name)
		if err != nil {
			return nil, false, err
		}
		new, ok, err := t.getFileByKey(keyBuf)
		if err != nil || !ok {
			return keyBuf, false, err
		}
		global = new
	}

	// Fixup the list of files we need.
	keyBuf, err = t.updateLocalNeed(keyBuf, folder, name, fl, global)
	if err != nil {
		return nil, false, err
	}

	if removedAt != 0 && insertedAt != 0 {
		l.Debugf(`new global for "%v" after update: %v`, file.Name, fl)
		if err := t.Put(gk, mustMarshal(&fl)); err != nil {
			return nil, false, err
		}
		return keyBuf, true, nil
	}

	// Remove the old global from the global size counter
	var oldGlobalFV FileVersion
	if removedAt == 0 {
		oldGlobalFV = removedFV
	} else if len(fl.Versions) > 1 {
		// The previous newest version is now at index 1
		oldGlobalFV = fl.Versions[1]
	}
	keyBuf, err = t.keyer.GenerateDeviceFileKey(keyBuf, folder, oldGlobalFV.Device, name)
	if err != nil {
		return nil, false, err
	}
	oldFile, ok, err := t.getFileByKey(keyBuf)
	if err != nil {
		return nil, false, err
	}
	if ok {
		// A failure to get the file here is surprising and our
		// global size data will be incorrect until a restart...
		meta.removeFile(protocol.GlobalDeviceID, oldFile)
	}

	// Add the new global to the global size counter
	meta.addFile(protocol.GlobalDeviceID, global)

	l.Debugf(`new global for "%v" after update: %v`, file.Name, fl)
	if err := t.Put(gk, mustMarshal(&fl)); err != nil {
		return nil, false, err
	}

	return keyBuf, true, nil
}

// updateLocalNeed checks whether the given file is still needed on the local
// device according to the version list and global FileInfo given and updates
// the db accordingly.
func (t readWriteTransaction) updateLocalNeed(keyBuf, folder, name []byte, fl VersionList, global protocol.FileInfo) ([]byte, error) {
	var err error
	keyBuf, err = t.keyer.GenerateNeedFileKey(keyBuf, folder, name)
	if err != nil {
		return nil, err
	}
	_, err = t.Get(keyBuf)
	if err != nil && !backend.IsNotFound(err) {
		return nil, err
	}
	hasNeeded := err == nil
	if localFV, haveLocalFV := fl.Get(protocol.LocalDeviceID[:]); need(global, haveLocalFV, localFV.Version) {
		if !hasNeeded {
			l.Debugf("local need insert; folder=%q, name=%q", folder, name)
			if err := t.Put(keyBuf, nil); err != nil {
				return nil, err
			}
		}
	} else if hasNeeded {
		l.Debugf("local need delete; folder=%q, name=%q", folder, name)
		if err := t.Delete(keyBuf); err != nil {
			return nil, err
		}
	}
	return keyBuf, nil
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
func (t readWriteTransaction) removeFromGlobal(gk, keyBuf, folder, device []byte, file []byte, meta *metadataTracker) ([]byte, error) {
	l.Debugf("remove from global; folder=%q device=%v file=%q", folder, protocol.DeviceIDFromBytes(device), file)

	svl, err := t.Get(gk)
	if backend.IsNotFound(err) {
		// We might be called to "remove" a global version that doesn't exist
		// if the first update for the file is already marked invalid.
		return keyBuf, nil
	} else if err != nil {
		return nil, err
	}

	var fl VersionList
	err = fl.Unmarshal(svl)
	if err != nil {
		return nil, err
	}

	fl, _, removedAt := fl.pop(device)
	if removedAt == -1 {
		// There is no version for the given device
		return keyBuf, nil
	}

	if removedAt == 0 {
		// A failure to get the file here is surprising and our
		// global size data will be incorrect until a restart...
		keyBuf, err = t.keyer.GenerateDeviceFileKey(keyBuf, folder, device, file)
		if err != nil {
			return nil, err
		}
		if f, ok, err := t.getFileByKey(keyBuf); err != nil {
			return keyBuf, nil
		} else if ok {
			meta.removeFile(protocol.GlobalDeviceID, f)
		}
	}

	if len(fl.Versions) == 0 {
		keyBuf, err = t.keyer.GenerateNeedFileKey(keyBuf, folder, file)
		if err != nil {
			return nil, err
		}
		if err := t.Delete(keyBuf); err != nil {
			return nil, err
		}
		if err := t.Delete(gk); err != nil {
			return nil, err
		}
		return keyBuf, nil
	}

	if removedAt == 0 {
		keyBuf, err = t.keyer.GenerateDeviceFileKey(keyBuf, folder, fl.Versions[0].Device, file)
		if err != nil {
			return nil, err
		}
		global, ok, err := t.getFileByKey(keyBuf)
		if err != nil || !ok {
			return keyBuf, err
		}
		keyBuf, err = t.updateLocalNeed(keyBuf, folder, file, fl, global)
		if err != nil {
			return nil, err
		}
		meta.addFile(protocol.GlobalDeviceID, global)
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
