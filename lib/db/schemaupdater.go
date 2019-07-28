// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package db

import (
	"fmt"
	"strings"

	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syndtr/goleveldb/leveldb/util"
)

// List of all dbVersion to dbMinSyncthingVersion pairs for convenience
//   0: v0.14.0
//   1: v0.14.46
//   2: v0.14.48
//   3: v0.14.49
//   4: v0.14.49
//   5: v0.14.49
//   6: v0.14.50
//   7: v0.14.53
const (
	dbVersion             = 7
	dbMinSyncthingVersion = "v0.14.53"
)

type databaseDowngradeError struct {
	minSyncthingVersion string
}

func (e databaseDowngradeError) Error() string {
	if e.minSyncthingVersion == "" {
		return "newer Syncthing required"
	}
	return fmt.Sprintf("Syncthing %s required", e.minSyncthingVersion)
}

func UpdateSchema(ll *Lowlevel) error {
	updater := &schemaUpdater{newInstance(ll)}
	return updater.updateSchema()
}

type schemaUpdater struct {
	*instance
}

func (db *schemaUpdater) updateSchema() error {
	miscDB := NewMiscDataNamespace(db.Lowlevel)
	prevVersion, _ := miscDB.Int64("dbVersion")

	if prevVersion > dbVersion {
		err := databaseDowngradeError{}
		if minSyncthingVersion, ok := miscDB.String("dbMinSyncthingVersion"); ok {
			err.minSyncthingVersion = minSyncthingVersion
		}
		return err
	}

	if prevVersion == dbVersion {
		return nil
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
	// This update fixes problems existing in versions 3 and 4
	if prevVersion == 3 || prevVersion == 4 {
		db.updateSchemaTo5()
	}
	if prevVersion < 6 {
		db.updateSchema5to6()
	}
	if prevVersion < 7 {
		db.updateSchema6to7()
	}

	miscDB.PutInt64("dbVersion", dbVersion)
	miscDB.PutString("dbMinSyncthingVersion", dbMinSyncthingVersion)

	return nil
}

func (db *schemaUpdater) updateSchema0to1() {
	t := db.newReadWriteTransaction()
	defer t.close()

	dbi := t.NewIterator(util.BytesPrefix([]byte{KeyTypeDevice}), nil)
	defer dbi.Release()

	symlinkConv := 0
	changedFolders := make(map[string]struct{})
	ignAdded := 0
	meta := newMetadataTracker() // dummy metadata tracker
	var gk, buf []byte

	for dbi.Next() {
		folder, ok := db.keyer.FolderFromDeviceFileKey(dbi.Key())
		if !ok {
			// not having the folder in the index is bad; delete and continue
			t.Delete(dbi.Key(), nil)
			continue
		}
		device, ok := db.keyer.DeviceFromDeviceFileKey(dbi.Key())
		if !ok {
			// not having the device in the index is bad; delete and continue
			t.Delete(dbi.Key(), nil)
			continue
		}
		name := db.keyer.NameFromDeviceFileKey(dbi.Key())

		// Remove files with absolute path (see #4799)
		if strings.HasPrefix(string(name), "/") {
			if _, ok := changedFolders[string(folder)]; !ok {
				changedFolders[string(folder)] = struct{}{}
			}
			gk = db.keyer.GenerateGlobalVersionKey(t, gk, folder, name)
			buf = removeFromGlobal(t, db.keyer, gk, buf, folder, device, nil, nil)
			t.Delete(dbi.Key(), nil)
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
			t.Put(dbi.Key(), bs, nil)
			symlinkConv++
		}

		// Add invalid files to global list
		if f.IsInvalid() {
			gk = db.keyer.GenerateGlobalVersionKey(t, gk, folder, name)
			if buf, ok = updateGlobal(t, db.keyer, gk, buf, folder, device, f, meta); ok {
				if _, ok = changedFolders[string(folder)]; !ok {
					changedFolders[string(folder)] = struct{}{}
				}
				ignAdded++
			}
		}
	}

	for folder := range changedFolders {
		deleteKeyPrefix(t, db.keyer.GenerateFolderMetaKey(t, nil, []byte(folder)))
	}
}

// updateSchema1to2 introduces a sequenceKey->deviceKey bucket for local items
// to allow iteration in sequence order (simplifies sending indexes).
func (db *schemaUpdater) updateSchema1to2() {
	t := db.newReadWriteTransaction()
	defer t.close()

	var sk []byte
	var dk []byte
	for _, folderStr := range db.ListFolders() {
		folder := []byte(folderStr)
		withHave(t, db.keyer, folder, protocol.LocalDeviceID[:], nil, true, func(f FileIntf) bool {
			sk = db.keyer.GenerateSequenceKey(t, sk, folder, f.SequenceNo())
			dk = db.keyer.GenerateDeviceFileKey(t, dk, folder, protocol.LocalDeviceID[:], []byte(f.FileName()))
			t.Put(sk, dk, nil)
			return true
		})
	}
}

// updateSchema2to3 introduces a needKey->nil bucket for locally needed files.
func (db *schemaUpdater) updateSchema2to3() {
	t := db.newReadWriteTransaction()
	defer t.close()

	var nk []byte
	var dk []byte
	for _, folderStr := range db.ListFolders() {
		folder := []byte(folderStr)
		withGlobal(t, db.keyer, folder, nil, true, func(f FileIntf) bool {
			name := []byte(f.FileName())
			dk = db.keyer.GenerateDeviceFileKey(t, dk, folder, protocol.LocalDeviceID[:], name)
			var v protocol.Vector
			haveFile, ok := getFileTrunc(t, dk, true)
			if ok {
				v = haveFile.FileVersion()
			}
			if !need(f, ok, v) {
				return true
			}
			nk = db.keyer.GenerateNeedFileKey(t, nk, folder, []byte(f.FileName()))
			t.Put(nk, nil, nil)
			return true
		})
	}
}

// updateSchemaTo5 resets the need bucket due to bugs existing in the v0.14.49
// release candidates (dbVersion 3 and 4)
// https://github.com/syncthing/syncthing/issues/5007
// https://github.com/syncthing/syncthing/issues/5053
func (db *schemaUpdater) updateSchemaTo5() {
	t := db.newReadWriteTransaction()
	var nk []byte
	for _, folderStr := range db.ListFolders() {
		nk = db.keyer.GenerateNeedFileKey(t, nk, []byte(folderStr), nil)
		deleteKeyPrefix(t, nk[:keyPrefixLen+keyFolderLen])
	}
	t.close()

	db.updateSchema2to3()
}

func (db *schemaUpdater) updateSchema5to6() {
	// For every local file with the Invalid bit set, clear the Invalid bit and
	// set LocalFlags = FlagLocalIgnored.

	t := db.newReadWriteTransaction()
	defer t.close()

	var dk []byte

	for _, folderStr := range db.ListFolders() {
		folder := []byte(folderStr)
		withHave(t, db.keyer, folder, protocol.LocalDeviceID[:], nil, false, func(f FileIntf) bool {
			if !f.IsInvalid() {
				return true
			}

			fi := f.(protocol.FileInfo)
			fi.RawInvalid = false
			fi.LocalFlags = protocol.FlagLocalIgnored
			bs, _ := fi.Marshal()

			dk = db.keyer.GenerateDeviceFileKey(t, dk, folder, protocol.LocalDeviceID[:], []byte(fi.Name))
			t.Put(dk, bs, nil)

			return true
		})
	}
}

// updateSchema6to7 checks whether all currently locally needed files are really
// needed and removes them if not.
func (db *schemaUpdater) updateSchema6to7() {
	t := db.newReadWriteTransaction()
	defer t.close()

	var gk []byte
	var nk []byte

	for _, folderStr := range db.ListFolders() {
		folder := []byte(folderStr)
		withNeedLocal(t, db.keyer, folder, false, func(f FileIntf) bool {
			name := []byte(f.FileName())
			global := f.(protocol.FileInfo)
			gk = db.keyer.GenerateGlobalVersionKey(t, gk, folder, name)
			svl, err := t.Get(gk, nil)
			if err != nil {
				// If there is no global list, we hardly need it.
				t.Delete(db.keyer.GenerateNeedFileKey(t, nk, folder, name), nil)
				return true
			}
			var fl VersionList
			err = fl.Unmarshal(svl)
			if err != nil {
				// This can't happen, but it's ignored everywhere else too,
				// so lets not act on it.
				return true
			}
			if localFV, haveLocalFV := fl.Get(protocol.LocalDeviceID[:]); !need(global, haveLocalFV, localFV.Version) {
				t.Delete(db.keyer.GenerateNeedFileKey(t, nk, folder, name), nil)
			}
			return true
		})
	}
}
