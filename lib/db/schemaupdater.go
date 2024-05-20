// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package db

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/syncthing/syncthing/lib/db/backend"
	"github.com/syncthing/syncthing/lib/protocol"
)

// dbMigrationVersion is for migrations that do not change the schema and thus
// do not put restrictions on downgrades (e.g. for repairs after a bugfix).
const (
	dbVersion             = 14
	dbMigrationVersion    = 20
	dbMinSyncthingVersion = "v1.9.0"
)

type migration struct {
	schemaVersion       int64
	migrationVersion    int64
	minSyncthingVersion string
	migration           func(prevSchema int) error
}

type databaseDowngradeError struct {
	minSyncthingVersion string
}

func (e *databaseDowngradeError) Error() string {
	if e.minSyncthingVersion == "" {
		return "newer Syncthing required"
	}
	return fmt.Sprintf("Syncthing %s required", e.minSyncthingVersion)
}

// UpdateSchema updates a possibly outdated database to the current schema and
// also does repairs where necessary.
func UpdateSchema(db *Lowlevel) error {
	updater := &schemaUpdater{db}
	return updater.updateSchema()
}

type schemaUpdater struct {
	*Lowlevel
}

func (db *schemaUpdater) updateSchema() error {
	// Updating the schema can touch any and all parts of the database. Make
	// sure we do not run GC concurrently with schema migrations.
	db.gcMut.Lock()
	defer db.gcMut.Unlock()

	miscDB := NewMiscDataNamespace(db.Lowlevel)
	prevVersion, _, err := miscDB.Int64("dbVersion")
	if err != nil {
		return err
	}

	if prevVersion > dbVersion {
		err := &databaseDowngradeError{}
		if minSyncthingVersion, ok, dbErr := miscDB.String("dbMinSyncthingVersion"); dbErr != nil {
			return dbErr
		} else if ok {
			err.minSyncthingVersion = minSyncthingVersion
		}
		return err
	}

	prevMigration, _, err := miscDB.Int64("dbMigrationVersion")
	if err != nil {
		return err
	}
	// Cover versions before adding `dbMigrationVersion` (== 0) and possible future weirdness.
	if prevMigration < prevVersion {
		prevMigration = prevVersion
	}

	if prevVersion == dbVersion && prevMigration >= dbMigrationVersion {
		return nil
	}

	migrations := []migration{
		{1, 1, "v0.14.0", db.updateSchema0to1},
		{2, 2, "v0.14.46", db.updateSchema1to2},
		{3, 3, "v0.14.48", db.updateSchema2to3},
		{5, 5, "v0.14.49", db.updateSchemaTo5},
		{6, 6, "v0.14.50", db.updateSchema5to6},
		{7, 7, "v0.14.53", db.updateSchema6to7},
		{9, 9, "v1.4.0", db.updateSchemaTo9},
		{10, 10, "v1.6.0", db.updateSchemaTo10},
		{11, 11, "v1.6.0", db.updateSchemaTo11},
		{13, 13, "v1.7.0", db.updateSchemaTo13},
		{14, 14, "v1.9.0", db.updateSchemaTo14},
		{14, 16, "v1.9.0", db.checkRepairMigration},
		{14, 17, "v1.9.0", db.migration17},
		{14, 19, "v1.9.0", db.dropAllIndexIDsMigration},
		{14, 20, "v1.9.0", db.dropOutgoingIndexIDsMigration},
	}

	for _, m := range migrations {
		if prevMigration < m.migrationVersion {
			l.Infof("Running database migration %d...", m.migrationVersion)
			if err := m.migration(int(prevVersion)); err != nil {
				return fmt.Errorf("failed to do migration %v: %w", m.migrationVersion, err)
			}
			if err := db.writeVersions(m, miscDB); err != nil {
				return fmt.Errorf("failed to write versions after migration %v: %w", m.migrationVersion, err)
			}
		}
	}

	if err := db.writeVersions(migration{
		schemaVersion:       dbVersion,
		migrationVersion:    dbMigrationVersion,
		minSyncthingVersion: dbMinSyncthingVersion,
	}, miscDB); err != nil {
		return fmt.Errorf("failed to write versions after migrations: %w", err)
	}

	l.Infoln("Compacting database after migration...")
	return db.Compact()
}

func (*schemaUpdater) writeVersions(m migration, miscDB *NamespacedKV) error {
	if err := miscDB.PutInt64("dbVersion", m.schemaVersion); err != nil {
		return err
	}
	if err := miscDB.PutString("dbMinSyncthingVersion", m.minSyncthingVersion); err != nil {
		return err
	}
	if err := miscDB.PutInt64("dbMigrationVersion", m.migrationVersion); err != nil {
		return err
	}
	return nil
}

func (db *schemaUpdater) updateSchema0to1(_ int) error {
	t, err := db.newReadWriteTransaction()
	if err != nil {
		return err
	}
	defer t.close()

	dbi, err := t.NewPrefixIterator([]byte{KeyTypeDevice})
	if err != nil {
		return err
	}
	defer dbi.Release()

	symlinkConv := 0
	changedFolders := make(map[string]struct{})
	ignAdded := 0
	var gk []byte
	ro := t.readOnlyTransaction

	for dbi.Next() {
		folder, ok := db.keyer.FolderFromDeviceFileKey(dbi.Key())
		if !ok {
			// not having the folder in the index is bad; delete and continue
			if err := t.Delete(dbi.Key()); err != nil {
				return err
			}
			continue
		}
		device, ok := db.keyer.DeviceFromDeviceFileKey(dbi.Key())
		if !ok {
			// not having the device in the index is bad; delete and continue
			if err := t.Delete(dbi.Key()); err != nil {
				return err
			}
			continue
		}
		name := db.keyer.NameFromDeviceFileKey(dbi.Key())

		// Remove files with absolute path (see #4799)
		if strings.HasPrefix(string(name), "/") {
			if _, ok := changedFolders[string(folder)]; !ok {
				changedFolders[string(folder)] = struct{}{}
			}
			if err := t.Delete(dbi.Key()); err != nil {
				return err
			}
			gk, err = db.keyer.GenerateGlobalVersionKey(gk, folder, name)
			if err != nil {
				return err
			}
			fl, err := getGlobalVersionsByKeyBefore11(gk, ro)
			if backend.IsNotFound(err) {
				// Shouldn't happen, but not critical.
				continue
			} else if err != nil {
				return err
			}
			_, _ = fl.pop(device)
			if len(fl.Versions) == 0 {
				err = t.Delete(gk)
			} else {
				err = t.Put(gk, mustMarshal(&fl))
			}
			if err != nil {
				return err
			}
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
		if f.Type == protocol.FileInfoTypeSymlinkDirectory || f.Type == protocol.FileInfoTypeSymlinkFile {
			f.Type = protocol.FileInfoTypeSymlink
			bs, err := f.Marshal()
			if err != nil {
				panic("can't happen: " + err.Error())
			}
			if err := t.Put(dbi.Key(), bs); err != nil {
				return err
			}
			symlinkConv++
		}

		// Add invalid files to global list
		if f.IsInvalid() {
			gk, err = db.keyer.GenerateGlobalVersionKey(gk, folder, name)
			if err != nil {
				return err
			}

			fl, err := getGlobalVersionsByKeyBefore11(gk, ro)
			if err != nil && !backend.IsNotFound(err) {
				return err
			}
			i := sort.Search(len(fl.Versions), func(j int) bool {
				return fl.Versions[j].Invalid
			})
			for ; i < len(fl.Versions); i++ {
				ordering := fl.Versions[i].Version.Compare(f.Version)
				shouldInsert := ordering == protocol.Equal
				if !shouldInsert {
					shouldInsert, err = shouldInsertBefore(ordering, folder, fl.Versions[i].Device, true, f, ro)
					if err != nil {
						return err
					}
				}
				if shouldInsert {
					nv := FileVersionDeprecated{
						Device:  device,
						Version: f.Version,
						Invalid: true,
					}
					fl.insertAt(i, nv)
					if err := t.Put(gk, mustMarshal(&fl)); err != nil {
						return err
					}
					if _, ok := changedFolders[string(folder)]; !ok {
						changedFolders[string(folder)] = struct{}{}
					}
					ignAdded++
					break
				}

			}
		}
		if err := t.Checkpoint(); err != nil {
			return err
		}
	}
	dbi.Release()
	if err != dbi.Error() {
		return err
	}

	return t.Commit()
}

// updateSchema1to2 introduces a sequenceKey->deviceKey bucket for local items
// to allow iteration in sequence order (simplifies sending indexes).
func (db *schemaUpdater) updateSchema1to2(_ int) error {
	t, err := db.newReadWriteTransaction()
	if err != nil {
		return err
	}
	defer t.close()

	var sk []byte
	var dk []byte
	for _, folderStr := range db.ListFolders() {
		folder := []byte(folderStr)
		var putErr error
		err := t.withHave(folder, protocol.LocalDeviceID[:], nil, true, func(f protocol.FileIntf) bool {
			sk, putErr = db.keyer.GenerateSequenceKey(sk, folder, f.SequenceNo())
			if putErr != nil {
				return false
			}
			dk, putErr = db.keyer.GenerateDeviceFileKey(dk, folder, protocol.LocalDeviceID[:], []byte(f.FileName()))
			if putErr != nil {
				return false
			}
			putErr = t.Put(sk, dk)
			return putErr == nil
		})
		if putErr != nil {
			return putErr
		}
		if err != nil {
			return err
		}
	}
	return t.Commit()
}

// updateSchema2to3 introduces a needKey->nil bucket for locally needed files.
func (db *schemaUpdater) updateSchema2to3(_ int) error {
	t, err := db.newReadWriteTransaction()
	if err != nil {
		return err
	}
	defer t.close()

	var nk []byte
	var dk []byte
	for _, folderStr := range db.ListFolders() {
		folder := []byte(folderStr)
		var putErr error
		err := withGlobalBefore11(folder, true, func(f protocol.FileIntf) bool {
			name := []byte(f.FileName())
			dk, putErr = db.keyer.GenerateDeviceFileKey(dk, folder, protocol.LocalDeviceID[:], name)
			if putErr != nil {
				return false
			}
			var v protocol.Vector
			haveFile, ok, err := t.getFileTrunc(dk, true)
			if err != nil {
				putErr = err
				return false
			}
			if ok {
				v = haveFile.FileVersion()
			}
			fv := FileVersionDeprecated{
				Version: f.FileVersion(),
				Invalid: f.IsInvalid(),
				Deleted: f.IsDeleted(),
			}
			if !needDeprecated(fv, ok, v) {
				return true
			}
			nk, putErr = t.keyer.GenerateNeedFileKey(nk, folder, []byte(f.FileName()))
			if putErr != nil {
				return false
			}
			putErr = t.Put(nk, nil)
			return putErr == nil
		}, t.readOnlyTransaction)
		if putErr != nil {
			return putErr
		}
		if err != nil {
			return err
		}
	}
	return t.Commit()
}

// updateSchemaTo5 resets the need bucket due to bugs existing in the v0.14.49
// release candidates (dbVersion 3 and 4)
// https://github.com/syncthing/syncthing/issues/5007
// https://github.com/syncthing/syncthing/issues/5053
func (db *schemaUpdater) updateSchemaTo5(prevVersion int) error {
	if prevVersion != 3 && prevVersion != 4 {
		return nil
	}

	t, err := db.newReadWriteTransaction()
	if err != nil {
		return err
	}
	var nk []byte
	for _, folderStr := range db.ListFolders() {
		nk, err = db.keyer.GenerateNeedFileKey(nk, []byte(folderStr), nil)
		if err != nil {
			return err
		}
		if err := t.deleteKeyPrefix(nk[:keyPrefixLen+keyFolderLen]); err != nil {
			return err
		}
	}
	if err := t.Commit(); err != nil {
		return err
	}

	return db.updateSchema2to3(2)
}

func (db *schemaUpdater) updateSchema5to6(_ int) error {
	// For every local file with the Invalid bit set, clear the Invalid bit and
	// set LocalFlags = FlagLocalIgnored.

	t, err := db.newReadWriteTransaction()
	if err != nil {
		return err
	}
	defer t.close()

	var dk []byte

	for _, folderStr := range db.ListFolders() {
		folder := []byte(folderStr)
		var iterErr error
		err := t.withHave(folder, protocol.LocalDeviceID[:], nil, false, func(f protocol.FileIntf) bool {
			if !f.IsInvalid() {
				return true
			}

			fi := f.(protocol.FileInfo)
			fi.RawInvalid = false
			fi.LocalFlags = protocol.FlagLocalIgnored
			bs, _ := fi.Marshal()

			dk, iterErr = db.keyer.GenerateDeviceFileKey(dk, folder, protocol.LocalDeviceID[:], []byte(fi.Name))
			if iterErr != nil {
				return false
			}
			if iterErr = t.Put(dk, bs); iterErr != nil {
				return false
			}
			iterErr = t.Checkpoint()
			return iterErr == nil
		})
		if iterErr != nil {
			return iterErr
		}
		if err != nil {
			return err
		}
	}
	return t.Commit()
}

// updateSchema6to7 checks whether all currently locally needed files are really
// needed and removes them if not.
func (db *schemaUpdater) updateSchema6to7(_ int) error {
	t, err := db.newReadWriteTransaction()
	if err != nil {
		return err
	}
	defer t.close()

	var gk []byte
	var nk []byte

	for _, folderStr := range db.ListFolders() {
		folder := []byte(folderStr)
		var delErr error
		err := withNeedLocalBefore11(folder, false, func(f protocol.FileIntf) bool {
			name := []byte(f.FileName())
			gk, delErr = db.keyer.GenerateGlobalVersionKey(gk, folder, name)
			if delErr != nil {
				return false
			}
			svl, err := t.Get(gk)
			if err != nil {
				// If there is no global list, we hardly need it.
				key, err := t.keyer.GenerateNeedFileKey(nk, folder, name)
				if err != nil {
					delErr = err
					return false
				}
				delErr = t.Delete(key)
				return delErr == nil
			}
			var fl VersionListDeprecated
			err = fl.Unmarshal(svl)
			if err != nil {
				// This can't happen, but it's ignored everywhere else too,
				// so lets not act on it.
				return true
			}
			globalFV := FileVersionDeprecated{
				Version: f.FileVersion(),
				Invalid: f.IsInvalid(),
				Deleted: f.IsDeleted(),
			}

			if localFV, haveLocalFV := fl.Get(protocol.LocalDeviceID[:]); !needDeprecated(globalFV, haveLocalFV, localFV.Version) {
				key, err := t.keyer.GenerateNeedFileKey(nk, folder, name)
				if err != nil {
					delErr = err
					return false
				}
				delErr = t.Delete(key)
			}
			return delErr == nil
		}, t.readOnlyTransaction)
		if delErr != nil {
			return delErr
		}
		if err != nil {
			return err
		}
		if err := t.Checkpoint(); err != nil {
			return err
		}
	}
	return t.Commit()
}

func (db *schemaUpdater) updateSchemaTo9(_ int) error {
	// Loads and rewrites all files with blocks, to deduplicate block lists.

	t, err := db.newReadWriteTransaction()
	if err != nil {
		return err
	}
	defer t.close()

	if err := rewriteFiles(t); err != nil {
		return err
	}

	db.recordTime(indirectGCTimeKey)

	return t.Commit()
}

func rewriteFiles(t readWriteTransaction) error {
	it, err := t.NewPrefixIterator([]byte{KeyTypeDevice})
	if err != nil {
		return err
	}
	defer it.Release()
	for it.Next() {
		intf, err := t.unmarshalTrunc(it.Value(), false)
		if backend.IsNotFound(err) {
			// Unmarshal error due to missing parts (block list), probably
			// due to a bad migration in a previous RC. Drop this key, as
			// getFile would anyway return this as a "not found" in the
			// normal flow of things.
			if err := t.Delete(it.Key()); err != nil {
				return err
			}
			continue
		} else if err != nil {
			return err
		}
		fi := intf.(protocol.FileInfo)
		if fi.Blocks == nil {
			continue
		}
		if err := t.putFile(it.Key(), fi); err != nil {
			return err
		}
		if err := t.Checkpoint(); err != nil {
			return err
		}
	}
	it.Release()
	return it.Error()
}

func (db *schemaUpdater) updateSchemaTo10(_ int) error {
	// Rewrites global lists to include a Deleted flag.

	t, err := db.newReadWriteTransaction()
	if err != nil {
		return err
	}
	defer t.close()

	var buf []byte

	for _, folderStr := range db.ListFolders() {
		folder := []byte(folderStr)

		buf, err = t.keyer.GenerateGlobalVersionKey(buf, folder, nil)
		if err != nil {
			return err
		}
		buf = globalVersionKey(buf).WithoutName()
		dbi, err := t.NewPrefixIterator(buf)
		if err != nil {
			return err
		}
		defer dbi.Release()

		for dbi.Next() {
			var vl VersionListDeprecated
			if err := vl.Unmarshal(dbi.Value()); err != nil {
				return err
			}

			changed := false
			name := t.keyer.NameFromGlobalVersionKey(dbi.Key())

			for i, fv := range vl.Versions {
				buf, err = t.keyer.GenerateDeviceFileKey(buf, folder, fv.Device, name)
				if err != nil {
					return err
				}
				f, ok, err := t.getFileTrunc(buf, true)
				if !ok {
					return errEntryFromGlobalMissing
				}
				if err != nil {
					return err
				}
				if f.IsDeleted() {
					vl.Versions[i].Deleted = true
					changed = true
				}
			}

			if changed {
				if err := t.Put(dbi.Key(), mustMarshal(&vl)); err != nil {
					return err
				}
				if err := t.Checkpoint(); err != nil {
					return err
				}
			}
		}
		dbi.Release()
	}

	// Trigger metadata recalc
	if err := t.deleteKeyPrefix([]byte{KeyTypeFolderMeta}); err != nil {
		return err
	}

	return t.Commit()
}

func (db *schemaUpdater) updateSchemaTo11(_ int) error {
	// Populates block list map for every folder.

	t, err := db.newReadWriteTransaction()
	if err != nil {
		return err
	}
	defer t.close()

	var dk []byte
	for _, folderStr := range db.ListFolders() {
		folder := []byte(folderStr)
		var putErr error
		err := t.withHave(folder, protocol.LocalDeviceID[:], nil, true, func(fi protocol.FileIntf) bool {
			f := fi.(FileInfoTruncated)
			if f.IsDirectory() || f.IsDeleted() || f.IsSymlink() || f.IsInvalid() || f.BlocksHash == nil {
				return true
			}

			name := []byte(f.FileName())
			dk, putErr = db.keyer.GenerateBlockListMapKey(dk, folder, f.BlocksHash, name)
			if putErr != nil {
				return false
			}

			if putErr = t.Put(dk, nil); putErr != nil {
				return false
			}
			putErr = t.Checkpoint()
			return putErr == nil
		})
		if putErr != nil {
			return putErr
		}
		if err != nil {
			return err
		}
	}
	return t.Commit()
}

func (db *schemaUpdater) updateSchemaTo13(prev int) error {
	// Loads and rewrites all files, to deduplicate version vectors.

	t, err := db.newReadWriteTransaction()
	if err != nil {
		return err
	}
	defer t.close()

	if prev < 12 {
		if err := rewriteFiles(t); err != nil {
			return err
		}
	}

	if err := rewriteGlobals(t); err != nil {
		return err
	}

	return t.Commit()
}

func (db *schemaUpdater) updateSchemaTo14(_ int) error {
	// Checks for missing blocks and marks those entries as requiring a
	// rehash/being invalid. The db is checked/repaired afterwards, i.e.
	// no care is taken to get metadata and sequences right.
	// If the corresponding files changed on disk compared to the global
	// version, this will cause a conflict.

	var key, gk []byte
	for _, folderStr := range db.ListFolders() {
		folder := []byte(folderStr)
		meta := newMetadataTracker(db.keyer, db.evLogger)
		meta.counts.Created = 0 // Recalculate metadata afterwards

		t, err := db.newReadWriteTransaction(meta.CommitHook(folder))
		if err != nil {
			return err
		}
		defer t.close()

		key, err = t.keyer.GenerateDeviceFileKey(key, folder, protocol.LocalDeviceID[:], nil)
		if err != nil {
			return err
		}
		it, err := t.NewPrefixIterator(key)
		if err != nil {
			return err
		}
		defer it.Release()
		for it.Next() {
			var fi protocol.FileInfo
			if err := fi.Unmarshal(it.Value()); err != nil {
				return err
			}
			if len(fi.Blocks) > 0 || len(fi.BlocksHash) == 0 {
				continue
			}
			key = t.keyer.GenerateBlockListKey(key, fi.BlocksHash)
			_, err := t.Get(key)
			if err == nil {
				continue
			}

			fi.SetMustRescan()
			if err = t.putFile(it.Key(), fi); err != nil {
				return err
			}

			gk, err = t.keyer.GenerateGlobalVersionKey(gk, folder, []byte(fi.Name))
			if err != nil {
				return err
			}
			key, err = t.updateGlobal(gk, key, folder, protocol.LocalDeviceID[:], fi, meta)
			if err != nil {
				return err
			}
		}
		it.Release()

		if err = t.Commit(); err != nil {
			return err
		}
		t.close()
	}

	return nil
}

func (db *schemaUpdater) checkRepairMigration(_ int) error {
	for _, folder := range db.ListFolders() {
		_, err := db.getMetaAndCheckGCLocked(folder)
		if err != nil {
			return err
		}
	}
	return nil
}

// migration17 finds all files that were pulled as invalid from an invalid
// global and make sure they get scanned/pulled again.
func (db *schemaUpdater) migration17(prev int) error {
	if prev < 16 {
		// Issue was introduced in migration to 16
		return nil
	}
	t, err := db.newReadOnlyTransaction()
	if err != nil {
		return err
	}
	defer t.close()

	for _, folderStr := range db.ListFolders() {
		folder := []byte(folderStr)
		meta, err := db.loadMetadataTracker(folderStr)
		if err != nil {
			return err
		}
		batch := NewFileInfoBatch(func(fs []protocol.FileInfo) error {
			return db.updateLocalFiles(folder, fs, meta)
		})
		var innerErr error
		err = t.withHave(folder, protocol.LocalDeviceID[:], nil, false, func(fi protocol.FileIntf) bool {
			if fi.IsInvalid() && fi.FileLocalFlags() == 0 {
				f := fi.(protocol.FileInfo)
				f.SetMustRescan()
				f.Version = protocol.Vector{}
				batch.Append(f)
				innerErr = batch.FlushIfFull()
				return innerErr == nil
			}
			return true
		})
		if innerErr != nil {
			return innerErr
		}
		if err != nil {
			return err
		}
		if err := batch.Flush(); err != nil {
			return err
		}
	}
	return nil
}

func (db *schemaUpdater) dropAllIndexIDsMigration(_ int) error {
	return db.dropIndexIDs()
}

func (db *schemaUpdater) dropOutgoingIndexIDsMigration(_ int) error {
	return db.dropOtherDeviceIndexIDs()
}

func rewriteGlobals(t readWriteTransaction) error {
	it, err := t.NewPrefixIterator([]byte{KeyTypeGlobal})
	if err != nil {
		return err
	}
	defer it.Release()

	for it.Next() {
		var vl VersionListDeprecated
		if err := vl.Unmarshal(it.Value()); err != nil {
			// If we crashed during an earlier migration, some version
			// lists might already be in the new format: Skip those.
			var nvl VersionList
			if nerr := nvl.Unmarshal(it.Value()); nerr == nil {
				continue
			}
			return err
		}
		if len(vl.Versions) == 0 {
			if err := t.Delete(it.Key()); err != nil {
				return err
			}
		}

		newVl := convertVersionList(vl)
		if err := t.Put(it.Key(), mustMarshal(&newVl)); err != nil {
			return err
		}
		if err := t.Checkpoint(); err != nil {
			return err
		}
	}
	return it.Error()
}

func convertVersionList(vl VersionListDeprecated) VersionList {
	var newVl VersionList
	var newPos, oldPos int
	var lastVersion protocol.Vector

	for _, fv := range vl.Versions {
		if fv.Invalid {
			break
		}
		oldPos++
		if len(newVl.RawVersions) > 0 && lastVersion.Equal(fv.Version) {
			newVl.RawVersions[newPos].Devices = append(newVl.RawVersions[newPos].Devices, fv.Device)
			continue
		}
		newPos = len(newVl.RawVersions)
		newVl.RawVersions = append(newVl.RawVersions, newFileVersion(fv.Device, fv.Version, false, fv.Deleted))
		lastVersion = fv.Version
	}

	if oldPos == len(vl.Versions) {
		return newVl
	}

	if len(newVl.RawVersions) == 0 {
		fv := vl.Versions[oldPos]
		newVl.RawVersions = []FileVersion{newFileVersion(fv.Device, fv.Version, true, fv.Deleted)}
		oldPos++
	}
	newPos = 0
outer:
	for _, fv := range vl.Versions[oldPos:] {
		for _, nfv := range newVl.RawVersions[newPos:] {
			switch nfv.Version.Compare(fv.Version) {
			case protocol.Equal:
				newVl.RawVersions[newPos].InvalidDevices = append(newVl.RawVersions[newPos].InvalidDevices, fv.Device)
				continue outer
			case protocol.Lesser:
				newVl.insertAt(newPos, newFileVersion(fv.Device, fv.Version, true, fv.Deleted))
				continue outer
			case protocol.ConcurrentLesser, protocol.ConcurrentGreater:
				// The version is invalid, i.e. it looses anyway,
				// no need to check/get the conflicting file.
			}
			newPos++
		}
		// Couldn't insert into any existing versions
		newVl.RawVersions = append(newVl.RawVersions, newFileVersion(fv.Device, fv.Version, true, fv.Deleted))
		newPos++
	}

	return newVl
}

func getGlobalVersionsByKeyBefore11(key []byte, t readOnlyTransaction) (VersionListDeprecated, error) {
	bs, err := t.Get(key)
	if err != nil {
		return VersionListDeprecated{}, err
	}

	var vl VersionListDeprecated
	if err := vl.Unmarshal(bs); err != nil {
		return VersionListDeprecated{}, err
	}

	return vl, nil
}

func withGlobalBefore11(folder []byte, truncate bool, fn Iterator, t readOnlyTransaction) error {
	key, err := t.keyer.GenerateGlobalVersionKey(nil, folder, nil)
	if err != nil {
		return err
	}
	dbi, err := t.NewPrefixIterator(key)
	if err != nil {
		return err
	}
	defer dbi.Release()

	var dk []byte
	for dbi.Next() {
		name := t.keyer.NameFromGlobalVersionKey(dbi.Key())

		var vl VersionListDeprecated
		if err := vl.Unmarshal(dbi.Value()); err != nil {
			return err
		}

		dk, err = t.keyer.GenerateDeviceFileKey(dk, folder, vl.Versions[0].Device, name)
		if err != nil {
			return err
		}

		f, ok, err := t.getFileTrunc(dk, truncate)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}

		if !fn(f) {
			return nil
		}
	}
	if err != nil {
		return err
	}
	return dbi.Error()
}

func withNeedLocalBefore11(folder []byte, truncate bool, fn Iterator, t readOnlyTransaction) error {
	key, err := t.keyer.GenerateNeedFileKey(nil, folder, nil)
	if err != nil {
		return err
	}
	dbi, err := t.NewPrefixIterator(key.WithoutName())
	if err != nil {
		return err
	}
	defer dbi.Release()

	var keyBuf []byte
	var f protocol.FileIntf
	var ok bool
	for dbi.Next() {
		keyBuf, f, ok, err = getGlobalBefore11(keyBuf, folder, t.keyer.NameFromGlobalVersionKey(dbi.Key()), truncate, t)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		if !fn(f) {
			return nil
		}
	}
	return dbi.Error()
}

func getGlobalBefore11(keyBuf, folder, file []byte, truncate bool, t readOnlyTransaction) ([]byte, protocol.FileIntf, bool, error) {
	keyBuf, err := t.keyer.GenerateGlobalVersionKey(keyBuf, folder, file)
	if err != nil {
		return nil, nil, false, err
	}
	bs, err := t.Get(keyBuf)
	if backend.IsNotFound(err) {
		return keyBuf, nil, false, nil
	} else if err != nil {
		return nil, nil, false, err
	}
	var vl VersionListDeprecated
	if err := vl.Unmarshal(bs); err != nil {
		return nil, nil, false, err
	}
	if len(vl.Versions) == 0 {
		return nil, nil, false, nil
	}
	keyBuf, err = t.keyer.GenerateDeviceFileKey(keyBuf, folder, vl.Versions[0].Device, file)
	if err != nil {
		return nil, nil, false, err
	}
	fi, ok, err := t.getFileTrunc(keyBuf, truncate)
	if err != nil || !ok {
		return keyBuf, nil, false, err
	}
	return keyBuf, fi, true, nil
}

func (vl *VersionListDeprecated) String() string {
	var b bytes.Buffer
	var id protocol.DeviceID
	b.WriteString("{")
	for i, v := range vl.Versions {
		if i > 0 {
			b.WriteString(", ")
		}
		copy(id[:], v.Device)
		fmt.Fprintf(&b, "{%v, %v}", v.Version, id)
	}
	b.WriteString("}")
	return b.String()
}

func (vl *VersionListDeprecated) pop(device []byte) (FileVersionDeprecated, int) {
	for i, v := range vl.Versions {
		if bytes.Equal(v.Device, device) {
			vl.Versions = append(vl.Versions[:i], vl.Versions[i+1:]...)
			return v, i
		}
	}
	return FileVersionDeprecated{}, -1
}

func (vl *VersionListDeprecated) Get(device []byte) (FileVersionDeprecated, bool) {
	for _, v := range vl.Versions {
		if bytes.Equal(v.Device, device) {
			return v, true
		}
	}

	return FileVersionDeprecated{}, false
}

func (vl *VersionListDeprecated) insertAt(i int, v FileVersionDeprecated) {
	vl.Versions = append(vl.Versions, FileVersionDeprecated{})
	copy(vl.Versions[i+1:], vl.Versions[i:])
	vl.Versions[i] = v
}

func needDeprecated(global FileVersionDeprecated, haveLocal bool, localVersion protocol.Vector) bool {
	// We never need an invalid file.
	if global.Invalid {
		return false
	}
	// We don't need a deleted file if we don't have it.
	if global.Deleted && !haveLocal {
		return false
	}
	// We don't need the global file if we already have the same version.
	if haveLocal && localVersion.GreaterEqual(global.Version) {
		return false
	}
	return true
}
