// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package db

import (
	"errors"

	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/iterator"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"
)

// The reader interface covers the raw database, a snapshot, or a transaction
type reader interface {
	Get(key []byte, ro *opt.ReadOptions) ([]byte, error)
	Has(key []byte, ro *opt.ReadOptions) (bool, error)
	NewIterator(*util.Range, *opt.ReadOptions) iterator.Iterator
}

// The writer interface covers the raw database or a transaction
type writer interface {
	Put(key []byte, val []byte, wo *opt.WriteOptions) error
	Delete(key []byte, wo *opt.WriteOptions) error
	Write(batch *leveldb.Batch, wo *opt.WriteOptions) error
}

type readWriter interface {
	reader
	writer
}

// The snapshot must be released, and is also a reader
type snapshot interface {
	Release()
	reader
}

// A readOnlyTransaction represents a database snapshot.
type readOnlyTransaction struct {
	reader
}

// The errorWriter can be passed when a function takes a writer, but we
// don't have one and we're reasonably sure that one shouldn't be needed.
type errorWriter struct{}

var errReadOnly = errors.New("write in read only context")

func (errorWriter) Put(key []byte, val []byte, wo *opt.WriteOptions) error { return errReadOnly }
func (errorWriter) Delete(key []byte, wo *opt.WriteOptions) error          { return errReadOnly }
func (errorWriter) Write(batch *leveldb.Batch, wo *opt.WriteOptions) error { return errReadOnly }

func (db *instance) newReadOnlyTransaction() readOnlyTransaction {
	return newReadOnlyTransaction(db.GetSnapshot())
}

func newReadOnlyTransaction(reader reader) readOnlyTransaction {
	return readOnlyTransaction{
		reader: reader,
	}
}

func (t readOnlyTransaction) close() {
	if snap, ok := t.reader.(snapshot); ok {
		snap.Release()
	}
}

func getFile(r reader, k keyer, folder, device, file []byte) (protocol.FileInfo, bool) {
	return getFileByKey(r, k.GenerateDeviceFileKey(errorWriter{}, nil, folder, device, file))
}

func getFileByKey(r reader, key []byte) (protocol.FileInfo, bool) {
	if f, ok := getFileTrunc(r, key, false); ok {
		return f.(protocol.FileInfo), true
	}
	return protocol.FileInfo{}, false
}

func getFileTrunc(r reader, key []byte, trunc bool) (FileIntf, bool) {
	bs, err := r.Get(key, nil)
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

func getGlobal(r reader, k keyer, keyBuf, folder, file []byte, truncate bool) ([]byte, FileIntf, bool) {
	var ok bool
	keyBuf, ok = k.GenerateGlobalVersionKeyRO(keyBuf, folder, file)
	if !ok {
		return keyBuf, nil, false
	}

	bs, err := r.Get(keyBuf, nil)
	if err != nil {
		return keyBuf, nil, false
	}

	vl, ok := unmarshalVersionList(bs)
	if !ok {
		return keyBuf, nil, false
	}

	keyBuf = k.GenerateDeviceFileKey(errorWriter{}, keyBuf, folder, vl.Versions[0].Device, file)
	if fi, ok := getFileTrunc(r, keyBuf, truncate); ok {
		return keyBuf, fi, true
	}

	return keyBuf, nil, false
}

// A readWriteTransaction is a readOnlyTransaction plus a batch for writes.
// The batch will be committed on close() or by checkFlush() if it exceeds the
// batch size.
type readWriteTransaction struct {
	readOnlyTransaction
	*leveldb.Transaction
}

func (db *instance) newReadWriteTransaction() readWriteTransaction {
	tran, err := db.OpenTransaction()
	if err != nil && err != leveldb.ErrClosed {
		panic(err)
	}
	return readWriteTransaction{
		readOnlyTransaction: newReadOnlyTransaction(tran),
		Transaction:         tran,
	}
}

func (t readWriteTransaction) close() {
	if err := t.Transaction.Commit(); err != nil && err != leveldb.ErrClosed {
		panic(err)
	}
}

// updateGlobal adds this device+version to the version list for the given
// file. If the device is already present in the list, the version is updated.
// If the file does not have an entry in the global list, it is created.
func updateGlobal(rw readWriter, k keyer, gk, keyBuf, folder, device []byte, file protocol.FileInfo, meta *metadataTracker) ([]byte, bool) {
	l.Debugf("update global; folder=%q device=%v file=%q version=%v invalid=%v", folder, protocol.DeviceIDFromBytes(device), file.Name, file.Version, file.IsInvalid())

	var fl VersionList
	if svl, err := rw.Get(gk, nil); err == nil {
		fl.Unmarshal(svl) // Ignore error, continue with empty fl
	}
	fl, removedFV, removedAt, insertedAt := fl.update(rw, k, folder, device, file)
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
		keyBuf = k.GenerateDeviceFileKey(rw, keyBuf, folder, fl.Versions[0].Device, name)
		if new, ok := getFileByKey(rw, keyBuf); ok {
			global = new
		} else {
			// This file must exist in the db, so this must be caused
			// by the db being closed - bail out.
			l.Debugln("File should exist:", name)
			return keyBuf, false
		}
	}

	// Fixup the list of files we need.
	keyBuf = updateLocalNeed(rw, k, keyBuf, folder, name, fl, global)

	if removedAt != 0 && insertedAt != 0 {
		l.Debugf(`new global for "%v" after update: %v`, file.Name, fl)
		rw.Put(gk, mustMarshal(&fl), nil)
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
	keyBuf = k.GenerateDeviceFileKey(rw, keyBuf, folder, oldGlobalFV.Device, name)
	if oldFile, ok := getFileByKey(rw, keyBuf); ok {
		// A failure to get the file here is surprising and our
		// global size data will be incorrect until a restart...
		meta.removeFile(protocol.GlobalDeviceID, oldFile)
	}

	// Add the new global to the global size counter
	meta.addFile(protocol.GlobalDeviceID, global)

	l.Debugf(`new global for "%v" after update: %v`, file.Name, fl)
	rw.Put(gk, mustMarshal(&fl), nil)

	return keyBuf, true
}

// updateLocalNeed checks whether the given file is still needed on the local
// device according to the version list and global FileInfo given and updates
// the db accordingly.
func updateLocalNeed(rw readWriter, k keyer, keyBuf, folder, name []byte, fl VersionList, global protocol.FileInfo) []byte {
	keyBuf = k.GenerateNeedFileKey(rw, keyBuf, folder, name)
	hasNeeded, _ := rw.Has(keyBuf, nil)
	if localFV, haveLocalFV := fl.Get(protocol.LocalDeviceID[:]); need(global, haveLocalFV, localFV.Version) {
		if !hasNeeded {
			l.Debugf("local need insert; folder=%q, name=%q", folder, name)
			rw.Put(keyBuf, nil, nil)
		}
	} else if hasNeeded {
		l.Debugf("local need delete; folder=%q, name=%q", folder, name)
		rw.Delete(keyBuf, nil)
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
func removeFromGlobal(rw readWriter, k keyer, gk, keyBuf, folder, device []byte, file []byte, meta *metadataTracker) []byte {
	l.Debugf("remove from global; folder=%q device=%v file=%q", folder, protocol.DeviceIDFromBytes(device), file)

	svl, err := rw.Get(gk, nil)
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
		keyBuf = k.GenerateDeviceFileKey(rw, keyBuf, folder, device, file)
		if f, ok := getFileByKey(rw, keyBuf); ok {
			meta.removeFile(protocol.GlobalDeviceID, f)
		}
	}

	if len(fl.Versions) == 0 {
		keyBuf = k.GenerateNeedFileKey(rw, keyBuf, folder, file)
		rw.Delete(keyBuf, nil)
		rw.Delete(gk, nil)
		return keyBuf
	}

	if removedAt == 0 {
		keyBuf = k.GenerateDeviceFileKey(rw, keyBuf, folder, fl.Versions[0].Device, file)
		global, ok := getFileByKey(rw, keyBuf)
		if !ok {
			// This file must exist in the db, so this must be caused
			// by the db being closed - bail out.
			l.Debugln("File should exist:", file)
			return keyBuf
		}
		keyBuf = updateLocalNeed(rw, k, keyBuf, folder, file, fl, global)
		meta.addFile(protocol.GlobalDeviceID, global)
	}

	l.Debugf("new global after remove: %v", fl)
	rw.Put(gk, mustMarshal(&fl), nil)

	return keyBuf
}

func deleteKeyPrefix(rw readWriter, prefix []byte) {
	dbi := rw.NewIterator(util.BytesPrefix(prefix), nil)
	for dbi.Next() {
		rw.Delete(dbi.Key(), nil)
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
