// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package db

import (
	"bytes"
	"encoding/binary"
	"os"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/sync"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/errors"
	"github.com/syndtr/goleveldb/leveldb/iterator"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/storage"
	"github.com/syndtr/goleveldb/leveldb/util"
)

type deletionHandler func(t readWriteTransaction, folder, device, name []byte, dbi iterator.Iterator)

type Instance struct {
	committed int64 // this must be the first attribute in the struct to ensure 64 bit alignment on 32 bit plaforms
	*leveldb.DB
	location  string
	folderIdx *smallIndex
	deviceIdx *smallIndex
}

const (
	keyPrefixLen = 1
	keyFolderLen = 4 // indexed
	keyDeviceLen = 4 // indexed
	keyHashLen   = 32
)

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

	return newDBInstance(db, file), nil
}

func OpenMemory() *Instance {
	db, _ := leveldb.Open(storage.NewMemStorage(), nil)
	return newDBInstance(db, "<memory>")
}

func newDBInstance(db *leveldb.DB, location string) *Instance {
	i := &Instance{
		DB:       db,
		location: location,
	}
	i.folderIdx = newSmallIndex(i, []byte{KeyTypeFolderIdx})
	i.deviceIdx = newSmallIndex(i, []byte{KeyTypeDeviceIdx})
	return i
}

// Committed returns the number of items committed to the database since startup
func (db *Instance) Committed() int64 {
	return atomic.LoadInt64(&db.committed)
}

// Location returns the filesystem path where the database is stored
func (db *Instance) Location() string {
	return db.location
}

func (db *Instance) updateFiles(folder, device []byte, fs []protocol.FileInfo, meta *metadataTracker) {
	t := db.newReadWriteTransaction()
	defer t.close()

	var fk []byte
	for _, f := range fs {
		name := []byte(f.Name)
		fk = db.deviceKeyInto(fk[:cap(fk)], folder, device, name)

		// Get and unmarshal the file entry. If it doesn't exist or can't be
		// unmarshalled we'll add it as a new entry.
		bs, err := t.Get(fk, nil)
		var ef FileInfoTruncated
		if err == nil {
			err = ef.Unmarshal(bs)
		}

		// The Invalid flag might change without the version being bumped.
		if err == nil && ef.Version.Equal(f.Version) && ef.Invalid == f.Invalid {
			continue
		}

		devID := protocol.DeviceIDFromBytes(device)
		if err == nil {
			meta.removeFile(devID, ef)
		}
		meta.addFile(devID, f)

		t.insertFile(folder, device, f)
		t.updateGlobal(folder, device, f, meta)

		// Write out and reuse the batch every few records, to avoid the batch
		// growing too large and thus allocating unnecessarily much memory.
		t.checkFlush()
	}
}

func (db *Instance) withHave(folder, device, prefix []byte, truncate bool, fn Iterator) {
	t := db.newReadOnlyTransaction()
	defer t.close()

	dbi := t.NewIterator(util.BytesPrefix(db.deviceKey(folder, device, prefix)[:keyPrefixLen+keyFolderLen+keyDeviceLen+len(prefix)]), nil)
	defer dbi.Release()

	slashedPrefix := prefix
	if !bytes.HasSuffix(prefix, []byte{'/'}) {
		slashedPrefix = append(slashedPrefix, '/')
	}

	for dbi.Next() {
		name := db.deviceKeyName(dbi.Key())
		if len(prefix) > 0 && !bytes.Equal(name, prefix) && !bytes.HasPrefix(name, slashedPrefix) {
			return
		}

		// The iterator function may keep a reference to the unmarshalled
		// struct, which in turn references the buffer it was unmarshalled
		// from. dbi.Value() just returns an internal slice that it reuses, so
		// we need to copy it.
		f, err := unmarshalTrunc(append([]byte{}, dbi.Value()...), truncate)
		if err != nil {
			l.Debugln("unmarshal error:", err)
			continue
		}
		if cont := fn(f); !cont {
			return
		}
	}
}

func (db *Instance) withAllFolderTruncated(folder []byte, fn func(device []byte, f FileInfoTruncated) bool) {
	t := db.newReadWriteTransaction()
	defer t.close()

	dbi := t.NewIterator(util.BytesPrefix(db.deviceKey(folder, nil, nil)[:keyPrefixLen+keyFolderLen]), nil)
	defer dbi.Release()

	for dbi.Next() {
		device := db.deviceKeyDevice(dbi.Key())
		var f FileInfoTruncated
		// The iterator function may keep a reference to the unmarshalled
		// struct, which in turn references the buffer it was unmarshalled
		// from. dbi.Value() just returns an internal slice that it reuses, so
		// we need to copy it.
		err := f.Unmarshal(append([]byte{}, dbi.Value()...))
		if err != nil {
			l.Debugln("unmarshal error:", err)
			continue
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
	if err != nil {
		return nil, false
	}

	var vl VersionList
	err = vl.Unmarshal(bs)
	if err == leveldb.ErrNotFound {
		return nil, false
	}
	if err != nil {
		l.Debugln("unmarshal error:", k, err)
		return nil, false
	}
	if len(vl.Versions) == 0 {
		l.Debugln("no versions:", k)
		return nil, false
	}

	k = db.deviceKey(folder, vl.Versions[0].Device, file)
	bs, err = t.Get(k, nil)
	if err != nil {
		l.Debugln("surprise error:", k, err)
		return nil, false
	}

	fi, err := unmarshalTrunc(bs, truncate)
	if err != nil {
		l.Debugln("unmarshal error:", k, err)
		return nil, false
	}
	return fi, true
}

func (db *Instance) withGlobal(folder, prefix []byte, truncate bool, fn Iterator) {
	t := db.newReadOnlyTransaction()
	defer t.close()

	dbi := t.NewIterator(util.BytesPrefix(db.globalKey(folder, prefix)), nil)
	defer dbi.Release()

	slashedPrefix := prefix
	if !bytes.HasSuffix(prefix, []byte{'/'}) {
		slashedPrefix = append(slashedPrefix, '/')
	}

	var fk []byte
	for dbi.Next() {
		var vl VersionList
		err := vl.Unmarshal(dbi.Value())
		if err != nil {
			l.Debugln("unmarshal error:", err)
			continue
		}
		if len(vl.Versions) == 0 {
			l.Debugln("no versions:", dbi.Key())
			continue
		}

		name := db.globalKeyName(dbi.Key())
		if len(prefix) > 0 && !bytes.Equal(name, prefix) && !bytes.HasPrefix(name, slashedPrefix) {
			return
		}

		fk = db.deviceKeyInto(fk[:cap(fk)], folder, vl.Versions[0].Device, name)
		bs, err := t.Get(fk, nil)
		if err != nil {
			l.Debugln("surprise error:", err)
			continue
		}

		f, err := unmarshalTrunc(bs, truncate)
		if err != nil {
			l.Debugln("unmarshal error:", err)
			continue
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
		l.Debugln("surprise error:", err)
		return nil
	}

	var vl VersionList
	err = vl.Unmarshal(bs)
	if err != nil {
		l.Debugln("unmarshal error:", err)
		return nil
	}

	var devices []protocol.DeviceID
	for _, v := range vl.Versions {
		if !v.Version.Equal(vl.Versions[0].Version) {
			break
		}
		if v.Invalid {
			continue
		}
		n := protocol.DeviceIDFromBytes(v.Device)
		devices = append(devices, n)
	}

	return devices
}

func (db *Instance) withNeed(folder, device []byte, truncate bool, needAllInvalid bool, fn Iterator) {
	t := db.newReadOnlyTransaction()
	defer t.close()

	dbi := t.NewIterator(util.BytesPrefix(db.globalKey(folder, nil)[:keyPrefixLen+keyFolderLen]), nil)
	defer dbi.Release()

	var fk []byte
	for dbi.Next() {
		var vl VersionList
		err := vl.Unmarshal(dbi.Value())
		if err != nil {
			l.Debugln("unmarshal error:", err)
			continue
		}
		if len(vl.Versions) == 0 {
			l.Debugln("no versions:", dbi.Key())
			continue
		}

		have := false // If we have the file, any version
		need := false // If we have a lower version of the file
		var haveFileVersion FileVersion
		for _, v := range vl.Versions {
			if bytes.Equal(v.Device, device) {
				have = true
				haveFileVersion = v
				// We need invalid files regardless of version when
				// ignore patterns changed
				if v.Invalid && needAllInvalid {
					need = true
					break
				}
				// XXX: This marks Concurrent (i.e. conflicting) changes as
				// needs. Maybe we should do that, but it needs special
				// handling in the puller.
				need = !v.Version.GreaterEqual(vl.Versions[0].Version)
				break
			}
		}

		if have && !need {
			continue
		}

		name := db.globalKeyName(dbi.Key())
		needVersion := vl.Versions[0].Version
		needDevice := protocol.DeviceIDFromBytes(vl.Versions[0].Device)

		for i := range vl.Versions {
			if !vl.Versions[i].Version.Equal(needVersion) {
				// We haven't found a valid copy of the file with the needed version.
				break
			}

			if vl.Versions[i].Invalid {
				// The file is marked invalid, don't use it.
				continue
			}

			fk = db.deviceKeyInto(fk[:cap(fk)], folder, vl.Versions[i].Device, name)
			bs, err := t.Get(fk, nil)
			if err != nil {
				l.Debugln("surprise error:", err)
				continue
			}

			gf, err := unmarshalTrunc(bs, truncate)
			if err != nil {
				l.Debugln("unmarshal error:", err)
				continue
			}

			if gf.IsDeleted() && !have {
				// We don't need deleted files that we don't have
				break
			}

			l.Debugf("need folder=%q device=%v name=%q need=%v have=%v invalid=%v haveV=%v globalV=%v globalDev=%v", folder, protocol.DeviceIDFromBytes(device), name, need, have, haveFileVersion.Invalid, haveFileVersion.Version, needVersion, needDevice)

			if cont := fn(gf); !cont {
				return
			}

			// This file is handled, no need to look further in the version list
			break
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
		folder, ok := db.globalKeyFolder(dbi.Key())
		if ok && !folderExists[string(folder)] {
			folderExists[string(folder)] = true
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
		if bytes.Equal(folder, itemFolder) {
			db.Delete(dbi.Key(), nil)
		}
	}
	dbi.Release()

	// Remove all items related to the given folder from the global bucket
	dbi = t.NewIterator(util.BytesPrefix([]byte{KeyTypeGlobal}), nil)
	for dbi.Next() {
		itemFolder, ok := db.globalKeyFolder(dbi.Key())
		if ok && bytes.Equal(folder, itemFolder) {
			db.Delete(dbi.Key(), nil)
		}
	}
	dbi.Release()
}

func (db *Instance) dropDeviceFolder(device, folder []byte, globalSize *metadataTracker) {
	t := db.newReadWriteTransaction()
	defer t.close()

	dbi := t.NewIterator(util.BytesPrefix(db.deviceKey(folder, device, nil)), nil)
	defer dbi.Release()

	for dbi.Next() {
		key := dbi.Key()
		name := db.deviceKeyName(key)
		t.removeFromGlobal(folder, device, name, globalSize)
		t.Delete(key)
		t.checkFlush()
	}
}

func (db *Instance) checkGlobals(folder []byte, globalSize *metadataTracker) {
	t := db.newReadWriteTransaction()
	defer t.close()

	dbi := t.NewIterator(util.BytesPrefix(db.globalKey(folder, nil)[:keyPrefixLen+keyFolderLen]), nil)
	defer dbi.Release()

	var fk []byte
	for dbi.Next() {
		gk := dbi.Key()
		var vl VersionList
		err := vl.Unmarshal(dbi.Value())
		if err != nil {
			l.Debugln("unmarshal error:", err)
			continue
		}

		// Check the global version list for consistency. An issue in previous
		// versions of goleveldb could result in reordered writes so that
		// there are global entries pointing to no longer existing files. Here
		// we find those and clear them out.

		name := db.globalKeyName(gk)
		var newVL VersionList
		for i, version := range vl.Versions {
			fk = db.deviceKeyInto(fk[:cap(fk)], folder, version.Device, name)

			_, err := t.Get(fk, nil)
			if err == leveldb.ErrNotFound {
				continue
			}
			if err != nil {
				l.Debugln("surprise error:", err)
				return
			}
			newVL.Versions = append(newVL.Versions, version)

			if i == 0 {
				if fi, ok := t.getFile(folder, version.Device, name); ok {
					globalSize.addFile(globalDeviceID, fi)
				}
			}
		}

		if len(newVL.Versions) != len(vl.Versions) {
			t.Put(dbi.Key(), mustMarshal(&newVL))
			t.checkFlush()
		}
	}
	l.Debugf("db check completed for %q", folder)
}

// ConvertSymlinkTypes should be run once only on an old database. It
// changes SYMLINK_FILE and SYMLINK_DIRECTORY types to the current SYMLINK
// type (previously SYMLINK_UNKNOWN). It does this for all devices, both
// local and remote, and does not reset delta indexes. It shouldn't really
// matter what the symlink type is, but this cleans it up for a possible
// future when SYMLINK_FILE and SYMLINK_DIRECTORY are no longer understood.
func (db *Instance) ConvertSymlinkTypes() {
	t := db.newReadWriteTransaction()
	defer t.close()

	dbi := t.NewIterator(util.BytesPrefix([]byte{KeyTypeDevice}), nil)
	defer dbi.Release()

	conv := 0
	for dbi.Next() {
		var f protocol.FileInfo
		if err := f.Unmarshal(dbi.Value()); err != nil {
			// probably can't happen
			continue
		}
		if f.Type == protocol.FileInfoTypeDeprecatedSymlinkDirectory || f.Type == protocol.FileInfoTypeDeprecatedSymlinkFile {
			f.Type = protocol.FileInfoTypeSymlink
			bs, err := f.Marshal()
			if err != nil {
				panic("can't happen: " + err.Error())
			}
			t.Put(dbi.Key(), bs)
			t.checkFlush()
			conv++
		}
	}

	l.Infof("Updated symlink type for %d index entries", conv)
}

// AddInvalidToGlobal searches for invalid files and adds them to the global list.
// Invalid files exist in the db if they once were not ignored and subsequently
// ignored. In the new system this is still valid, but invalid files must also be
// in the global list such that they cannot be mistaken for missing files.
func (db *Instance) AddInvalidToGlobal(folder, device []byte) int {
	t := db.newReadWriteTransaction()
	defer t.close()

	dbi := t.NewIterator(util.BytesPrefix(db.deviceKey(folder, device, nil)[:keyPrefixLen+keyFolderLen+keyDeviceLen]), nil)
	defer dbi.Release()

	changed := 0
	for dbi.Next() {
		var file protocol.FileInfo
		if err := file.Unmarshal(dbi.Value()); err != nil {
			// probably can't happen
			continue
		}
		if file.Invalid {
			changed++

			l.Debugf("add invalid to global; folder=%q device=%v file=%q version=%v", folder, protocol.DeviceIDFromBytes(device), file.Name, file.Version)

			// this is an adapted version of readWriteTransaction.updateGlobal
			name := []byte(file.Name)
			gk := t.db.globalKey(folder, name)

			var fl VersionList
			if svl, err := t.Get(gk, nil); err == nil {
				fl.Unmarshal(svl) // skip error, range handles success case
			}

			nv := FileVersion{
				Device:  device,
				Version: file.Version,
				Invalid: file.Invalid,
			}

			inserted := false
			// Find a position in the list to insert this file. The file at the front
			// of the list is the newer, the "global".
		insert:
			for i := range fl.Versions {
				switch fl.Versions[i].Version.Compare(file.Version) {
				case protocol.Equal:
					// Invalid files should go after a valid file of equal version
					if nv.Invalid {
						continue insert
					}
					fallthrough

				case protocol.Lesser:
					// The version at this point in the list is equal to or lesser
					// ("older") than us. We insert ourselves in front of it.
					fl.Versions = insertVersion(fl.Versions, i, nv)
					inserted = true
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
						inserted = true
						break insert
					}
				}
			}

			if !inserted {
				// We didn't find a position for an insert above, so append to the end.
				fl.Versions = append(fl.Versions, nv)
			}

			t.Put(gk, mustMarshal(&fl))
		}
	}

	return changed
}

// deviceKey returns a byte slice encoding the following information:
//	   keyTypeDevice (1 byte)
//	   folder (4 bytes)
//	   device (4 bytes)
//	   name (variable size)
func (db *Instance) deviceKey(folder, device, file []byte) []byte {
	return db.deviceKeyInto(nil, folder, device, file)
}

func (db *Instance) deviceKeyInto(k []byte, folder, device, file []byte) []byte {
	reqLen := keyPrefixLen + keyFolderLen + keyDeviceLen + len(file)
	if len(k) < reqLen {
		k = make([]byte, reqLen)
	}
	k[0] = KeyTypeDevice
	binary.BigEndian.PutUint32(k[keyPrefixLen:], db.folderIdx.ID(folder))
	binary.BigEndian.PutUint32(k[keyPrefixLen+keyFolderLen:], db.deviceIdx.ID(device))
	copy(k[keyPrefixLen+keyFolderLen+keyDeviceLen:], file)
	return k[:reqLen]
}

// deviceKeyName returns the device ID from the key
func (db *Instance) deviceKeyName(key []byte) []byte {
	return key[keyPrefixLen+keyFolderLen+keyDeviceLen:]
}

// deviceKeyFolder returns the folder name from the key
func (db *Instance) deviceKeyFolder(key []byte) []byte {
	folder, ok := db.folderIdx.Val(binary.BigEndian.Uint32(key[keyPrefixLen:]))
	if !ok {
		panic("bug: lookup of nonexistent folder ID")
	}
	return folder
}

// deviceKeyDevice returns the device ID from the key
func (db *Instance) deviceKeyDevice(key []byte) []byte {
	device, ok := db.deviceIdx.Val(binary.BigEndian.Uint32(key[keyPrefixLen+keyFolderLen:]))
	if !ok {
		panic("bug: lookup of nonexistent device ID")
	}
	return device
}

// globalKey returns a byte slice encoding the following information:
//	   keyTypeGlobal (1 byte)
//	   folder (4 bytes)
//	   name (variable size)
func (db *Instance) globalKey(folder, file []byte) []byte {
	k := make([]byte, keyPrefixLen+keyFolderLen+len(file))
	k[0] = KeyTypeGlobal
	binary.BigEndian.PutUint32(k[keyPrefixLen:], db.folderIdx.ID(folder))
	copy(k[keyPrefixLen+keyFolderLen:], file)
	return k
}

// globalKeyName returns the filename from the key
func (db *Instance) globalKeyName(key []byte) []byte {
	return key[keyPrefixLen+keyFolderLen:]
}

// globalKeyFolder returns the folder name from the key
func (db *Instance) globalKeyFolder(key []byte) ([]byte, bool) {
	return db.folderIdx.Val(binary.BigEndian.Uint32(key[keyPrefixLen:]))
}

func (db *Instance) getIndexID(device, folder []byte) protocol.IndexID {
	key := db.indexIDKey(device, folder)
	cur, err := db.Get(key, nil)
	if err != nil {
		return 0
	}

	var id protocol.IndexID
	if err := id.Unmarshal(cur); err != nil {
		return 0
	}

	return id
}

func (db *Instance) setIndexID(device, folder []byte, id protocol.IndexID) {
	key := db.indexIDKey(device, folder)
	bs, _ := id.Marshal() // marshalling can't fail
	if err := db.Put(key, bs, nil); err != nil {
		panic("storing index ID: " + err.Error())
	}
}

func (db *Instance) indexIDKey(device, folder []byte) []byte {
	k := make([]byte, keyPrefixLen+keyDeviceLen+keyFolderLen)
	k[0] = KeyTypeIndexID
	binary.BigEndian.PutUint32(k[keyPrefixLen:], db.deviceIdx.ID(device))
	binary.BigEndian.PutUint32(k[keyPrefixLen+keyDeviceLen:], db.folderIdx.ID(folder))
	return k
}

func (db *Instance) mtimesKey(folder []byte) []byte {
	prefix := make([]byte, 5) // key type + 4 bytes folder idx number
	prefix[0] = KeyTypeVirtualMtime
	binary.BigEndian.PutUint32(prefix[1:], db.folderIdx.ID(folder))
	return prefix
}

func (db *Instance) folderMetaKey(folder []byte) []byte {
	prefix := make([]byte, 5) // key type + 4 bytes folder idx number
	prefix[0] = KeyTypeFolderMeta
	binary.BigEndian.PutUint32(prefix[1:], db.folderIdx.ID(folder))
	return prefix
}

// DropDeltaIndexIDs removes all index IDs from the database. This will
// cause a full index transmission on the next connection.
func (db *Instance) DropDeltaIndexIDs() {
	db.dropPrefix([]byte{KeyTypeIndexID})
}

func (db *Instance) dropMtimes(folder []byte) {
	db.dropPrefix(db.mtimesKey(folder))
}

func (db *Instance) dropFolderMeta(folder []byte) {
	db.dropPrefix(db.folderMetaKey(folder))
}

func (db *Instance) dropPrefix(prefix []byte) {
	t := db.newReadWriteTransaction()
	defer t.close()

	dbi := t.NewIterator(util.BytesPrefix(prefix), nil)
	defer dbi.Release()

	for dbi.Next() {
		t.Delete(dbi.Key())
	}
}

func unmarshalTrunc(bs []byte, truncate bool) (FileIntf, error) {
	if truncate {
		var tf FileInfoTruncated
		err := tf.Unmarshal(bs)
		return tf, err
	}

	var tf protocol.FileInfo
	err := tf.Unmarshal(bs)
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

// A smallIndex is an in memory bidirectional []byte to uint32 map. It gives
// fast lookups in both directions and persists to the database. Don't use for
// storing more items than fit comfortably in RAM.
type smallIndex struct {
	db     *Instance
	prefix []byte
	id2val map[uint32]string
	val2id map[string]uint32
	nextID uint32
	mut    sync.Mutex
}

func newSmallIndex(db *Instance, prefix []byte) *smallIndex {
	idx := &smallIndex{
		db:     db,
		prefix: prefix,
		id2val: make(map[uint32]string),
		val2id: make(map[string]uint32),
		mut:    sync.NewMutex(),
	}
	idx.load()
	return idx
}

// load iterates over the prefix space in the database and populates the in
// memory maps.
func (i *smallIndex) load() {
	tr := i.db.newReadOnlyTransaction()
	it := tr.NewIterator(util.BytesPrefix(i.prefix), nil)
	for it.Next() {
		val := string(it.Value())
		id := binary.BigEndian.Uint32(it.Key()[len(i.prefix):])
		i.id2val[id] = val
		i.val2id[val] = id
		if id >= i.nextID {
			i.nextID = id + 1
		}
	}
	it.Release()
	tr.close()
}

// ID returns the index number for the given byte slice, allocating a new one
// and persisting this to the database if necessary.
func (i *smallIndex) ID(val []byte) uint32 {
	i.mut.Lock()
	// intentionally avoiding defer here as we want this call to be as fast as
	// possible in the general case (folder ID already exists). The map lookup
	// with the conversion of []byte to string is compiler optimized to not
	// copy the []byte, which is why we don't assign it to a temp variable
	// here.
	if id, ok := i.val2id[string(val)]; ok {
		i.mut.Unlock()
		return id
	}

	id := i.nextID
	i.nextID++

	valStr := string(val)
	i.val2id[valStr] = id
	i.id2val[id] = valStr

	key := make([]byte, len(i.prefix)+8) // prefix plus uint32 id
	copy(key, i.prefix)
	binary.BigEndian.PutUint32(key[len(i.prefix):], id)
	i.db.Put(key, val, nil)

	i.mut.Unlock()
	return id
}

// Val returns the value for the given index number, or (nil, false) if there
// is no such index number.
func (i *smallIndex) Val(id uint32) ([]byte, bool) {
	i.mut.Lock()
	val, ok := i.id2val[id]
	i.mut.Unlock()
	if !ok {
		return nil, false
	}

	return []byte(val), true
}
