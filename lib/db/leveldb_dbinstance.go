// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package db

import (
	"bytes"
	"os"
	"sort"
	"strings"

	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/errors"
	"github.com/syndtr/goleveldb/leveldb/iterator"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/storage"
	"github.com/syndtr/goleveldb/leveldb/util"
)

type deletionHandler func(t readWriteTransaction, folder, device, name []byte, dbi iterator.Iterator) int64

type Instance struct {
	*leveldb.DB
}

func Open(file string) (*Instance, error) {
	opts := &opt.Options{
		OpenFilesCacheCapacity: 100,
		WriteBuffer:            4 << 20,
	}

	db, err := leveldb.OpenFile(file, opts)
	if leveldbIsCorrupted(err) {
		db, err = leveldb.RecoverFile(file, opts)
	}
	if leveldbIsCorrupted(err) {
		// The database is corrupted, and we've tried to recover it but it
		// didn't work. At this point there isn't much to do beyond dropping
		// the database and reindexing...
		l.Infoln("Database corruption detected, unable to recover. Reinitializing...")
		if err := os.RemoveAll(file); err != nil {
			return nil, err
		}
		db, err = leveldb.OpenFile(file, opts)
	}
	if err != nil {
		return nil, err
	}

	return newDBInstance(db), nil
}

func OpenMemory() *Instance {
	db, _ := leveldb.Open(storage.NewMemStorage(), nil)
	return newDBInstance(db)
}

func newDBInstance(db *leveldb.DB) *Instance {
	return &Instance{
		DB: db,
	}
}

func (db *Instance) Compact() error {
	return db.CompactRange(util.Range{})
}

func (db *Instance) genericReplace(folder, device []byte, fs []protocol.FileInfo, localSize, globalSize *sizeTracker, deleteFn deletionHandler) int64 {
	sort.Sort(fileList(fs)) // sort list on name, same as in the database

	start := db.deviceKey(folder, device, nil)                            // before all folder/device files
	limit := db.deviceKey(folder, device, []byte{0xff, 0xff, 0xff, 0xff}) // after all folder/device files

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
			oldName = db.deviceKeyName(dbi.Key())
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

func (db *Instance) replace(folder, device []byte, fs []protocol.FileInfo, localSize, globalSize *sizeTracker) int64 {
	// TODO: Return the remaining maxLocalVer?
	return db.genericReplace(folder, device, fs, localSize, globalSize, func(t readWriteTransaction, folder, device, name []byte, dbi iterator.Iterator) int64 {
		// Database has a file that we are missing. Remove it.
		l.Debugf("delete; folder=%q device=%v name=%q", folder, protocol.DeviceIDFromBytes(device), name)
		t.removeFromGlobal(folder, device, name, globalSize)
		t.Delete(dbi.Key())
		return 0
	})
}

func (db *Instance) updateFiles(folder, device []byte, fs []protocol.FileInfo, localSize, globalSize *sizeTracker) int64 {
	t := db.newReadWriteTransaction()
	defer t.close()

	var maxLocalVer int64
	var fk []byte
	isLocalDevice := bytes.Equal(device, protocol.LocalDeviceID[:])
	for _, f := range fs {
		name := []byte(f.Name)
		fk = db.deviceKeyInto(fk[:cap(fk)], folder, device, name)
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

func (db *Instance) withHave(folder, device, prefix []byte, truncate bool, fn Iterator) {
	t := db.newReadOnlyTransaction()
	defer t.close()

	dbi := t.NewIterator(util.BytesPrefix(db.deviceKey(folder, device, prefix)[:1+64+32+len(prefix)]), nil)
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

func (db *Instance) withAllFolderTruncated(folder []byte, fn func(device []byte, f FileInfoTruncated) bool) {
	start := db.deviceKey(folder, nil, nil)                                                  // before all folder/device files
	limit := db.deviceKey(folder, protocol.LocalDeviceID[:], []byte{0xff, 0xff, 0xff, 0xff}) // after all folder/device files

	t := db.newReadWriteTransaction()
	defer t.close()

	dbi := t.NewIterator(&util.Range{Start: start, Limit: limit}, nil)
	defer dbi.Release()

	for dbi.Next() {
		device := db.deviceKeyDevice(dbi.Key())
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

func (db *Instance) getFile(folder, device, file []byte) (protocol.FileInfo, bool) {
	return getFile(db, db.deviceKey(folder, device, file))
}

func (db *Instance) getGlobal(folder, file []byte, truncate bool) (FileIntf, bool) {
	k := db.globalKey(folder, file)

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

	k = db.deviceKey(folder, vl.versions[0].device, file)
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

func (db *Instance) withGlobal(folder, prefix []byte, truncate bool, fn Iterator) {
	t := db.newReadOnlyTransaction()
	defer t.close()

	dbi := t.NewIterator(util.BytesPrefix(db.globalKey(folder, prefix)), nil)
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
		name := db.globalKeyName(dbi.Key())
		fk = db.deviceKeyInto(fk[:cap(fk)], folder, vl.versions[0].device, name)
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

func (db *Instance) availability(folder, file []byte) []protocol.DeviceID {
	k := db.globalKey(folder, file)
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

func (db *Instance) withNeed(folder, device []byte, truncate bool, fn Iterator) {
	start := db.globalKey(folder, nil)
	limit := db.globalKey(folder, []byte{0xff, 0xff, 0xff, 0xff})

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
			name := db.globalKeyName(dbi.Key())
			needVersion := vl.versions[0].version

		nextVersion:
			for i := range vl.versions {
				if !vl.versions[i].version.Equal(needVersion) {
					// We haven't found a valid copy of the file with the needed version.
					continue nextFile
				}
				fk = db.deviceKeyInto(fk[:cap(fk)], folder, vl.versions[i].device, name)
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

func (db *Instance) ListFolders() []string {
	t := db.newReadOnlyTransaction()
	defer t.close()

	dbi := t.NewIterator(util.BytesPrefix([]byte{KeyTypeGlobal}), nil)
	defer dbi.Release()

	folderExists := make(map[string]bool)
	for dbi.Next() {
		folder := string(db.globalKeyFolder(dbi.Key()))
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

func (db *Instance) dropFolder(folder []byte) {
	t := db.newReadOnlyTransaction()
	defer t.close()

	// Remove all items related to the given folder from the device->file bucket
	dbi := t.NewIterator(util.BytesPrefix([]byte{KeyTypeDevice}), nil)
	for dbi.Next() {
		itemFolder := db.deviceKeyFolder(dbi.Key())
		if bytes.Compare(folder, itemFolder) == 0 {
			db.Delete(dbi.Key(), nil)
		}
	}
	dbi.Release()

	// Remove all items related to the given folder from the global bucket
	dbi = t.NewIterator(util.BytesPrefix([]byte{KeyTypeGlobal}), nil)
	for dbi.Next() {
		itemFolder := db.globalKeyFolder(dbi.Key())
		if bytes.Compare(folder, itemFolder) == 0 {
			db.Delete(dbi.Key(), nil)
		}
	}
	dbi.Release()
}

func (db *Instance) checkGlobals(folder []byte, globalSize *sizeTracker) {
	t := db.newReadWriteTransaction()
	defer t.close()

	start := db.globalKey(folder, nil)
	limit := db.globalKey(folder, []byte{0xff, 0xff, 0xff, 0xff})
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

		name := db.globalKeyName(gk)
		var newVL versionList
		for i, version := range vl.versions {
			fk = db.deviceKeyInto(fk[:cap(fk)], folder, version.device, name)

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

// deviceKey returns a byte slice encoding the following information:
//	   keyTypeDevice (1 byte)
//	   folder (64 bytes)
//	   device (32 bytes)
//	   name (variable size)
func (db *Instance) deviceKey(folder, device, file []byte) []byte {
	return db.deviceKeyInto(nil, folder, device, file)
}

func (db *Instance) deviceKeyInto(k []byte, folder, device, file []byte) []byte {
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

func (db *Instance) deviceKeyName(key []byte) []byte {
	return key[1+64+32:]
}

func (db *Instance) deviceKeyFolder(key []byte) []byte {
	folder := key[1 : 1+64]
	izero := bytes.IndexByte(folder, 0)
	if izero < 0 {
		return folder
	}
	return folder[:izero]
}

func (db *Instance) deviceKeyDevice(key []byte) []byte {
	return key[1+64 : 1+64+32]
}

// globalKey returns a byte slice encoding the following information:
//	   keyTypeGlobal (1 byte)
//	   folder (64 bytes)
//	   name (variable size)
func (db *Instance) globalKey(folder, file []byte) []byte {
	k := make([]byte, 1+64+len(file))
	k[0] = KeyTypeGlobal
	if len(folder) > 64 {
		panic("folder name too long")
	}
	copy(k[1:], []byte(folder))
	copy(k[1+64:], []byte(file))
	return k
}

func (db *Instance) globalKeyName(key []byte) []byte {
	return key[1+64:]
}

func (db *Instance) globalKeyFolder(key []byte) []byte {
	folder := key[1 : 1+64]
	izero := bytes.IndexByte(folder, 0)
	if izero < 0 {
		return folder
	}
	return folder[:izero]
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

// A "better" version of leveldb's errors.IsCorrupted.
func leveldbIsCorrupted(err error) bool {
	switch {
	case err == nil:
		return false

	case errors.IsCorrupted(err):
		return true

	case strings.Contains(err.Error(), "corrupted"):
		return true
	}

	return false
}
