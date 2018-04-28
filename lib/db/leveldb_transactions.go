// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package db

import (
	"bytes"
	"sync/atomic"

	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/util"
)

// A readOnlyTransaction represents a database snapshot.
type readOnlyTransaction struct {
	*leveldb.Snapshot
	db *Instance
}

func (db *Instance) newReadOnlyTransaction() readOnlyTransaction {
	snap, err := db.GetSnapshot()
	if err != nil {
		panic(err)
	}
	return readOnlyTransaction{
		Snapshot: snap,
		db:       db,
	}
}

func (t readOnlyTransaction) close() {
	t.Release()
}

func (t readOnlyTransaction) getFile(folder, device, file []byte) (protocol.FileInfo, bool) {
	return t.db.getFile(t.db.deviceKey(folder, device, file))
}

// A readWriteTransaction is a readOnlyTransaction plus a batch for writes.
// The batch will be committed on close() or by checkFlush() if it exceeds the
// batch size.
type readWriteTransaction struct {
	readOnlyTransaction
	*leveldb.Batch
}

func (db *Instance) newReadWriteTransaction() readWriteTransaction {
	t := db.newReadOnlyTransaction()
	return readWriteTransaction{
		readOnlyTransaction: t,
		Batch:               new(leveldb.Batch),
	}
}

func (t readWriteTransaction) close() {
	t.flush()
	t.readOnlyTransaction.close()
}

func (t readWriteTransaction) checkFlush() {
	if t.Batch.Len() > batchFlushSize {
		t.flush()
		t.Batch.Reset()
	}
}

func (t readWriteTransaction) flush() {
	if err := t.db.Write(t.Batch, nil); err != nil {
		panic(err)
	}
	atomic.AddInt64(&t.db.committed, int64(t.Batch.Len()))
}

func (t readWriteTransaction) insertFile(fk, folder, device []byte, file protocol.FileInfo) {
	l.Debugf("insert; folder=%q device=%v %v", folder, protocol.DeviceIDFromBytes(device), file)

	t.Put(fk, mustMarshal(&file))
}

// updateGlobal adds this device+version to the version list for the given
// file. If the device is already present in the list, the version is updated.
// If the file does not have an entry in the global list, it is created.
func (t readWriteTransaction) updateGlobal(gk, folder, device []byte, file protocol.FileInfo, meta *metadataTracker) bool {
	l.Debugf("update global; folder=%q device=%v file=%q version=%v invalid=%v", folder, protocol.DeviceIDFromBytes(device), file.Name, file.Version, file.Invalid)

	var fl VersionList
	if svl, err := t.Get(gk, nil); err == nil {
		fl.Unmarshal(svl) // Ignore error, continue with empty fl
	}
	fl, removedFV, removedAt, insertedAt := fl.update(folder, device, file, t.db)
	if insertedAt == -1 {
		l.Debugln("update global; same version, global unchanged")
		return false
	}

	if insertedAt == 0 {
		// We just inserted a new newest version.

		name := []byte(file.Name)

		// Fixup the global size calculation.

		var oldGlobalFV FileVersion
		if removedAt == 0 {
			// We have the old file version that was removed at the head of the list.
			oldGlobalFV = removedFV
		} else if len(fl.Versions) > 1 {
			// The previous newest version is now at index 1, grab it from there.
			oldGlobalFV = fl.Versions[1]
		}

		if !file.Version.Equal(oldGlobalFV.Version) || file.Invalid != oldGlobalFV.Invalid {
			if oldFile, ok := t.getFile(folder, oldGlobalFV.Device, name); ok {
				// A failure to get the file here is surprising and our
				// global size data will be incorrect until a restart...
				l.Debugln("update global, got old file", file.Name)
				meta.removeFile(globalDeviceID, oldFile)
			}
			meta.addFile(globalDeviceID, file)
		}

		// Fixup the list of files we need.

		var haveLocalFV bool
		var localFV FileVersion
		if bytes.Equal(device, protocol.LocalDeviceID[:]) {
			// The local file was just inserted at the beginning.
			haveLocalFV = true
			localFV = fl.Versions[0]
		} else {
			localFV, haveLocalFV = fl.Get(protocol.LocalDeviceID[:])
		}

		nk := t.db.needKey(folder, name)
		if hasNeeded, _ := t.db.Has(nk, nil); need(file, haveLocalFV, localFV.Version) {
			if !hasNeeded {
				l.Debugf("local need insert; folder=%q, name=%q", folder, name)
				t.Put(nk, nil)
			}
		} else if hasNeeded {
			l.Debugf("local need delete; folder=%q, name=%q", folder, name)
			t.Delete(nk)
		}
	}

	l.Debugf(`new global for "%v" after update: %v`, file.Name, fl)
	t.Put(gk, mustMarshal(&fl))

	return true
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
	if haveLocal && localVersion.Equal(global.FileVersion()) {
		return false
	}
	return true
}

// removeFromGlobal removes the device from the global version list for the
// given file. If the version list is empty after this, the file entry is
// removed entirely.
func (t readWriteTransaction) removeFromGlobal(gk, folder, device, file []byte, meta *metadataTracker) {
	l.Debugf("remove from global; folder=%q device=%v file=%q", folder, protocol.DeviceIDFromBytes(device), file)

	svl, err := t.Get(gk, nil)
	if err != nil {
		// We might be called to "remove" a global version that doesn't exist
		// if the first update for the file is already marked invalid.
		return
	}

	var fl VersionList
	err = fl.Unmarshal(svl)
	if err != nil {
		l.Debugln("unmarshal error:", err)
		return
	}

	removed := false
	for i := range fl.Versions {
		if bytes.Equal(fl.Versions[i].Device, device) {
			if i == 0 && meta != nil {
				f, ok := t.getFile(folder, device, file)
				if !ok {
					// didn't exist anyway, apparently
					continue
				}
				meta.removeFile(globalDeviceID, f)
				removed = true
			}
			fl.Versions = append(fl.Versions[:i], fl.Versions[i+1:]...)
			break
		}
	}

	if len(fl.Versions) == 0 {
		t.Delete(gk)
		return
	}
	l.Debugf("new global after remove: %v", fl)
	t.Put(gk, mustMarshal(&fl))
	if removed {
		if f, ok := t.getFile(folder, fl.Versions[0].Device, file); ok {
			// A failure to get the file here is surprising and our
			// global size data will be incorrect until a restart...
			meta.addFile(globalDeviceID, f)
		}
	}
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
