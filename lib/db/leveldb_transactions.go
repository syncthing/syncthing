// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package db

import (
	"bytes"

	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syndtr/goleveldb/leveldb"
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
	return getFile(t, t.db.deviceKey(folder, device, file))
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
	if err := t.db.Write(t.Batch, nil); err != nil {
		panic(err)
	}
	t.readOnlyTransaction.close()
}

func (t readWriteTransaction) checkFlush() {
	if t.Batch.Len() > batchFlushSize {
		if err := t.db.Write(t.Batch, nil); err != nil {
			panic(err)
		}
		t.Batch.Reset()
	}
}

func (t readWriteTransaction) insertFile(folder, device []byte, file protocol.FileInfo) int64 {
	l.Debugf("insert; folder=%q device=%v %v", folder, protocol.DeviceIDFromBytes(device), file)

	if file.LocalVersion == 0 {
		file.LocalVersion = clock(0)
	}

	name := []byte(file.Name)
	nk := t.db.deviceKey(folder, device, name)
	t.Put(nk, file.MustMarshalXDR())

	return file.LocalVersion
}

// updateGlobal adds this device+version to the version list for the given
// file. If the device is already present in the list, the version is updated.
// If the file does not have an entry in the global list, it is created.
func (t readWriteTransaction) updateGlobal(folder, device []byte, file protocol.FileInfo, globalSize *sizeTracker) bool {
	l.Debugf("update global; folder=%q device=%v file=%q version=%d", folder, protocol.DeviceIDFromBytes(device), file.Name, file.Version)
	name := []byte(file.Name)
	gk := t.db.globalKey(folder, name)
	svl, err := t.Get(gk, nil)
	if err != nil && err != leveldb.ErrNotFound {
		panic(err)
	}

	var fl VersionList
	var oldFile protocol.FileInfo
	var hasOldFile bool
	// Remove the device from the current version list
	if len(svl) != 0 {
		err = fl.UnmarshalXDR(svl)
		if err != nil {
			panic(err)
		}

		for i := range fl.versions {
			if bytes.Equal(fl.versions[i].device, device) {
				if fl.versions[i].version.Equal(file.Version) {
					// No need to do anything
					return false
				}

				if i == 0 {
					// Keep the current newest file around so we can subtract it from
					// the globalSize if we replace it.
					oldFile, hasOldFile = t.getFile(folder, fl.versions[0].device, name)
				}

				fl.versions = append(fl.versions[:i], fl.versions[i+1:]...)
				break
			}
		}
	}

	nv := fileVersion{
		device:  device,
		version: file.Version,
	}

	insertedAt := -1
	// Find a position in the list to insert this file. The file at the front
	// of the list is the newer, the "global".
	for i := range fl.versions {
		switch fl.versions[i].version.Compare(file.Version) {
		case protocol.Equal, protocol.Lesser:
			// The version at this point in the list is equal to or lesser
			// ("older") than us. We insert ourselves in front of it.
			fl.versions = insertVersion(fl.versions, i, nv)
			insertedAt = i
			goto done

		case protocol.ConcurrentLesser, protocol.ConcurrentGreater:
			// The version at this point is in conflict with us. We must pull
			// the actual file metadata to determine who wins. If we win, we
			// insert ourselves in front of the loser here. (The "Lesser" and
			// "Greater" in the condition above is just based on the device
			// IDs in the version vector, which is not the only thing we use
			// to determine the winner.)
			of, ok := t.getFile(folder, fl.versions[i].device, name)
			if !ok {
				panic("file referenced in version list does not exist")
			}
			if file.WinsConflict(of) {
				fl.versions = insertVersion(fl.versions, i, nv)
				insertedAt = i
				goto done
			}
		}
	}

	// We didn't find a position for an insert above, so append to the end.
	fl.versions = append(fl.versions, nv)
	insertedAt = len(fl.versions) - 1

done:
	if insertedAt == 0 {
		// We just inserted a new newest version. Fixup the global size
		// calculation.
		if !file.Version.Equal(oldFile.Version) {
			globalSize.addFile(file)
			if hasOldFile {
				// We have the old file that was removed at the head of the list.
				globalSize.removeFile(oldFile)
			} else if len(fl.versions) > 1 {
				// The previous newest version is now at index 1, grab it from there.
				oldFile, ok := t.getFile(folder, fl.versions[1].device, name)
				if !ok {
					panic("file referenced in version list does not exist")
				}
				globalSize.removeFile(oldFile)
			}
		}
	}

	l.Debugf("new global after update: %v", fl)
	t.Put(gk, fl.MustMarshalXDR())

	return true
}

// removeFromGlobal removes the device from the global version list for the
// given file. If the version list is empty after this, the file entry is
// removed entirely.
func (t readWriteTransaction) removeFromGlobal(folder, device, file []byte, globalSize *sizeTracker) {
	l.Debugf("remove from global; folder=%q device=%v file=%q", folder, protocol.DeviceIDFromBytes(device), file)

	gk := t.db.globalKey(folder, file)
	svl, err := t.Get(gk, nil)
	if err != nil {
		// We might be called to "remove" a global version that doesn't exist
		// if the first update for the file is already marked invalid.
		return
	}

	var fl VersionList
	err = fl.UnmarshalXDR(svl)
	if err != nil {
		panic(err)
	}

	removed := false
	for i := range fl.versions {
		if bytes.Equal(fl.versions[i].device, device) {
			if i == 0 && globalSize != nil {
				f, ok := t.getFile(folder, device, file)
				if !ok {
					panic("removing nonexistent file")
				}
				globalSize.removeFile(f)
				removed = true
			}
			fl.versions = append(fl.versions[:i], fl.versions[i+1:]...)
			break
		}
	}

	if len(fl.versions) == 0 {
		t.Delete(gk)
	} else {
		l.Debugf("new global after remove: %v", fl)
		t.Put(gk, fl.MustMarshalXDR())
		if removed {
			f, ok := t.getFile(folder, fl.versions[0].device, file)
			if !ok {
				panic("new global is nonexistent file")
			}
			globalSize.addFile(f)
		}
	}
}

func insertVersion(vl []fileVersion, i int, v fileVersion) []fileVersion {
	t := append(vl, fileVersion{})
	copy(t[i+1:], t[i:])
	t[i] = v
	return t
}
