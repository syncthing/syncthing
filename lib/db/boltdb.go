// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package db

import (
	"bytes"
	"runtime"
	"sort"
	"time"

	"github.com/boltdb/bolt"
	"github.com/syncthing/protocol"
	"github.com/syncthing/syncthing/lib/sync"
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

type boltDeletionHandler func(folBuc, devBuc *bolt.Bucket, device, name []byte) int64

type BoltDB struct {
	*bolt.DB
	dirty  chan struct{}
	closed chan struct{}
}

/*
	"folders":
		<folder>:
			<device>:
				<file>: protocol.FileInfo
			"global":
				<file>: versionList
*/

var (
	globalBucketID = []byte("global")
	folderBucketID = []byte("folders")
)

func folderBucket(tx *bolt.Tx, folder []byte) *bolt.Bucket {
	return tx.Bucket(folderBucketID).Bucket(folder)
}

func folderDeviceBucket(tx *bolt.Tx, folder, device []byte) *bolt.Bucket {
	bkt, err := folderBucket(tx, folder).CreateBucketIfNotExists(device)
	if err != nil {
		panic(err)
	}
	return bkt
}

func NewBoltDB(path string) (*BoltDB, error) {
	dbh, err := bolt.Open(path, 0644, &bolt.Options{Timeout: time.Second})
	if err != nil {
		return nil, err
	}
	dbh.NoSync = true

	bdb := &BoltDB{
		DB:     dbh,
		dirty:  make(chan struct{}),
		closed: make(chan struct{}),
	}

	go bdb.committer()

	return bdb, nil
}

func (db *BoltDB) Close() error {
	close(db.closed)
	return db.DB.Close()
}

func (db *BoltDB) initFolder(folder []byte) {
	db.Update(func(tx *bolt.Tx) error {
		bkt, err := tx.CreateBucketIfNotExists(folderBucketID)
		if err != nil {
			panic(err)
		}
		_, err = bkt.CreateBucketIfNotExists(folder)
		if err != nil {
			panic(err)
		}
		return nil
	})
}

const syncInterval = 5 * time.Second

func (db *BoltDB) committer() {
	nextSync := time.NewTimer(syncInterval)
	timerRunning := true
	for {
		select {
		case <-nextSync.C:
			timerRunning = false
			if err := db.Sync(); err != nil {
				panic(err)
			}

		case <-db.dirty:
			if !timerRunning {
				nextSync.Reset(syncInterval)
				timerRunning = true
			}

		case <-db.closed:
			return
		}
	}
}

func (db *BoltDB) setDirty() {
	select {
	case db.dirty <- struct{}{}:
	case <-db.closed:
	}
}

func (db *BoltDB) genericReplace(folder, device []byte, fs []protocol.FileInfo, deleteFn boltDeletionHandler) int64 {
	runtime.GC()
	defer db.setDirty()

	sort.Sort(fileList(fs)) // sort list on name, same as in the database

	var maxLocalVer int64
	db.Update(func(tx *bolt.Tx) error {
		folBuc := folderBucket(tx, folder)
		devBuc := folderDeviceBucket(tx, folder, device)

		it := devBuc.Cursor()
		k, v := it.First()

		fsi := 0

		for {
			var newName, oldName []byte
			moreDb := k != nil
			moreFs := fsi < len(fs)

			if !moreDb && !moreFs {
				break
			}

			if moreFs {
				newName = []byte(fs[fsi].Name)
			}

			if moreDb {
				oldName = k
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
				if lv := db.insert(devBuc, fs[fsi]); lv > maxLocalVer {
					maxLocalVer = lv
				}
				if fs[fsi].IsInvalid() {
					db.removeFromGlobal(folBuc, device, newName)
				} else {
					db.updateGlobal(folBuc, device, fs[fsi])
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
				ef.UnmarshalXDR(v)
				if !fs[fsi].Version.Equal(ef.Version) || fs[fsi].Flags != ef.Flags {
					if debugDB {
						l.Debugln("generic replace; differs - insert")
					}
					if lv := db.insert(devBuc, fs[fsi]); lv > maxLocalVer {
						maxLocalVer = lv
					}
					if fs[fsi].IsInvalid() {
						db.removeFromGlobal(folBuc, device, newName)
					} else {
						db.updateGlobal(folBuc, device, fs[fsi])
					}
				} else if debugDB {
					l.Debugln("generic replace; equal - ignore")
				}

				fsi++
				k, v = it.Next()

			case moreDb && (!moreFs || cmp == 1):
				if debugDB {
					l.Debugln("generic replace; exists - remove")
				}
				if lv := deleteFn(folBuc, devBuc, device, oldName); lv > maxLocalVer {
					maxLocalVer = lv
				}
				k, v = it.Next()
			}
		}

		return nil
	})

	return maxLocalVer
}

func (db *BoltDB) replace(folder, device []byte, fs []protocol.FileInfo) int64 {
	// TODO: Return the remaining maxLocalVer?
	return db.genericReplace(folder, device, fs, func(folBuc, devBuc *bolt.Bucket, device, name []byte) int64 {
		// Database has a file that we are missing. Remove it.
		db.removeFromGlobal(folBuc, device, name)
		devBuc.Delete(name)
		return 0
	})
}

func (db *BoltDB) replaceWithDelete(folder, device []byte, fs []protocol.FileInfo, myID uint64) int64 {
	return db.genericReplace(folder, device, fs, func(folBuc, devBuc *bolt.Bucket, device, name []byte) int64 {
		var tf FileInfoTruncated
		err := tf.UnmarshalXDR(devBuc.Get(name))
		if err != nil {
			panic(err)
		}
		if !tf.IsDeleted() {
			ts := clock(tf.LocalVersion)
			f := protocol.FileInfo{
				Name:         tf.Name,
				Version:      tf.Version.Update(myID),
				LocalVersion: ts,
				Flags:        tf.Flags | protocol.FlagDeleted,
				Modified:     tf.Modified,
			}
			devBuc.Put(name, f.MustMarshalXDR())
			db.updateGlobal(folBuc, device, f)
			return ts
		}
		return 0
	})
}

func (db *BoltDB) update(folder, device []byte, fs []protocol.FileInfo) int64 {
	runtime.GC()
	defer db.setDirty()

	var maxLocalVer int64
	db.Update(func(tx *bolt.Tx) error {
		folBuc := folderBucket(tx, folder)
		devBuc := folderDeviceBucket(tx, folder, device)
		for _, f := range fs {
			name := []byte(f.Name)
			bs := devBuc.Get(name)
			if bs == nil {
				if lv := db.insert(devBuc, f); lv > maxLocalVer {
					maxLocalVer = lv
				}
				if f.IsInvalid() {
					db.removeFromGlobal(folBuc, device, name)
				} else {
					db.updateGlobal(folBuc, device, f)
				}
				continue
			}

			var ef FileInfoTruncated
			if err := ef.UnmarshalXDR(bs); err != nil {
				panic(err)
			}
			// Flags might change without the version being bumped when we set the
			// invalid flag on an existing file.
			if !ef.Version.Equal(f.Version) || ef.Flags != f.Flags {
				if lv := db.insert(devBuc, f); lv > maxLocalVer {
					maxLocalVer = lv
				}
				if f.IsInvalid() {
					db.removeFromGlobal(folBuc, device, name)
				} else {
					db.updateGlobal(folBuc, device, f)
				}
			}
		}

		return nil
	})

	return maxLocalVer
}

func (db *BoltDB) insert(devBuc *bolt.Bucket, file protocol.FileInfo) int64 {
	if file.LocalVersion == 0 {
		file.LocalVersion = clock(0)
	}

	devBuc.Put([]byte(file.Name), file.MustMarshalXDR())
	return file.LocalVersion
}

// ldbUpdateGlobal adds this device+version to the version list for the given
// file. If the device is already present in the list, the version is updated.
// If the file does not have an entry in the global list, it is created.
func (db *BoltDB) updateGlobal(folBuc *bolt.Bucket, device []byte, file protocol.FileInfo) bool {
	gloBuc, err := folBuc.CreateBucketIfNotExists(globalBucketID)
	if err != nil {
		panic(err)
	}

	name := []byte(file.Name)
	svl := gloBuc.Get(name)

	var fl versionList

	// Remove the device from the current version list
	if svl != nil {
		err := fl.UnmarshalXDR(svl)
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
			of, ok := getFromBucket(folBuc, fl.versions[i].device, name)
			if !ok {
				panic("file referenced in version list does not exist")
			}
			if file.WinsConflict(of) {
				fl.versions = insertVersion(fl.versions, i, nv)
				goto done
			}
		}
	}

	fl.versions = append(fl.versions, nv)

done:
	gloBuc.Put(name, fl.MustMarshalXDR())

	return true
}

func insertVersion(vl []fileVersion, i int, v fileVersion) []fileVersion {
	t := append(vl, fileVersion{})
	copy(t[i+1:], t[i:])
	t[i] = v
	return t
}

func getFromBucket(folBuc *bolt.Bucket, device, file []byte) (fi protocol.FileInfo, ok bool) {
	devBuc := folBuc.Bucket(device)
	if devBuc == nil {
		return
	}
	bs := devBuc.Get(file)
	if bs == nil {
		return
	}

	if err := fi.UnmarshalXDR(bs); err != nil {
		panic(err)
	}

	ok = true
	return
}

// ldbRemoveFromGlobal removes the device from the global version list for the
// given file. If the version list is empty after this, the file entry is
// removed entirely.
func (db *BoltDB) removeFromGlobal(folBuc *bolt.Bucket, device, file []byte) {
	gloBuc, err := folBuc.CreateBucketIfNotExists(globalBucketID)
	if err != nil {
		panic(err)
	}
	svl := gloBuc.Get(file)
	if svl == nil {
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
		gloBuc.Delete(file)
	} else {
		gloBuc.Put(file, fl.MustMarshalXDR())
	}
}

func (db *BoltDB) withHave(folder, device []byte, truncate bool, fn Iterator) {
	db.View(func(tx *bolt.Tx) error {
		devBuc := folderBucket(tx, folder).Bucket(device)
		if devBuc == nil {
			return nil
		}

		c := devBuc.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			f, err := unmarshalTrunc(v, truncate)
			if err != nil {
				panic(err)
			}
			if cont := fn(f); !cont {
				return nil
			}
		}
		return nil
	})
}

func (db *BoltDB) withAllFolderTruncated(folder []byte, fn func(device []byte, f FileInfoTruncated) bool) {
	runtime.GC()

	db.View(func(tx *bolt.Tx) error {
		folBuc := folderBucket(tx, folder)
		if folBuc == nil {
			return nil
		}

		fc := folBuc.Cursor()
		for fk, fv := fc.First(); fk != nil; fk, fv = fc.Next() {
			if bytes.Compare(fk, globalBucketID) == 0 {
				continue
			}

			if fv != nil {
				// This is a top level value directly under a folder. Should
				// not happen.
				l.Debugf("%x", fk)
				l.Debugf("%x", fv)
				panic("top level value?")
			}

			devBuc := folBuc.Bucket(fk)
			dc := devBuc.Cursor()
			for dk, dv := dc.First(); dk != nil; dk, dv = dc.Next() {

				var f FileInfoTruncated
				err := f.UnmarshalXDR(dv)
				if err != nil {
					panic(err)
				}

				if cont := fn(fk, f); !cont {
					return nil
				}
			}
		}

		return nil
	})
}

func (db *BoltDB) get(folder, device, file []byte) (fi protocol.FileInfo, ok bool) {
	db.View(func(tx *bolt.Tx) error {
		folBuc := folderBucket(tx, folder)
		if folBuc == nil {
			return nil
		}
		devBuc := folBuc.Bucket(device)
		if devBuc == nil {
			return nil
		}
		bs := devBuc.Get(file)
		if bs == nil {
			return nil
		}

		if err := fi.UnmarshalXDR(bs); err != nil {
			panic(err)
		}

		ok = true
		return nil
	})

	return
}

func (db *BoltDB) getGlobal(folder, file []byte, truncate bool) (fi FileIntf, ok bool) {
	db.View(func(tx *bolt.Tx) error {
		folBuc := folderBucket(tx, folder)
		if folBuc == nil {
			return nil
		}
		gloBuc := folBuc.Bucket(globalBucketID)
		if gloBuc == nil {
			return nil
		}

		bs := gloBuc.Get(file)
		if bs == nil {
			return nil
		}

		var vl versionList
		err := vl.UnmarshalXDR(bs)
		if err != nil {
			panic(err)
		}
		if len(vl.versions) == 0 {
			l.Debugf("%x", folder)
			l.Debugf("%x", file)
			panic("no versions?")
		}

		// nil pointer exception here if the db is corrupt
		bs = folBuc.Bucket(vl.versions[0].device).Get(file)

		fi, err = unmarshalTrunc(bs, truncate)
		if err != nil {
			panic(err)
		}

		ok = true
		return nil
	})

	return
}

func (db *BoltDB) withGlobal(folder, prefix []byte, truncate bool, fn Iterator) {
	runtime.GC()

	db.View(func(tx *bolt.Tx) error {
		folBuc := folderBucket(tx, folder)
		if folBuc == nil {
			return nil
		}
		gloBuc := folBuc.Bucket(globalBucketID)
		if gloBuc == nil {
			return nil
		}

		c := gloBuc.Cursor()
		for name, v := c.Seek(prefix); name != nil; name, v = c.Next() {
			if !bytes.HasPrefix(name, prefix) {
				break
			}

			var vl versionList
			err := vl.UnmarshalXDR(v)
			if err != nil {
				panic(err)
			}
			if len(vl.versions) == 0 {
				l.Debugf("%x", name)
				l.Debugf("%x", v)
				panic("no versions?")
			}

			bs := folBuc.Bucket(vl.versions[0].device).Get(name)
			if bs == nil {
				l.Debugf("folder: %q (%x)", folder, folder)
				l.Debugf("name: %q (%x)", name, name)
				l.Debugf("vl: %v", vl)
				l.Debugf("vl.versions[0].device: %x", vl.versions[0].device)
				panic("global file missing")
			}

			f, err := unmarshalTrunc(bs, truncate)
			if err != nil {
				panic(err)
			}

			if cont := fn(f); !cont {
				return nil
			}
		}

		return nil
	})
}

func (db *BoltDB) availability(folder, file []byte) (devs []protocol.DeviceID) {
	db.View(func(tx *bolt.Tx) error {
		folBuc := folderBucket(tx, folder)
		if folBuc == nil {
			return nil
		}
		gloBuc := folBuc.Bucket(globalBucketID)
		if gloBuc == nil {
			return nil
		}

		bs := gloBuc.Get(file)
		if bs == nil {
			return nil
		}

		var vl versionList
		if err := vl.UnmarshalXDR(bs); err != nil {
			panic(err)
		}

		for _, v := range vl.versions {
			if !v.version.Equal(vl.versions[0].version) {
				break
			}
			n := protocol.DeviceIDFromBytes(v.device)
			devs = append(devs, n)
		}

		return nil
	})

	return
}

func (db *BoltDB) withNeed(folder, device []byte, truncate bool, fn Iterator) {
	db.withNeedWritable(folder, device, truncate, false, func(fi FileIntf, _ *bolt.Tx) (bool, error) {
		return fn(fi), nil
	})
}

func (db *BoltDB) overrideRemoteChanges(folder []byte) int64 {
	var maxLocalVer int64
	db.withNeedWritable(folder, protocol.LocalDeviceID[:], false, true, func(fi FileIntf, tx *bolt.Tx) (bool, error) {
		need := fi.(protocol.FileInfo)

		folBuc := folderBucket(tx, folder)
		locBuc := folderDeviceBucket(tx, folder, protocol.LocalDeviceID[:])
		bs := locBuc.Get([]byte(need.Name))
		if bs == nil {
			// We don't have this file
			need.Blocks = nil
			need.Flags |= protocol.FlagDeleted
			need.Modified = time.Now().Unix()
			need.Version = need.Version.Update(protocol.LocalDeviceID.Short()) // should be our real short device id?
		} else {
			var have protocol.FileInfo
			if err := have.UnmarshalXDR(bs); err != nil {
				panic(err)
			}
			have.Version = have.Version.Merge(need.Version).Update(protocol.LocalDeviceID.Short())
			need = have
		}
		need.LocalVersion = 0 // Will be set by insert()

		if lv := db.insert(locBuc, need); lv > maxLocalVer {
			maxLocalVer = lv
		}
		db.updateGlobal(folBuc, protocol.LocalDeviceID[:], need)

		return true, nil
	})

	return maxLocalVer
}

// withNeedWriteable iterates over all files in the need list, potentially in
// a writable transaction. The callback is passed the needed file and the
// current bolt transaction.
func (db *BoltDB) withNeedWritable(folder, device []byte, truncate bool, writeable bool, fn func(f FileIntf, tx *bolt.Tx) (bool, error)) {
	runtime.GC()

	transaction := db.View
	if writeable {
		transaction = db.Update
	}

	transaction(func(tx *bolt.Tx) error {
		folBuc := folderBucket(tx, folder)
		if folBuc == nil {
			return nil
		}
		gloBuc := folBuc.Bucket(globalBucketID)
		if gloBuc == nil {
			return nil
		}

		c := gloBuc.Cursor()

	nextFile:
		for file, v := c.First(); file != nil; file, v = c.Next() {
			var vl versionList
			err := vl.UnmarshalXDR(v)
			if err != nil {
				l.Debugf("%x", file)
				l.Debugf("%x", v)
				panic(err)
			}
			if len(vl.versions) == 0 {
				l.Debugf("%x", file)
				l.Debugf("%x", v)
				panic("no versions?")
			}

			have := false // If we have the file, any version
			need := false // If we have a lower version of the file
			for _, v := range vl.versions {
				if bytes.Compare(v.device, device) == 0 {
					have = true
					// XXX: This marks Concurrent (i.e. conflicting) changes as
					// needs. Maybe we should do that, but it needs special
					// handling in the puller.
					need = !v.version.GreaterEqual(vl.versions[0].version)
					break
				}
			}

			if need || !have {
				needVersion := vl.versions[0].version

			nextVersion:
				for i := range vl.versions {
					if !vl.versions[i].version.Equal(needVersion) {
						// We haven't found a valid copy of the file with the needed version.
						continue nextFile
					}

					bs := folBuc.Bucket(vl.versions[i].device).Get(file)
					if bs == nil {
						var id protocol.DeviceID
						copy(id[:], device)
						l.Debugf("device: %v", id)
						l.Debugf("need: %v, have: %v", need, have)
						l.Debugf("vl: %v", vl)
						l.Debugf("i: %v", i)
						l.Debugf("file: %q (%x)", file, file)
						panic("not found")
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

					if cont, err := fn(gf, tx); err != nil {
						return err
					} else if !cont {
						return nil
					}

					// This file is handled, no need to look further in the version list
					continue nextFile
				}
			}
		}
		return nil
	})
}

func (db *BoltDB) listFolders() []string {
	runtime.GC()

	folderExists := make(map[string]bool)
	db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(folderBucketID)
		if bkt == nil {
			return nil
		}
		c := bkt.Cursor()
		for folder, v := c.First(); folder != nil; folder, v = c.Next() {
			if v != nil {
				l.Debugf("%x", folder)
				l.Debugf("%x", v)
				panic("top level value")
			}

			fStr := string(folder)
			if !folderExists[fStr] {
				folderExists[fStr] = true
			}
		}
		return nil
	})

	folders := make([]string, 0, len(folderExists))
	for k := range folderExists {
		folders = append(folders, k)
	}

	sort.Strings(folders)
	return folders
}

func (db *BoltDB) dropFolder(folder []byte) {
	db.Update(func(tx *bolt.Tx) error {
		if err := tx.Bucket(folderBucketID).DeleteBucket(folder); err != nil {
			panic(err)
		}
		return nil
	})
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
