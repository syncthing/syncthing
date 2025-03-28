// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package db

import (
	"fmt"

	"google.golang.org/protobuf/proto"

	"github.com/syncthing/syncthing/internal/gen/bep"
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

	if prevVersion > 0 && prevVersion < 14 {
		// This is a database version that is too old to be upgraded directly.
		// The user will have to upgrade to an older version first.
		return fmt.Errorf("database version %d is too old to be upgraded directly; step via Syncthing v1.27.0 to upgrade", prevVersion)
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
			var bepf bep.FileInfo
			if err := proto.Unmarshal(it.Value(), &bepf); err != nil {
				return err
			}
			fi := protocol.FileInfoFromDB(&bepf)
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
		err = t.withHave(folder, protocol.LocalDeviceID[:], nil, false, func(fi protocol.FileInfo) bool {
			if fi.IsInvalid() && fi.FileLocalFlags() == 0 {
				fi.SetMustRescan()
				fi.Version = protocol.Vector{}
				batch.Append(fi)
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
