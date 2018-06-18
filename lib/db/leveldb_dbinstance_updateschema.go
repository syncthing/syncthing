// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package db

import (
	"strings"

	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syndtr/goleveldb/leveldb/util"
)

const dbVersion = 5

func (db *Instance) updateSchema() {
	miscDB := NewNamespacedKV(db, string(KeyTypeMiscData))
	prevVersion, _ := miscDB.Int64("dbVersion")

	if prevVersion >= dbVersion {
		return
	}

	if prevVersion < 1 {
		db.updateSchema0to1()
	}
	if prevVersion < 2 {
		db.updateSchema1to2()
	}
	if prevVersion < 3 {
		db.updateSchema2to3()
	}
	// This update fixes a problem that only exists in dbVersion 3.
	if prevVersion == 3 {
		db.updateSchema3to4()
	}
	if prevVersion < 5 {
		db.updateSchema4to5()
	}

	miscDB.PutInt64("dbVersion", dbVersion)
}

func (db *Instance) updateSchema0to1() {
	t := db.newReadWriteTransaction()
	defer t.close()

	dbi := t.NewIterator(util.BytesPrefix([]byte{KeyTypeDevice}), nil)
	defer dbi.Release()

	symlinkConv := 0
	changedFolders := make(map[string]struct{})
	ignAdded := 0
	meta := newMetadataTracker() // dummy metadata tracker
	var gk []byte

	for dbi.Next() {
		folder := db.deviceKeyFolder(dbi.Key())
		device := db.deviceKeyDevice(dbi.Key())
		name := db.deviceKeyName(dbi.Key())

		// Remove files with absolute path (see #4799)
		if strings.HasPrefix(string(name), "/") {
			if _, ok := changedFolders[string(folder)]; !ok {
				changedFolders[string(folder)] = struct{}{}
			}
			gk = db.globalKeyInto(gk, folder, name)
			t.removeFromGlobal(gk, folder, device, nil, nil)
			t.Delete(dbi.Key())
			t.checkFlush()
			continue
		}

		// Change SYMLINK_FILE and SYMLINK_DIRECTORY types to the current SYMLINK
		// type (previously SYMLINK_UNKNOWN). It does this for all devices, both
		// local and remote, and does not reset delta indexes. It shouldn't really
		// matter what the symlink type is, but this cleans it up for a possible
		// future when SYMLINK_FILE and SYMLINK_DIRECTORY are no longer understood.
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
			symlinkConv++
		}

		// Add invalid files to global list
		if f.IsInvalid() {
			gk = db.globalKeyInto(gk, folder, name)
			if t.updateGlobal(gk, folder, device, f, meta) {
				if _, ok := changedFolders[string(folder)]; !ok {
					changedFolders[string(folder)] = struct{}{}
				}
				ignAdded++
			}
		}
	}

	for folder := range changedFolders {
		db.dropFolderMeta([]byte(folder))
	}
}

// updateSchema1to2 introduces a sequenceKey->deviceKey bucket for local items
// to allow iteration in sequence order (simplifies sending indexes).
func (db *Instance) updateSchema1to2() {
	t := db.newReadWriteTransaction()
	defer t.close()

	var sk []byte
	var dk []byte
	for _, folderStr := range db.ListFolders() {
		folder := []byte(folderStr)
		db.withHave(folder, protocol.LocalDeviceID[:], nil, true, func(f FileIntf) bool {
			sk = db.sequenceKeyInto(sk, folder, f.SequenceNo())
			dk = db.deviceKeyInto(dk, folder, protocol.LocalDeviceID[:], []byte(f.FileName()))
			t.Put(sk, dk)
			t.checkFlush()
			return true
		})
	}
}

// updateSchema2to3 introduces a needKey->nil bucket for locally needed files.
func (db *Instance) updateSchema2to3() {
	t := db.newReadWriteTransaction()
	defer t.close()

	var nk []byte
	var dk []byte
	for _, folderStr := range db.ListFolders() {
		folder := []byte(folderStr)
		db.withGlobal(folder, nil, true, func(f FileIntf) bool {
			name := []byte(f.FileName())
			dk = db.deviceKeyInto(dk, folder, protocol.LocalDeviceID[:], name)
			var v protocol.Vector
			haveFile, ok := db.getFileTrunc(dk, true)
			if ok {
				v = haveFile.FileVersion()
			}
			if !need(f, ok, v) {
				return true
			}
			nk = t.db.needKeyInto(nk, folder, []byte(f.FileName()))
			t.Put(nk, nil)
			t.checkFlush()
			return true
		})
	}
}

// updateSchema3to4 resets the need bucket due a bug existing in dbVersion 3 /
// v0.14.49-rc.1
// https://github.com/syncthing/syncthing/issues/5007
func (db *Instance) updateSchema3to4() {
	t := db.newReadWriteTransaction()
	var nk []byte
	for _, folderStr := range db.ListFolders() {
		nk = db.needKeyInto(nk, []byte(folderStr), nil)
		t.deleteKeyPrefix(nk[:keyPrefixLen+keyFolderLen])
	}
	t.close()

	db.updateSchema2to3()
}

func (db *Instance) updateSchema4to5() {
	// For every local file with the Invalid bit set, clear the Invalid bit and
	// set LocalFlags = FlagLocalIgnored.

	t := db.newReadWriteTransaction()
	defer t.close()

	var dk []byte

	for _, folderStr := range db.ListFolders() {
		folder := []byte(folderStr)
		db.withHave(folder, protocol.LocalDeviceID[:], nil, false, func(f FileIntf) bool {
			if !f.IsInvalid() {
				return true
			}

			fi := f.(protocol.FileInfo)
			fi.RawInvalid = false
			fi.LocalFlags = protocol.FlagLocalIgnored
			bs, _ := fi.Marshal()

			dk = db.deviceKeyInto(dk, folder, protocol.LocalDeviceID[:], []byte(fi.Name))
			t.Put(dk, bs)

			t.checkFlush()
			return true
		})
	}
}
