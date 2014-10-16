// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

package files

import (
	"bytes"
	"fmt"
	"runtime"
	"sort"
	"sync"

	"github.com/syncthing/syncthing/internal/lamport"
	"github.com/syncthing/syncthing/internal/protocol"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/iterator"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"
)

var (
	clockTick uint64
	clockMut  sync.Mutex
)

func clock(v uint64) uint64 {
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
	keyTypeDevice = iota
	keyTypeGlobal
	keyTypeBlock
)

type fileVersion struct {
	version uint64
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

// deviceKey returns a byte slice encoding the following information:
//	   keyTypeDevice (1 byte)
//	   folder (64 bytes)
//	   device (32 bytes)
//	   name (variable size)
func deviceKey(folder, device, file []byte) []byte {
	k := make([]byte, 1+64+32+len(file))
	k[0] = keyTypeDevice
	if len(folder) > 64 {
		panic("folder name too long")
	}
	copy(k[1:], []byte(folder))
	copy(k[1+64:], device[:])
	copy(k[1+64+32:], []byte(file))
	return k
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
	k[0] = keyTypeGlobal
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

type deletionHandler func(db dbReader, batch dbWriter, folder, device, name []byte, dbi iterator.Iterator) uint64

type fileIterator func(f protocol.FileIntf) bool

func ldbGenericReplace(db *leveldb.DB, folder, device []byte, fs []protocol.FileInfo, deleteFn deletionHandler) uint64 {
	runtime.GC()

	sort.Sort(fileList(fs)) // sort list on name, same as in the database

	start := deviceKey(folder, device, nil)                            // before all folder/device files
	limit := deviceKey(folder, device, []byte{0xff, 0xff, 0xff, 0xff}) // after all folder/device files

	batch := new(leveldb.Batch)
	snap, err := db.GetSnapshot()
	if err != nil {
		panic(err)
	}
	defer snap.Release()
	dbi := snap.NewIterator(&util.Range{Start: start, Limit: limit}, nil)
	defer dbi.Release()

	moreDb := dbi.Next()
	fsi := 0
	var maxLocalVer uint64

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

		if debug {
			l.Debugf("generic replace; folder=%q device=%v moreFs=%v moreDb=%v cmp=%d newName=%q oldName=%q", folder, protocol.DeviceIDFromBytes(device), moreFs, moreDb, cmp, newName, oldName)
		}

		switch {
		case moreFs && (!moreDb || cmp == -1):
			// Database is missing this file. Insert it.
			if lv := ldbInsert(batch, folder, device, fs[fsi]); lv > maxLocalVer {
				maxLocalVer = lv
			}
			if fs[fsi].IsInvalid() {
				ldbRemoveFromGlobal(snap, batch, folder, device, newName)
			} else {
				ldbUpdateGlobal(snap, batch, folder, device, newName, fs[fsi].Version)
			}
			fsi++

		case moreFs && moreDb && cmp == 0:
			// File exists on both sides - compare versions. We might get an
			// update with the same version and different flags if a device has
			// marked a file as invalid, so handle that too.
			var ef protocol.FileInfoTruncated
			ef.UnmarshalXDR(dbi.Value())
			if fs[fsi].Version > ef.Version ||
				(fs[fsi].Version == ef.Version && fs[fsi].Flags != ef.Flags) {
				if lv := ldbInsert(batch, folder, device, fs[fsi]); lv > maxLocalVer {
					maxLocalVer = lv
				}
				if fs[fsi].IsInvalid() {
					ldbRemoveFromGlobal(snap, batch, folder, device, newName)
				} else {
					ldbUpdateGlobal(snap, batch, folder, device, newName, fs[fsi].Version)
				}
			}
			fsi++
			moreDb = dbi.Next()

		case moreDb && (!moreFs || cmp == 1):
			if lv := deleteFn(snap, batch, folder, device, oldName, dbi); lv > maxLocalVer {
				maxLocalVer = lv
			}
			moreDb = dbi.Next()
		}
	}

	err = db.Write(batch, nil)
	if err != nil {
		panic(err)
	}

	return maxLocalVer
}

func ldbReplace(db *leveldb.DB, folder, device []byte, fs []protocol.FileInfo) uint64 {
	// TODO: Return the remaining maxLocalVer?
	return ldbGenericReplace(db, folder, device, fs, func(db dbReader, batch dbWriter, folder, device, name []byte, dbi iterator.Iterator) uint64 {
		// Database has a file that we are missing. Remove it.
		if debug {
			l.Debugf("delete; folder=%q device=%v name=%q", folder, protocol.DeviceIDFromBytes(device), name)
		}
		ldbRemoveFromGlobal(db, batch, folder, device, name)
		batch.Delete(dbi.Key())
		return 0
	})
}

func ldbReplaceWithDelete(db *leveldb.DB, folder, device []byte, fs []protocol.FileInfo) uint64 {
	return ldbGenericReplace(db, folder, device, fs, func(db dbReader, batch dbWriter, folder, device, name []byte, dbi iterator.Iterator) uint64 {
		var tf protocol.FileInfoTruncated
		err := tf.UnmarshalXDR(dbi.Value())
		if err != nil {
			panic(err)
		}
		if !tf.IsDeleted() {
			if debug {
				l.Debugf("mark deleted; folder=%q device=%v name=%q", folder, protocol.DeviceIDFromBytes(device), name)
			}
			ts := clock(tf.LocalVersion)
			f := protocol.FileInfo{
				Name:         tf.Name,
				Version:      lamport.Default.Tick(tf.Version),
				LocalVersion: ts,
				Flags:        tf.Flags | protocol.FlagDeleted,
				Modified:     tf.Modified,
			}
			batch.Put(dbi.Key(), f.MarshalXDR())
			ldbUpdateGlobal(db, batch, folder, device, deviceKeyName(dbi.Key()), f.Version)
			return ts
		}
		return 0
	})
}

func ldbUpdate(db *leveldb.DB, folder, device []byte, fs []protocol.FileInfo) uint64 {
	runtime.GC()

	batch := new(leveldb.Batch)
	snap, err := db.GetSnapshot()
	if err != nil {
		panic(err)
	}
	defer snap.Release()

	var maxLocalVer uint64
	for _, f := range fs {
		name := []byte(f.Name)
		fk := deviceKey(folder, device, name)
		bs, err := snap.Get(fk, nil)
		if err == leveldb.ErrNotFound {
			if lv := ldbInsert(batch, folder, device, f); lv > maxLocalVer {
				maxLocalVer = lv
			}
			if f.IsInvalid() {
				ldbRemoveFromGlobal(snap, batch, folder, device, name)
			} else {
				ldbUpdateGlobal(snap, batch, folder, device, name, f.Version)
			}
			continue
		}

		var ef protocol.FileInfoTruncated
		err = ef.UnmarshalXDR(bs)
		if err != nil {
			panic(err)
		}
		// Flags might change without the version being bumped when we set the
		// invalid flag on an existing file.
		if ef.Version != f.Version || ef.Flags != f.Flags {
			if lv := ldbInsert(batch, folder, device, f); lv > maxLocalVer {
				maxLocalVer = lv
			}
			if f.IsInvalid() {
				ldbRemoveFromGlobal(snap, batch, folder, device, name)
			} else {
				ldbUpdateGlobal(snap, batch, folder, device, name, f.Version)
			}
		}
	}

	err = db.Write(batch, nil)
	if err != nil {
		panic(err)
	}

	return maxLocalVer
}

func ldbInsert(batch dbWriter, folder, device []byte, file protocol.FileInfo) uint64 {
	if debug {
		l.Debugf("insert; folder=%q device=%v %v", folder, protocol.DeviceIDFromBytes(device), file)
	}

	if file.LocalVersion == 0 {
		file.LocalVersion = clock(0)
	}

	name := []byte(file.Name)
	nk := deviceKey(folder, device, name)
	batch.Put(nk, file.MarshalXDR())

	return file.LocalVersion
}

// ldbUpdateGlobal adds this device+version to the version list for the given
// file. If the device is already present in the list, the version is updated.
// If the file does not have an entry in the global list, it is created.
func ldbUpdateGlobal(db dbReader, batch dbWriter, folder, device, file []byte, version uint64) bool {
	if debug {
		l.Debugf("update global; folder=%q device=%v file=%q version=%d", folder, protocol.DeviceIDFromBytes(device), file, version)
	}
	gk := globalKey(folder, file)
	svl, err := db.Get(gk, nil)
	if err != nil && err != leveldb.ErrNotFound {
		panic(err)
	}

	var fl versionList
	nv := fileVersion{
		device:  device,
		version: version,
	}
	if svl != nil {
		err = fl.UnmarshalXDR(svl)
		if err != nil {
			panic(err)
		}

		for i := range fl.versions {
			if bytes.Compare(fl.versions[i].device, device) == 0 {
				if fl.versions[i].version == version {
					// No need to do anything
					return false
				}
				fl.versions = append(fl.versions[:i], fl.versions[i+1:]...)
				break
			}
		}
	}

	for i := range fl.versions {
		if fl.versions[i].version <= version {
			t := append(fl.versions, fileVersion{})
			copy(t[i+1:], t[i:])
			t[i] = nv
			fl.versions = t
			goto done
		}
	}

	fl.versions = append(fl.versions, nv)

done:
	batch.Put(gk, fl.MarshalXDR())

	return true
}

// ldbRemoveFromGlobal removes the device from the global version list for the
// given file. If the version list is empty after this, the file entry is
// removed entirely.
func ldbRemoveFromGlobal(db dbReader, batch dbWriter, folder, device, file []byte) {
	if debug {
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
		batch.Delete(gk)
	} else {
		batch.Put(gk, fl.MarshalXDR())
	}
}

func ldbWithHave(db *leveldb.DB, folder, device []byte, truncate bool, fn fileIterator) {
	start := deviceKey(folder, device, nil)                            // before all folder/device files
	limit := deviceKey(folder, device, []byte{0xff, 0xff, 0xff, 0xff}) // after all folder/device files
	snap, err := db.GetSnapshot()
	if err != nil {
		panic(err)
	}
	defer snap.Release()
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

func ldbWithAllFolderTruncated(db *leveldb.DB, folder []byte, fn func(device []byte, f protocol.FileInfoTruncated) bool) {
	runtime.GC()

	start := deviceKey(folder, nil, nil)                                                  // before all folder/device files
	limit := deviceKey(folder, protocol.LocalDeviceID[:], []byte{0xff, 0xff, 0xff, 0xff}) // after all folder/device files
	snap, err := db.GetSnapshot()
	if err != nil {
		panic(err)
	}
	defer snap.Release()
	dbi := snap.NewIterator(&util.Range{Start: start, Limit: limit}, nil)
	defer dbi.Release()

	for dbi.Next() {
		device := deviceKeyDevice(dbi.Key())
		var f protocol.FileInfoTruncated
		err := f.UnmarshalXDR(dbi.Value())
		if err != nil {
			panic(err)
		}
		if cont := fn(device, f); !cont {
			return
		}
	}
}

func ldbGet(db *leveldb.DB, folder, device, file []byte) protocol.FileInfo {
	nk := deviceKey(folder, device, file)
	bs, err := db.Get(nk, nil)
	if err == leveldb.ErrNotFound {
		return protocol.FileInfo{}
	}
	if err != nil {
		panic(err)
	}

	var f protocol.FileInfo
	err = f.UnmarshalXDR(bs)
	if err != nil {
		panic(err)
	}
	return f
}

func ldbGetGlobal(db *leveldb.DB, folder, file []byte) protocol.FileInfo {
	k := globalKey(folder, file)
	snap, err := db.GetSnapshot()
	if err != nil {
		panic(err)
	}
	defer snap.Release()

	bs, err := snap.Get(k, nil)
	if err == leveldb.ErrNotFound {
		return protocol.FileInfo{}
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
	bs, err = snap.Get(k, nil)
	if err != nil {
		panic(err)
	}

	var f protocol.FileInfo
	err = f.UnmarshalXDR(bs)
	if err != nil {
		panic(err)
	}
	return f
}

func ldbWithGlobal(db *leveldb.DB, folder []byte, truncate bool, fn fileIterator) {
	runtime.GC()

	start := globalKey(folder, nil)
	limit := globalKey(folder, []byte{0xff, 0xff, 0xff, 0xff})
	snap, err := db.GetSnapshot()
	if err != nil {
		panic(err)
	}
	defer snap.Release()
	dbi := snap.NewIterator(&util.Range{Start: start, Limit: limit}, nil)
	defer dbi.Release()

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
		fk := deviceKey(folder, vl.versions[0].device, name)
		bs, err := snap.Get(fk, nil)
		if err != nil {
			l.Debugf("folder: %q (%x)", folder, folder)
			l.Debugf("key: %q (%x)", dbi.Key(), dbi.Key())
			l.Debugf("vl: %v", vl)
			l.Debugf("name: %q (%x)", name, name)
			l.Debugf("fk: %q (%x)", fk, fk)
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
		if v.version != vl.versions[0].version {
			break
		}
		n := protocol.DeviceIDFromBytes(v.device)
		devices = append(devices, n)
	}

	return devices
}

func ldbWithNeed(db *leveldb.DB, folder, device []byte, truncate bool, fn fileIterator) {
	runtime.GC()

	start := globalKey(folder, nil)
	limit := globalKey(folder, []byte{0xff, 0xff, 0xff, 0xff})
	snap, err := db.GetSnapshot()
	if err != nil {
		panic(err)
	}
	defer snap.Release()
	dbi := snap.NewIterator(&util.Range{Start: start, Limit: limit}, nil)
	defer dbi.Release()

outer:
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
		var haveVersion uint64
		for _, v := range vl.versions {
			if bytes.Compare(v.device, device) == 0 {
				have = true
				haveVersion = v.version
				need = v.version < vl.versions[0].version
				break
			}
		}

		if need || !have {
			name := globalKeyName(dbi.Key())
			needVersion := vl.versions[0].version
		inner:
			for i := range vl.versions {
				if vl.versions[i].version != needVersion {
					// We haven't found a valid copy of the file with the needed version.
					continue outer
				}
				fk := deviceKey(folder, vl.versions[i].device, name)
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
					continue inner
				}

				if gf.IsDeleted() && !have {
					// We don't need deleted files that we don't have
					continue outer
				}

				if debug {
					l.Debugf("need folder=%q device=%v name=%q need=%v have=%v haveV=%d globalV=%d", folder, protocol.DeviceIDFromBytes(device), name, need, have, haveVersion, vl.versions[0].version)
				}

				if cont := fn(gf); !cont {
					return
				}

				// This file is handled, no need to look further in the version list
				continue outer
			}
		}
	}
}

func ldbListFolders(db *leveldb.DB) []string {
	runtime.GC()

	start := []byte{keyTypeGlobal}
	limit := []byte{keyTypeGlobal + 1}
	snap, err := db.GetSnapshot()
	if err != nil {
		panic(err)
	}
	defer snap.Release()
	dbi := snap.NewIterator(&util.Range{Start: start, Limit: limit}, nil)
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
	defer snap.Release()

	// Remove all items related to the given folder from the device->file bucket
	start := []byte{keyTypeDevice}
	limit := []byte{keyTypeDevice + 1}
	dbi := snap.NewIterator(&util.Range{Start: start, Limit: limit}, nil)
	for dbi.Next() {
		itemFolder := deviceKeyFolder(dbi.Key())
		if bytes.Compare(folder, itemFolder) == 0 {
			db.Delete(dbi.Key(), nil)
		}
	}
	dbi.Release()

	// Remove all items related to the given folder from the global bucket
	start = []byte{keyTypeGlobal}
	limit = []byte{keyTypeGlobal + 1}
	dbi = snap.NewIterator(&util.Range{Start: start, Limit: limit}, nil)
	for dbi.Next() {
		itemFolder := globalKeyFolder(dbi.Key())
		if bytes.Compare(folder, itemFolder) == 0 {
			db.Delete(dbi.Key(), nil)
		}
	}
	dbi.Release()
}

func unmarshalTrunc(bs []byte, truncate bool) (protocol.FileIntf, error) {
	if truncate {
		var tf protocol.FileInfoTruncated
		err := tf.UnmarshalXDR(bs)
		return tf, err
	} else {
		var tf protocol.FileInfo
		err := tf.UnmarshalXDR(bs)
		return tf, err
	}
}
