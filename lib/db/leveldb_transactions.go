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

func (t readWriteTransaction) insertFile(folder, device []byte, file protocol.FileInfo) {
	l.Debugf("insert; folder=%q device=%v %v", folder, protocol.DeviceIDFromBytes(device), file)

	name := []byte(file.Name)
	nk := t.db.deviceKey(folder, device, name)
	t.Put(nk, mustMarshal(&file))
}

// updateGlobal adds this device+version to the version list for the given
// file. If the device is already present in the list, the version is updated.
// If the file does not have an entry in the global list, it is created.
func (t readWriteTransaction) updateGlobal(folder, device []byte, file protocol.FileInfo, meta *metadataTracker) bool {
	l.Debugf("update global; folder=%q device=%v file=%q version=%v invalid=%v", folder, protocol.DeviceIDFromBytes(device), file.Name, file.Version, file.Invalid)
	name := []byte(file.Name)
	gk := t.db.globalKey(folder, name)
	svl, _ := t.Get(gk, nil) // skip error, we check len(svl) != 0 later

	var fl VersionList
	var oldFile protocol.FileInfo
	var hasOldFile bool
	// Remove the device from the current version list
	if len(svl) != 0 {
		fl.Unmarshal(svl) // skip error, range handles success case
		for i := range fl.Versions {
			if bytes.Equal(fl.Versions[i].Device, device) {
				if fl.Versions[i].Version.Equal(file.Version) && fl.Versions[i].Invalid == file.Invalid {
					// No need to do anything
					return false
				}

				if i == 0 {
					// Keep the current newest file around so we can subtract it from
					// the globalSize if we replace it.
					oldFile, hasOldFile = t.getFile(folder, fl.Versions[0].Device, name)
				}

				fl.Versions = append(fl.Versions[:i], fl.Versions[i+1:]...)
				break
			}
		}
	}

	nv := FileVersion{
		Device:  device,
		Version: file.Version,
		Invalid: file.Invalid,
	}

	insertedAt := -1
	// Find a position in the list to insert this file. The file at the front
	// of the list is the newer, the "global".
insert:
	for i := range fl.Versions {
		switch fl.Versions[i].Version.Compare(file.Version) {
		case protocol.Equal:
			if nv.Invalid {
				continue insert
			}
			fallthrough

		case protocol.Lesser:
			// The version at this point in the list is equal to or lesser
			// ("older") than us. We insert ourselves in front of it.
			fl.Versions = insertVersion(fl.Versions, i, nv)
			insertedAt = i
			break insert

		case protocol.ConcurrentLesser, protocol.ConcurrentGreater:
			// The version at this point is in conflict with us. We must pull
			// the actual file metadata to determine who wins. If we win, we
			// insert ourselves in front of the loser here. (The "Lesser" and
			// "Greater" in the condition above is just based on the device
			// IDs in the version vector, which is not the only thing we use
			// to determine the winner.)
			//
			// A surprise missing file entry here is counted as a win for us.
			of, ok := t.getFile(folder, fl.Versions[i].Device, name)
			if !ok || file.WinsConflict(of) {
				fl.Versions = insertVersion(fl.Versions, i, nv)
				insertedAt = i
				break insert
			}
		}
	}

	if insertedAt == -1 {
		// We didn't find a position for an insert above, so append to the end.
		fl.Versions = append(fl.Versions, nv)
		insertedAt = len(fl.Versions) - 1
	}

	if insertedAt == 0 {
		// We just inserted a new newest version. Fixup the global size
		// calculation.
		if !file.Version.Equal(oldFile.Version) {
			meta.addFile(globalDeviceID, file)
			if hasOldFile {
				// We have the old file that was removed at the head of the list.
				meta.removeFile(globalDeviceID, oldFile)
			} else if len(fl.Versions) > 1 {
				// The previous newest version is now at index 1, grab it from there.
				if oldFile, ok := t.getFile(folder, fl.Versions[1].Device, name); ok {
					// A failure to get the file here is surprising and our
					// global size data will be incorrect until a restart...
					meta.removeFile(globalDeviceID, oldFile)
				}
			}
		}
	}

	l.Debugf("new global after update: %v", fl)
	t.Put(gk, mustMarshal(&fl))

	return true
}

// removeFromGlobal removes the device from the global version list for the
// given file. If the version list is empty after this, the file entry is
// removed entirely.
func (t readWriteTransaction) removeFromGlobal(folder, device, file []byte, meta *metadataTracker) {
	l.Debugf("remove from global; folder=%q device=%v file=%q", folder, protocol.DeviceIDFromBytes(device), file)

	gk := t.db.globalKey(folder, file)
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

func insertVersion(vl []FileVersion, i int, v FileVersion) []FileVersion {
	t := append(vl, FileVersion{})
	copy(t[i+1:], t[i:])
	t[i] = v
	return t
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
