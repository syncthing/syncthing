// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

//go:generate -command genxdr go run ../../Godeps/_workspace/src/github.com/calmh/xdr/cmd/genxdr/main.go
//go:generate genxdr -o leveldb_xdr.go leveldb.go

package db

import (
	"bytes"
	"fmt"
	"sort"

	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/sync"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/iterator"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"
)

var (
	clockTick int64
	clockMut  = sync.NewMutex()
)

func clock(v int64) int64 {
	clockMut.Lock()
	defer clockMut.Unlock()
	if v > clockTick {
		clockTick = v + 1
	} else {
		clockTick++
	}
	return clockTick
}

const (
	KeyTypeDevice = iota
	KeyTypeGlobal
	KeyTypeBlock
	KeyTypeDeviceStatistic
	KeyTypeFolderStatistic
	KeyTypeVirtualMtime
)

type fileVersion struct {
	version protocol.Vector
	device  []byte
}

type versionList struct {
	versions []fileVersion
}

func (l versionList) String() string {
	var b bytes.Buffer
	var id protocol.DeviceID
	b.WriteString("{")
	for i, v := range l.versions {
		if i > 0 {
			b.WriteString(", ")
		}
		copy(id[:], v.device)
		fmt.Fprintf(&b, "{%d, %v}", v.version, id)
	}
	b.WriteString("}")
	return b.String()
}

type fileList []protocol.FileInfo

func (l fileList) Len() int {
	return len(l)
}

func (l fileList) Swap(a, b int) {
	l[a], l[b] = l[b], l[a]
}

func (l fileList) Less(a, b int) bool {
	return l[a].Name < l[b].Name
}

type dbReader interface {
	Get([]byte, *opt.ReadOptions) ([]byte, error)
}

type dbInstance struct {
	*leveldb.DB
}

type readOnlyTransaction struct {
	*leveldb.Snapshot
	db *leveldb.DB
}

type readWriteTransaction struct {
	readOnlyTransaction
	*leveldb.Batch
}

// Flush batches to disk when they contain this many records.
const batchFlushSize = 64

// deviceKey returns a byte slice encoding the following information:
//	   keyTypeDevice (1 byte)
//	   folder (64 bytes)
//	   device (32 bytes)
//	   name (variable size)
func deviceKey(folder, device, file []byte) []byte {
	return deviceKeyInto(nil, folder, device, file)
}

func deviceKeyInto(k []byte, folder, device, file []byte) []byte {
	reqLen := 1 + 64 + 32 + len(file)
	if len(k) < reqLen {
		k = make([]byte, reqLen)
	}
	k[0] = KeyTypeDevice
	if len(folder) > 64 {
		panic("folder name too long")
	}
	copy(k[1:], []byte(folder))
	copy(k[1+64:], device[:])
	copy(k[1+64+32:], []byte(file))
	return k[:reqLen]
}

func deviceKeyName(key []byte) []byte {
	return key[1+64+32:]
}

func deviceKeyFolder(key []byte) []byte {
	folder := key[1 : 1+64]
	izero := bytes.IndexByte(folder, 0)
	if izero < 0 {
		return folder
	}
	return folder[:izero]
}

func deviceKeyDevice(key []byte) []byte {
	return key[1+64 : 1+64+32]
}

// globalKey returns a byte slice encoding the following information:
//	   keyTypeGlobal (1 byte)
//	   folder (64 bytes)
//	   name (variable size)
func globalKey(folder, file []byte) []byte {
	k := make([]byte, 1+64+len(file))
	k[0] = KeyTypeGlobal
	if len(folder) > 64 {
		panic("folder name too long")
	}
	copy(k[1:], []byte(folder))
	copy(k[1+64:], []byte(file))
	return k
}

func globalKeyName(key []byte) []byte {
	return key[1+64:]
}

func globalKeyFolder(key []byte) []byte {
	folder := key[1 : 1+64]
	izero := bytes.IndexByte(folder, 0)
	if izero < 0 {
		return folder
	}
	return folder[:izero]
}

type deletionHandler func(t readWriteTransaction, folder, device, name []byte, dbi iterator.Iterator) int64

func (db *dbInstance) newReadOnlyTransaction() readOnlyTransaction {
	snap, err := db.GetSnapshot()
	if err != nil {
		panic(err)
	}
	return readOnlyTransaction{
		Snapshot: snap,
		db:       db.DB,
	}
}

func (db *dbInstance) newReadWriteTransaction() readWriteTransaction {
	t := db.newReadOnlyTransaction()
	return readWriteTransaction{
		readOnlyTransaction: t,
		Batch:               new(leveldb.Batch),
	}
}

func (t readOnlyTransaction) close() {
	t.Release()
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

func (db *dbInstance) genericReplace(folder, device []byte, fs []protocol.FileInfo, localSize, globalSize *sizeTracker, deleteFn deletionHandler) int64 {
	sort.Sort(fileList(fs)) // sort list on name, same as in the database

	start := deviceKey(folder, device, nil)                            // before all folder/device files
	limit := deviceKey(folder, device, []byte{0xff, 0xff, 0xff, 0xff}) // after all folder/device files

	t := db.newReadWriteTransaction()
	defer t.close()

	dbi := t.NewIterator(&util.Range{Start: start, Limit: limit}, nil)
	defer dbi.Release()

	moreDb := dbi.Next()
	fsi := 0
	var maxLocalVer int64

	isLocalDevice := bytes.Equal(device, protocol.LocalDeviceID[:])
	for {
		var newName, oldName []byte
		moreFs := fsi < len(fs)

		if !moreDb && !moreFs {
			break
		}

		if moreFs {
			newName = []byte(fs[fsi].Name)
		}

		if moreDb {
			oldName = deviceKeyName(dbi.Key())
		}

		cmp := bytes.Compare(newName, oldName)

		l.Debugf("generic replace; folder=%q device=%v moreFs=%v moreDb=%v cmp=%d newName=%q oldName=%q", folder, protocol.DeviceIDFromBytes(device), moreFs, moreDb, cmp, newName, oldName)

		switch {
		case moreFs && (!moreDb || cmp == -1):
			l.Debugln("generic replace; missing - insert")
			// Database is missing this file. Insert it.
			if lv := t.insertFile(folder, device, fs[fsi]); lv > maxLocalVer {
				maxLocalVer = lv
			}
			if isLocalDevice {
				localSize.addFile(fs[fsi])
			}
			if fs[fsi].IsInvalid() {
				t.removeFromGlobal(folder, device, newName, globalSize)
			} else {
				t.updateGlobal(folder, device, fs[fsi], globalSize)
			}
			fsi++

		case moreFs && moreDb && cmp == 0:
			// File exists on both sides - compare versions. We might get an
			// update with the same version and different flags if a device has
			// marked a file as invalid, so handle that too.
			l.Debugln("generic replace; exists - compare")
			var ef FileInfoTruncated
			ef.UnmarshalXDR(dbi.Value())
			if !fs[fsi].Version.Equal(ef.Version) || fs[fsi].Flags != ef.Flags {
				l.Debugln("generic replace; differs - insert")
				if lv := t.insertFile(folder, device, fs[fsi]); lv > maxLocalVer {
					maxLocalVer = lv
				}
				if isLocalDevice {
					localSize.removeFile(ef)
					localSize.addFile(fs[fsi])
				}
				if fs[fsi].IsInvalid() {
					t.removeFromGlobal(folder, device, newName, globalSize)
				} else {
					t.updateGlobal(folder, device, fs[fsi], globalSize)
				}
			} else {
				l.Debugln("generic replace; equal - ignore")
			}

			fsi++
			moreDb = dbi.Next()

		case moreDb && (!moreFs || cmp == 1):
			l.Debugln("generic replace; exists - remove")
			if lv := deleteFn(t, folder, device, oldName, dbi); lv > maxLocalVer {
				maxLocalVer = lv
			}
			moreDb = dbi.Next()
		}

		// Write out and reuse the batch every few records, to avoid the batch
		// growing too large and thus allocating unnecessarily much memory.
		t.checkFlush()
	}

	return maxLocalVer
}

func (db *dbInstance) replace(folder, device []byte, fs []protocol.FileInfo, localSize, globalSize *sizeTracker) int64 {
	// TODO: Return the remaining maxLocalVer?
	return db.genericReplace(folder, device, fs, localSize, globalSize, func(t readWriteTransaction, folder, device, name []byte, dbi iterator.Iterator) int64 {
		// Database has a file that we are missing. Remove it.
		l.Debugf("delete; folder=%q device=%v name=%q", folder, protocol.DeviceIDFromBytes(device), name)
		t.removeFromGlobal(folder, device, name, globalSize)
		t.Delete(dbi.Key())
		return 0
	})
}

func (db *dbInstance) updateFiles(folder, device []byte, fs []protocol.FileInfo, localSize, globalSize *sizeTracker) int64 {
	t := db.newReadWriteTransaction()
	defer t.close()

	var maxLocalVer int64
	var fk []byte
	isLocalDevice := bytes.Equal(device, protocol.LocalDeviceID[:])
	for _, f := range fs {
		name := []byte(f.Name)
		fk = deviceKeyInto(fk[:cap(fk)], folder, device, name)
		bs, err := t.Get(fk, nil)
		if err == leveldb.ErrNotFound {
			if isLocalDevice {
				localSize.addFile(f)
			}

			if lv := t.insertFile(folder, device, f); lv > maxLocalVer {
				maxLocalVer = lv
			}
			if f.IsInvalid() {
				t.removeFromGlobal(folder, device, name, globalSize)
			} else {
				t.updateGlobal(folder, device, f, globalSize)
			}
			continue
		}

		var ef FileInfoTruncated
		err = ef.UnmarshalXDR(bs)
		if err != nil {
			panic(err)
		}
		// Flags might change without the version being bumped when we set the
		// invalid flag on an existing file.
		if !ef.Version.Equal(f.Version) || ef.Flags != f.Flags {
			if isLocalDevice {
				localSize.removeFile(ef)
				localSize.addFile(f)
			}

			if lv := t.insertFile(folder, device, f); lv > maxLocalVer {
				maxLocalVer = lv
			}
			if f.IsInvalid() {
				t.removeFromGlobal(folder, device, name, globalSize)
			} else {
				t.updateGlobal(folder, device, f, globalSize)
			}
		}

		// Write out and reuse the batch every few records, to avoid the batch
		// growing too large and thus allocating unnecessarily much memory.
		t.checkFlush()
	}

	return maxLocalVer
}

func (t readWriteTransaction) insertFile(folder, device []byte, file protocol.FileInfo) int64 {
	l.Debugf("insert; folder=%q device=%v %v", folder, protocol.DeviceIDFromBytes(device), file)

	if file.LocalVersion == 0 {
		file.LocalVersion = clock(0)
	}

	name := []byte(file.Name)
	nk := deviceKey(folder, device, name)
	t.Put(nk, file.MustMarshalXDR())

	return file.LocalVersion
}

// updateGlobal adds this device+version to the version list for the given
// file. If the device is already present in the list, the version is updated.
// If the file does not have an entry in the global list, it is created.
func (t readWriteTransaction) updateGlobal(folder, device []byte, file protocol.FileInfo, globalSize *sizeTracker) bool {
	l.Debugf("update global; folder=%q device=%v file=%q version=%d", folder, protocol.DeviceIDFromBytes(device), file.Name, file.Version)
	name := []byte(file.Name)
	gk := globalKey(folder, name)
	svl, err := t.Get(gk, nil)
	if err != nil && err != leveldb.ErrNotFound {
		panic(err)
	}

	var fl versionList
	var oldFile protocol.FileInfo
	var hasOldFile bool
	// Remove the device from the current version list
	if svl != nil {
		err = fl.UnmarshalXDR(svl)
		if err != nil {
			panic(err)
		}

		for i := range fl.versions {
			if bytes.Compare(fl.versions[i].device, device) == 0 {
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

func insertVersion(vl []fileVersion, i int, v fileVersion) []fileVersion {
	t := append(vl, fileVersion{})
	copy(t[i+1:], t[i:])
	t[i] = v
	return t
}

// ldbRemoveFromGlobal removes the device from the global version list for the
// given file. If the version list is empty after this, the file entry is
// removed entirely.
func (t readWriteTransaction) removeFromGlobal(folder, device, file []byte, globalSize *sizeTracker) {
	l.Debugf("remove from global; folder=%q device=%v file=%q", folder, protocol.DeviceIDFromBytes(device), file)

	gk := globalKey(folder, file)
	svl, err := t.Get(gk, nil)
	if err != nil {
		// We might be called to "remove" a global version that doesn't exist
		// if the first update for the file is already marked invalid.
		return
	}

	var fl versionList
	err = fl.UnmarshalXDR(svl)
	if err != nil {
		panic(err)
	}

	removed := false
	for i := range fl.versions {
		if bytes.Compare(fl.versions[i].device, device) == 0 {
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

func (db *dbInstance) withHave(folder, device []byte, truncate bool, fn Iterator) {
	start := deviceKey(folder, device, nil)                            // before all folder/device files
	limit := deviceKey(folder, device, []byte{0xff, 0xff, 0xff, 0xff}) // after all folder/device files

	t := db.newReadOnlyTransaction()
	defer t.close()

	dbi := t.NewIterator(&util.Range{Start: start, Limit: limit}, nil)
	defer dbi.Release()

	for dbi.Next() {
		f, err := unmarshalTrunc(dbi.Value(), truncate)
		if err != nil {
			panic(err)
		}
		if cont := fn(f); !cont {
			return
		}
	}
}

func (db *dbInstance) withAllFolderTruncated(folder []byte, fn func(device []byte, f FileInfoTruncated) bool) {
	start := deviceKey(folder, nil, nil)                                                  // before all folder/device files
	limit := deviceKey(folder, protocol.LocalDeviceID[:], []byte{0xff, 0xff, 0xff, 0xff}) // after all folder/device files

	t := db.newReadWriteTransaction()
	defer t.close()

	dbi := t.NewIterator(&util.Range{Start: start, Limit: limit}, nil)
	defer dbi.Release()

	for dbi.Next() {
		device := deviceKeyDevice(dbi.Key())
		var f FileInfoTruncated
		err := f.UnmarshalXDR(dbi.Value())
		if err != nil {
			panic(err)
		}

		switch f.Name {
		case "", ".", "..", "/": // A few obviously invalid filenames
			l.Infof("Dropping invalid filename %q from database", f.Name)
			t.removeFromGlobal(folder, device, nil, nil)
			t.Delete(dbi.Key())
			t.checkFlush()
			continue
		}

		if cont := fn(device, f); !cont {
			return
		}
	}
}

func (db *dbInstance) getFile(folder, device, file []byte) (protocol.FileInfo, bool) {
	return ldbGetFile(db, folder, device, file)
}

func (t readOnlyTransaction) getFile(folder, device, file []byte) (protocol.FileInfo, bool) {
	return ldbGetFile(t, folder, device, file)
}

func ldbGetFile(db dbReader, folder, device, file []byte) (protocol.FileInfo, bool) {
	nk := deviceKey(folder, device, file)
	bs, err := db.Get(nk, nil)
	if err == leveldb.ErrNotFound {
		return protocol.FileInfo{}, false
	}
	if err != nil {
		panic(err)
	}

	var f protocol.FileInfo
	err = f.UnmarshalXDR(bs)
	if err != nil {
		panic(err)
	}
	return f, true
}

func (db *dbInstance) getGlobal(folder, file []byte, truncate bool) (FileIntf, bool) {
	k := globalKey(folder, file)

	t := db.newReadOnlyTransaction()
	defer t.close()

	bs, err := t.Get(k, nil)
	if err == leveldb.ErrNotFound {
		return nil, false
	}
	if err != nil {
		panic(err)
	}

	var vl versionList
	err = vl.UnmarshalXDR(bs)
	if err != nil {
		panic(err)
	}
	if len(vl.versions) == 0 {
		l.Debugln(k)
		panic("no versions?")
	}

	k = deviceKey(folder, vl.versions[0].device, file)
	bs, err = t.Get(k, nil)
	if err != nil {
		panic(err)
	}

	fi, err := unmarshalTrunc(bs, truncate)
	if err != nil {
		panic(err)
	}
	return fi, true
}

func (db *dbInstance) withGlobal(folder, prefix []byte, truncate bool, fn Iterator) {
	t := db.newReadOnlyTransaction()
	defer t.close()

	dbi := t.NewIterator(util.BytesPrefix(globalKey(folder, prefix)), nil)
	defer dbi.Release()

	var fk []byte
	for dbi.Next() {
		var vl versionList
		err := vl.UnmarshalXDR(dbi.Value())
		if err != nil {
			panic(err)
		}
		if len(vl.versions) == 0 {
			l.Debugln(dbi.Key())
			panic("no versions?")
		}
		name := globalKeyName(dbi.Key())
		fk = deviceKeyInto(fk[:cap(fk)], folder, vl.versions[0].device, name)
		bs, err := t.Get(fk, nil)
		if err != nil {
			l.Debugf("folder: %q (%x)", folder, folder)
			l.Debugf("key: %q (%x)", dbi.Key(), dbi.Key())
			l.Debugf("vl: %v", vl)
			l.Debugf("vl.versions[0].device: %x", vl.versions[0].device)
			l.Debugf("name: %q (%x)", name, name)
			l.Debugf("fk: %q", fk)
			l.Debugf("fk: %x %x %x", fk[1:1+64], fk[1+64:1+64+32], fk[1+64+32:])
			panic(err)
		}

		f, err := unmarshalTrunc(bs, truncate)
		if err != nil {
			panic(err)
		}

		if cont := fn(f); !cont {
			return
		}
	}
}

func (db *dbInstance) availability(folder, file []byte) []protocol.DeviceID {
	k := globalKey(folder, file)
	bs, err := db.Get(k, nil)
	if err == leveldb.ErrNotFound {
		return nil
	}
	if err != nil {
		panic(err)
	}

	var vl versionList
	err = vl.UnmarshalXDR(bs)
	if err != nil {
		panic(err)
	}

	var devices []protocol.DeviceID
	for _, v := range vl.versions {
		if !v.version.Equal(vl.versions[0].version) {
			break
		}
		n := protocol.DeviceIDFromBytes(v.device)
		devices = append(devices, n)
	}

	return devices
}

func (db *dbInstance) withNeed(folder, device []byte, truncate bool, fn Iterator) {
	start := globalKey(folder, nil)
	limit := globalKey(folder, []byte{0xff, 0xff, 0xff, 0xff})

	t := db.newReadOnlyTransaction()
	defer t.close()

	dbi := t.NewIterator(&util.Range{Start: start, Limit: limit}, nil)
	defer dbi.Release()

	var fk []byte
nextFile:
	for dbi.Next() {
		var vl versionList
		err := vl.UnmarshalXDR(dbi.Value())
		if err != nil {
			panic(err)
		}
		if len(vl.versions) == 0 {
			l.Debugln(dbi.Key())
			panic("no versions?")
		}

		have := false // If we have the file, any version
		need := false // If we have a lower version of the file
		var haveVersion protocol.Vector
		for _, v := range vl.versions {
			if bytes.Compare(v.device, device) == 0 {
				have = true
				haveVersion = v.version
				// XXX: This marks Concurrent (i.e. conflicting) changes as
				// needs. Maybe we should do that, but it needs special
				// handling in the puller.
				need = !v.version.GreaterEqual(vl.versions[0].version)
				break
			}
		}

		if need || !have {
			name := globalKeyName(dbi.Key())
			needVersion := vl.versions[0].version

		nextVersion:
			for i := range vl.versions {
				if !vl.versions[i].version.Equal(needVersion) {
					// We haven't found a valid copy of the file with the needed version.
					continue nextFile
				}
				fk = deviceKeyInto(fk[:cap(fk)], folder, vl.versions[i].device, name)
				bs, err := t.Get(fk, nil)
				if err != nil {
					var id protocol.DeviceID
					copy(id[:], device)
					l.Debugf("device: %v", id)
					l.Debugf("need: %v, have: %v", need, have)
					l.Debugf("key: %q (%x)", dbi.Key(), dbi.Key())
					l.Debugf("vl: %v", vl)
					l.Debugf("i: %v", i)
					l.Debugf("fk: %q (%x)", fk, fk)
					l.Debugf("name: %q (%x)", name, name)
					panic(err)
				}

				gf, err := unmarshalTrunc(bs, truncate)
				if err != nil {
					panic(err)
				}

				if gf.IsInvalid() {
					// The file is marked invalid for whatever reason, don't use it.
					continue nextVersion
				}

				if gf.IsDeleted() && !have {
					// We don't need deleted files that we don't have
					continue nextFile
				}

				l.Debugf("need folder=%q device=%v name=%q need=%v have=%v haveV=%d globalV=%d", folder, protocol.DeviceIDFromBytes(device), name, need, have, haveVersion, vl.versions[0].version)

				if cont := fn(gf); !cont {
					return
				}

				// This file is handled, no need to look further in the version list
				continue nextFile
			}
		}
	}
}

func (db *dbInstance) listFolders() []string {
	t := db.newReadOnlyTransaction()
	defer t.close()

	dbi := t.NewIterator(util.BytesPrefix([]byte{KeyTypeGlobal}), nil)
	defer dbi.Release()

	folderExists := make(map[string]bool)
	for dbi.Next() {
		folder := string(globalKeyFolder(dbi.Key()))
		if !folderExists[folder] {
			folderExists[folder] = true
		}
	}

	folders := make([]string, 0, len(folderExists))
	for k := range folderExists {
		folders = append(folders, k)
	}

	sort.Strings(folders)
	return folders
}

func (db *dbInstance) dropFolder(folder []byte) {
	t := db.newReadOnlyTransaction()
	defer t.close()

	// Remove all items related to the given folder from the device->file bucket
	dbi := t.NewIterator(util.BytesPrefix([]byte{KeyTypeDevice}), nil)
	for dbi.Next() {
		itemFolder := deviceKeyFolder(dbi.Key())
		if bytes.Compare(folder, itemFolder) == 0 {
			db.Delete(dbi.Key(), nil)
		}
	}
	dbi.Release()

	// Remove all items related to the given folder from the global bucket
	dbi = t.NewIterator(util.BytesPrefix([]byte{KeyTypeGlobal}), nil)
	for dbi.Next() {
		itemFolder := globalKeyFolder(dbi.Key())
		if bytes.Compare(folder, itemFolder) == 0 {
			db.Delete(dbi.Key(), nil)
		}
	}
	dbi.Release()
}

func unmarshalTrunc(bs []byte, truncate bool) (FileIntf, error) {
	if truncate {
		var tf FileInfoTruncated
		err := tf.UnmarshalXDR(bs)
		return tf, err
	}

	var tf protocol.FileInfo
	err := tf.UnmarshalXDR(bs)
	return tf, err
}

func (db *dbInstance) checkGlobals(folder []byte, globalSize *sizeTracker) {
	t := db.newReadWriteTransaction()
	defer t.close()

	start := globalKey(folder, nil)
	limit := globalKey(folder, []byte{0xff, 0xff, 0xff, 0xff})
	dbi := t.NewIterator(&util.Range{Start: start, Limit: limit}, nil)
	defer dbi.Release()

	var fk []byte
	for dbi.Next() {
		gk := dbi.Key()
		var vl versionList
		err := vl.UnmarshalXDR(dbi.Value())
		if err != nil {
			panic(err)
		}

		// Check the global version list for consistency. An issue in previous
		// versions of goleveldb could result in reordered writes so that
		// there are global entries pointing to no longer existing files. Here
		// we find those and clear them out.

		name := globalKeyName(gk)
		var newVL versionList
		for i, version := range vl.versions {
			fk = deviceKeyInto(fk[:cap(fk)], folder, version.device, name)

			_, err := t.Get(fk, nil)
			if err == leveldb.ErrNotFound {
				continue
			}
			if err != nil {
				panic(err)
			}
			newVL.versions = append(newVL.versions, version)

			if i == 0 {
				fi, ok := t.getFile(folder, version.device, name)
				if !ok {
					panic("nonexistent global master file")
				}
				globalSize.addFile(fi)
			}
		}

		if len(newVL.versions) != len(vl.versions) {
			t.Put(dbi.Key(), newVL.MustMarshalXDR())
			t.checkFlush()
		}
	}
	l.Debugf("db check completed for %q", folder)
}
