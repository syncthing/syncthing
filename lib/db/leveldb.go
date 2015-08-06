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
	"runtime"
	"sort"

	"github.com/syncthing/protocol"
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

type dbWriter interface {
	Put([]byte, []byte)
	Delete([]byte)
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

type deletionHandler func(db dbReader, batch dbWriter, folder, device, name []byte, dbi iterator.Iterator) int64

func ldbGenericReplace(db *leveldb.DB, folder, device []byte, fs []protocol.FileInfo, deleteFn deletionHandler) int64 {
	runtime.GC()

	sort.Sort(fileList(fs)) // sort list on name, same as in the database

	start := deviceKey(folder, device, nil)                            // before all folder/device files
	limit := deviceKey(folder, device, []byte{0xff, 0xff, 0xff, 0xff}) // after all folder/device files

	batch := new(leveldb.Batch)
	if debugDB {
		l.Debugf("new batch %p", batch)
	}
	snap, err := db.GetSnapshot()
	if err != nil {
		panic(err)
	}
	if debugDB {
		l.Debugf("created snapshot %p", snap)
	}
	defer func() {
		if debugDB {
			l.Debugf("close snapshot %p", snap)
		}
		snap.Release()
	}()

	dbi := snap.NewIterator(&util.Range{Start: start, Limit: limit}, nil)
	defer dbi.Release()

	moreDb := dbi.Next()
	fsi := 0
	var maxLocalVer int64

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

		if debugDB {
			l.Debugf("generic replace; folder=%q device=%v moreFs=%v moreDb=%v cmp=%d newName=%q oldName=%q", folder, protocol.DeviceIDFromBytes(device), moreFs, moreDb, cmp, newName, oldName)
		}

		switch {
		case moreFs && (!moreDb || cmp == -1):
			if debugDB {
				l.Debugln("generic replace; missing - insert")
			}
			// Database is missing this file. Insert it.
			if lv := ldbInsert(batch, folder, device, fs[fsi]); lv > maxLocalVer {
				maxLocalVer = lv
			}
			if fs[fsi].IsInvalid() {
				ldbRemoveFromGlobal(snap, batch, folder, device, newName)
			} else {
				ldbUpdateGlobal(snap, batch, folder, device, fs[fsi])
			}
			fsi++

		case moreFs && moreDb && cmp == 0:
			// File exists on both sides - compare versions. We might get an
			// update with the same version and different flags if a device has
			// marked a file as invalid, so handle that too.
			if debugDB {
				l.Debugln("generic replace; exists - compare")
			}
			var ef FileInfoTruncated
			ef.UnmarshalXDR(dbi.Value())
			if !fs[fsi].Version.Equal(ef.Version) || fs[fsi].Flags != ef.Flags {
				if debugDB {
					l.Debugln("generic replace; differs - insert")
				}
				if lv := ldbInsert(batch, folder, device, fs[fsi]); lv > maxLocalVer {
					maxLocalVer = lv
				}
				if fs[fsi].IsInvalid() {
					ldbRemoveFromGlobal(snap, batch, folder, device, newName)
				} else {
					ldbUpdateGlobal(snap, batch, folder, device, fs[fsi])
				}
			} else if debugDB {
				l.Debugln("generic replace; equal - ignore")
			}

			fsi++
			moreDb = dbi.Next()

		case moreDb && (!moreFs || cmp == 1):
			if debugDB {
				l.Debugln("generic replace; exists - remove")
			}
			if lv := deleteFn(snap, batch, folder, device, oldName, dbi); lv > maxLocalVer {
				maxLocalVer = lv
			}
			moreDb = dbi.Next()
		}

		// Write out and reuse the batch every few records, to avoid the batch
		// growing too large and thus allocating unnecessarily much memory.
		if batch.Len() > batchFlushSize {
			if debugDB {
				l.Debugf("db.Write %p", batch)
			}

			err = db.Write(batch, nil)
			if err != nil {
				panic(err)
			}

			batch.Reset()
		}
	}

	if debugDB {
		l.Debugf("db.Write %p", batch)
	}
	err = db.Write(batch, nil)
	if err != nil {
		panic(err)
	}

	return maxLocalVer
}

func ldbReplace(db *leveldb.DB, folder, device []byte, fs []protocol.FileInfo) int64 {
	// TODO: Return the remaining maxLocalVer?
	return ldbGenericReplace(db, folder, device, fs, func(db dbReader, batch dbWriter, folder, device, name []byte, dbi iterator.Iterator) int64 {
		// Database has a file that we are missing. Remove it.
		if debugDB {
			l.Debugf("delete; folder=%q device=%v name=%q", folder, protocol.DeviceIDFromBytes(device), name)
		}
		ldbRemoveFromGlobal(db, batch, folder, device, name)
		if debugDB {
			l.Debugf("batch.Delete %p %x", batch, dbi.Key())
		}
		batch.Delete(dbi.Key())
		return 0
	})
}

func ldbUpdate(db *leveldb.DB, folder, device []byte, fs []protocol.FileInfo) int64 {
	runtime.GC()

	batch := new(leveldb.Batch)
	if debugDB {
		l.Debugf("new batch %p", batch)
	}
	snap, err := db.GetSnapshot()
	if err != nil {
		panic(err)
	}
	if debugDB {
		l.Debugf("created snapshot %p", snap)
	}
	defer func() {
		if debugDB {
			l.Debugf("close snapshot %p", snap)
		}
		snap.Release()
	}()

	var maxLocalVer int64
	var fk []byte
	for _, f := range fs {
		name := []byte(f.Name)
		fk = deviceKeyInto(fk[:cap(fk)], folder, device, name)
		if debugDB {
			l.Debugf("snap.Get %p %x", snap, fk)
		}
		bs, err := snap.Get(fk, nil)
		if err == leveldb.ErrNotFound {
			if lv := ldbInsert(batch, folder, device, f); lv > maxLocalVer {
				maxLocalVer = lv
			}
			if f.IsInvalid() {
				ldbRemoveFromGlobal(snap, batch, folder, device, name)
			} else {
				ldbUpdateGlobal(snap, batch, folder, device, f)
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
			if lv := ldbInsert(batch, folder, device, f); lv > maxLocalVer {
				maxLocalVer = lv
			}
			if f.IsInvalid() {
				ldbRemoveFromGlobal(snap, batch, folder, device, name)
			} else {
				ldbUpdateGlobal(snap, batch, folder, device, f)
			}
		}

		// Write out and reuse the batch every few records, to avoid the batch
		// growing too large and thus allocating unnecessarily much memory.
		if batch.Len() > batchFlushSize {
			if debugDB {
				l.Debugf("db.Write %p", batch)
			}

			err = db.Write(batch, nil)
			if err != nil {
				panic(err)
			}

			batch.Reset()
		}
	}

	if debugDB {
		l.Debugf("db.Write %p", batch)
	}
	err = db.Write(batch, nil)
	if err != nil {
		panic(err)
	}

	return maxLocalVer
}

func ldbInsert(batch dbWriter, folder, device []byte, file protocol.FileInfo) int64 {
	if debugDB {
		l.Debugf("insert; folder=%q device=%v %v", folder, protocol.DeviceIDFromBytes(device), file)
	}

	if file.LocalVersion == 0 {
		file.LocalVersion = clock(0)
	}

	name := []byte(file.Name)
	nk := deviceKey(folder, device, name)
	if debugDB {
		l.Debugf("batch.Put %p %x", batch, nk)
	}
	batch.Put(nk, file.MustMarshalXDR())

	return file.LocalVersion
}

// ldbUpdateGlobal adds this device+version to the version list for the given
// file. If the device is already present in the list, the version is updated.
// If the file does not have an entry in the global list, it is created.
func ldbUpdateGlobal(db dbReader, batch dbWriter, folder, device []byte, file protocol.FileInfo) bool {
	if debugDB {
		l.Debugf("update global; folder=%q device=%v file=%q version=%d", folder, protocol.DeviceIDFromBytes(device), file.Name, file.Version)
	}
	name := []byte(file.Name)
	gk := globalKey(folder, name)
	svl, err := db.Get(gk, nil)
	if err != nil && err != leveldb.ErrNotFound {
		panic(err)
	}

	var fl versionList

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
				fl.versions = append(fl.versions[:i], fl.versions[i+1:]...)
				break
			}
		}
	}

	nv := fileVersion{
		device:  device,
		version: file.Version,
	}

	// Find a position in the list to insert this file. The file at the front
	// of the list is the newer, the "global".
	for i := range fl.versions {
		switch fl.versions[i].version.Compare(file.Version) {
		case protocol.Equal, protocol.Lesser:
			// The version at this point in the list is equal to or lesser
			// ("older") than us. We insert ourselves in front of it.
			fl.versions = insertVersion(fl.versions, i, nv)
			goto done

		case protocol.ConcurrentLesser, protocol.ConcurrentGreater:
			// The version at this point is in conflict with us. We must pull
			// the actual file metadata to determine who wins. If we win, we
			// insert ourselves in front of the loser here. (The "Lesser" and
			// "Greater" in the condition above is just based on the device
			// IDs in the version vector, which is not the only thing we use
			// to determine the winner.)
			of, ok := ldbGet(db, folder, fl.versions[i].device, name)
			if !ok {
				panic("file referenced in version list does not exist")
			}
			if file.WinsConflict(of) {
				fl.versions = insertVersion(fl.versions, i, nv)
				goto done
			}
		}
	}

	// We didn't find a position for an insert above, so append to the end.
	fl.versions = append(fl.versions, nv)

done:
	if debugDB {
		l.Debugf("batch.Put %p %x", batch, gk)
		l.Debugf("new global after update: %v", fl)
	}
	batch.Put(gk, fl.MustMarshalXDR())

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
func ldbRemoveFromGlobal(db dbReader, batch dbWriter, folder, device, file []byte) {
	if debugDB {
		l.Debugf("remove from global; folder=%q device=%v file=%q", folder, protocol.DeviceIDFromBytes(device), file)
	}

	gk := globalKey(folder, file)
	svl, err := db.Get(gk, nil)
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

	for i := range fl.versions {
		if bytes.Compare(fl.versions[i].device, device) == 0 {
			fl.versions = append(fl.versions[:i], fl.versions[i+1:]...)
			break
		}
	}

	if len(fl.versions) == 0 {
		if debugDB {
			l.Debugf("batch.Delete %p %x", batch, gk)
		}
		batch.Delete(gk)
	} else {
		if debugDB {
			l.Debugf("batch.Put %p %x", batch, gk)
			l.Debugf("new global after remove: %v", fl)
		}
		batch.Put(gk, fl.MustMarshalXDR())
	}
}

func ldbWithHave(db *leveldb.DB, folder, device []byte, truncate bool, fn Iterator) {
	start := deviceKey(folder, device, nil)                            // before all folder/device files
	limit := deviceKey(folder, device, []byte{0xff, 0xff, 0xff, 0xff}) // after all folder/device files
	snap, err := db.GetSnapshot()
	if err != nil {
		panic(err)
	}
	if debugDB {
		l.Debugf("created snapshot %p", snap)
	}
	defer func() {
		if debugDB {
			l.Debugf("close snapshot %p", snap)
		}
		snap.Release()
	}()

	dbi := snap.NewIterator(&util.Range{Start: start, Limit: limit}, nil)
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

func ldbWithAllFolderTruncated(db *leveldb.DB, folder []byte, fn func(device []byte, f FileInfoTruncated) bool) {
	runtime.GC()

	start := deviceKey(folder, nil, nil)                                                  // before all folder/device files
	limit := deviceKey(folder, protocol.LocalDeviceID[:], []byte{0xff, 0xff, 0xff, 0xff}) // after all folder/device files
	snap, err := db.GetSnapshot()
	if err != nil {
		panic(err)
	}
	if debugDB {
		l.Debugf("created snapshot %p", snap)
	}
	defer func() {
		if debugDB {
			l.Debugf("close snapshot %p", snap)
		}
		snap.Release()
	}()

	dbi := snap.NewIterator(&util.Range{Start: start, Limit: limit}, nil)
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
			batch := new(leveldb.Batch)
			ldbRemoveFromGlobal(db, batch, folder, device, nil)
			batch.Delete(dbi.Key())
			db.Write(batch, nil)
			continue
		}

		if cont := fn(device, f); !cont {
			return
		}
	}
}

func ldbGet(db dbReader, folder, device, file []byte) (protocol.FileInfo, bool) {
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

func ldbGetGlobal(db *leveldb.DB, folder, file []byte, truncate bool) (FileIntf, bool) {
	k := globalKey(folder, file)
	snap, err := db.GetSnapshot()
	if err != nil {
		panic(err)
	}
	if debugDB {
		l.Debugf("created snapshot %p", snap)
	}
	defer func() {
		if debugDB {
			l.Debugf("close snapshot %p", snap)
		}
		snap.Release()
	}()

	if debugDB {
		l.Debugf("snap.Get %p %x", snap, k)
	}
	bs, err := snap.Get(k, nil)
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
	if debugDB {
		l.Debugf("snap.Get %p %x", snap, k)
	}
	bs, err = snap.Get(k, nil)
	if err != nil {
		panic(err)
	}

	fi, err := unmarshalTrunc(bs, truncate)
	if err != nil {
		panic(err)
	}
	return fi, true
}

func ldbWithGlobal(db *leveldb.DB, folder, prefix []byte, truncate bool, fn Iterator) {
	runtime.GC()

	snap, err := db.GetSnapshot()
	if err != nil {
		panic(err)
	}
	if debugDB {
		l.Debugf("created snapshot %p", snap)
	}
	defer func() {
		if debugDB {
			l.Debugf("close snapshot %p", snap)
		}
		snap.Release()
	}()

	dbi := snap.NewIterator(util.BytesPrefix(globalKey(folder, prefix)), nil)
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
		if debugDB {
			l.Debugf("snap.Get %p %x", snap, fk)
		}
		bs, err := snap.Get(fk, nil)
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

func ldbAvailability(db *leveldb.DB, folder, file []byte) []protocol.DeviceID {
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

func ldbWithNeed(db *leveldb.DB, folder, device []byte, truncate bool, fn Iterator) {
	runtime.GC()

	start := globalKey(folder, nil)
	limit := globalKey(folder, []byte{0xff, 0xff, 0xff, 0xff})
	snap, err := db.GetSnapshot()
	if err != nil {
		panic(err)
	}
	if debugDB {
		l.Debugf("created snapshot %p", snap)
	}
	defer func() {
		if debugDB {
			l.Debugf("close snapshot %p", snap)
		}
		snap.Release()
	}()

	dbi := snap.NewIterator(&util.Range{Start: start, Limit: limit}, nil)
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
				if debugDB {
					l.Debugf("snap.Get %p %x", snap, fk)
				}
				bs, err := snap.Get(fk, nil)
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

				if debugDB {
					l.Debugf("need folder=%q device=%v name=%q need=%v have=%v haveV=%d globalV=%d", folder, protocol.DeviceIDFromBytes(device), name, need, have, haveVersion, vl.versions[0].version)
				}

				if cont := fn(gf); !cont {
					return
				}

				// This file is handled, no need to look further in the version list
				continue nextFile
			}
		}
	}
}

func ldbListFolders(db *leveldb.DB) []string {
	runtime.GC()

	snap, err := db.GetSnapshot()
	if err != nil {
		panic(err)
	}
	if debugDB {
		l.Debugf("created snapshot %p", snap)
	}
	defer func() {
		if debugDB {
			l.Debugf("close snapshot %p", snap)
		}
		snap.Release()
	}()

	dbi := snap.NewIterator(util.BytesPrefix([]byte{KeyTypeGlobal}), nil)
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

func ldbDropFolder(db *leveldb.DB, folder []byte) {
	runtime.GC()

	snap, err := db.GetSnapshot()
	if err != nil {
		panic(err)
	}
	if debugDB {
		l.Debugf("created snapshot %p", snap)
	}
	defer func() {
		if debugDB {
			l.Debugf("close snapshot %p", snap)
		}
		snap.Release()
	}()

	// Remove all items related to the given folder from the device->file bucket
	dbi := snap.NewIterator(util.BytesPrefix([]byte{KeyTypeDevice}), nil)
	for dbi.Next() {
		itemFolder := deviceKeyFolder(dbi.Key())
		if bytes.Compare(folder, itemFolder) == 0 {
			db.Delete(dbi.Key(), nil)
		}
	}
	dbi.Release()

	// Remove all items related to the given folder from the global bucket
	dbi = snap.NewIterator(util.BytesPrefix([]byte{KeyTypeGlobal}), nil)
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

func ldbCheckGlobals(db *leveldb.DB, folder []byte) {
	defer runtime.GC()

	snap, err := db.GetSnapshot()
	if err != nil {
		panic(err)
	}
	if debugDB {
		l.Debugf("created snapshot %p", snap)
	}
	defer func() {
		if debugDB {
			l.Debugf("close snapshot %p", snap)
		}
		snap.Release()
	}()

	start := globalKey(folder, nil)
	limit := globalKey(folder, []byte{0xff, 0xff, 0xff, 0xff})
	dbi := snap.NewIterator(&util.Range{Start: start, Limit: limit}, nil)
	defer dbi.Release()

	batch := new(leveldb.Batch)
	if debugDB {
		l.Debugf("new batch %p", batch)
	}

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
		for _, version := range vl.versions {
			fk = deviceKeyInto(fk[:cap(fk)], folder, version.device, name)
			if debugDB {
				l.Debugf("snap.Get %p %x", snap, fk)
			}
			_, err := snap.Get(fk, nil)
			if err == leveldb.ErrNotFound {
				continue
			}
			if err != nil {
				panic(err)
			}
			newVL.versions = append(newVL.versions, version)
		}

		if len(newVL.versions) != len(vl.versions) {
			l.Infof("db repair: rewriting global version list for %x %x", gk[1:1+64], gk[1+64:])
			batch.Put(dbi.Key(), newVL.MustMarshalXDR())
		}
	}
	if debugDB {
		l.Infoln("db check completed for %q", folder)
	}
	db.Write(batch, nil)
}
