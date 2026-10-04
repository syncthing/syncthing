// Copyright (C) 2026 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package sqlite

import (
	"fmt"
	"slices"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/syncthing/syncthing/lib/protocol"
)

func TestSchemaIndexes(t *testing.T) {
	for _, version := range []int{0, 6, 7} {
		name := "fresh"
		if version > 0 {
			name = fmt.Sprintf("upgrade_v%d", version)
		}
		if version == currentSchemaVersion {
			name = fmt.Sprintf("reopen_v%d", version)
		}

		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			sdb, err := Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			// Capture the variable so cleanup also closes a reopened database.
			t.Cleanup(func() {
				if err := sdb.Close(); err != nil {
					t.Error(err)
				}
			})
			remote := protocol.DeviceID{42}
			file := genFile("test/a", 1, 1)
			for _, dev := range []protocol.DeviceID{protocol.LocalDeviceID, remote} {
				if err := sdb.Update(folderID, dev, []protocol.FileInfo{file}); err != nil {
					t.Fatal(err)
				}
			}
			fdb, err := sdb.getFolderDB(folderID, false)
			if err != nil {
				t.Fatal(err)
			}
			if version > 0 {

				if version == 6 {
					// Reconstruct the v6 index layout, keeping populated tables and all
					// other schema objects, then run the real startup migration path.
					schemaExec(t, sdb.sql,
						`DROP INDEX folders_database_name_unique`,
						`CREATE INDEX folders_database_name ON folders(database_name) WHERE database_name IS NOT NULL`,
						`DELETE FROM schemamigrations`,
						`INSERT INTO schemamigrations VALUES (6, 0, '')`,
					)
					schemaExec(t, fdb.sql,
						`DROP INDEX files_name_device`,
						`DROP INDEX files_device_remote_sequence`,
						`CREATE UNIQUE INDEX files_remote_sequence ON files(device_idx,remote_sequence) WHERE remote_sequence IS NOT NULL`,
						// Intermediate index experiments may exist without a version bump.
						`CREATE INDEX files_device_sequence ON files(device_idx,sequence)`,
						`CREATE INDEX files_global_name ON files(name_idx) WHERE local_flags & 16 != 0`,
						`CREATE INDEX files_needed_size ON files(size) WHERE local_flags & 32 != 0`,
						`CREATE UNIQUE INDEX files_device_name ON files(device_idx, name_idx)`,
						`CREATE INDEX files_name_idx_only ON files(name_idx)`,
						`DELETE FROM schemamigrations`,
						`INSERT INTO schemamigrations VALUES (6, 0, '')`,
					)
				}
				if err := sdb.Close(); err != nil {
					t.Fatal(err)
				}
				reopened, err := Open(dir)
				if err != nil {
					t.Fatal(err)
				}
				sdb = reopened
				fdb, err = sdb.getFolderDB(folderID, false)
				if err != nil {
					t.Fatal(err)
				}
			}
			checkSchemaIndex(t, sdb.sql, "folders", "folders_database_name_unique", true, false, []string{"database_name"})
			checkSchemaIndex(t, fdb.sql, "files", "files_name_device", true, false, []string{"name_idx", "device_idx"})
			checkSchemaIndex(t, fdb.sql, "files", "files_device_remote_sequence", true, false, []string{"device_idx", "remote_sequence"})
			for _, d := range []*sqlx.DB{sdb.sql, fdb.sql} {
				var n int
				if err := d.Get(&n, `SELECT count(*) FROM sqlite_schema WHERE type='index' AND name IN ('folders_database_name', 'files_device_name', 'files_name_idx_only', 'files_remote_sequence', 'files_device_sequence', 'files_global_name', 'files_needed_size')`); err != nil {
					t.Fatal(err)
				}
				if n != 0 {
					t.Errorf("found %d obsolete indexes", n)
				}
				var version int
				if err := d.Get(&version, `SELECT max(schema_version) FROM schemamigrations`); err != nil {
					t.Fatal(err)
				}
				if version != currentSchemaVersion {
					t.Errorf("schema version %d, want %d", version, currentSchemaVersion)
				}
			}
			// NULL names remain allowed; non-NULL database names must be unique.
			schemaExec(t, sdb.sql, `INSERT INTO folders(folder_id) VALUES ('null1'), ('null2')`)
			if _, err := sdb.sql.Exec(`INSERT INTO folders(folder_id, database_name) SELECT 'duplicate', database_name FROM folders WHERE folder_id=?`, folderID); err == nil {
				t.Error("accepted duplicate database name")
			}
			for _, dev := range []protocol.DeviceID{protocol.LocalDeviceID, remote} {
				got, ok, err := sdb.GetDeviceFile(folderID, dev, file.Name)
				if err != nil {
					t.Fatal(err)
				}
				if !ok || got.Name != file.Name || !got.Version.Equal(file.Version) || len(got.Blocks) != len(file.Blocks) {
					t.Errorf("file was not preserved for %v: %+v", dev, got)
				}
			}
			// Check that reversing the unique index preserves replacement semantics.
			if err := sdb.Update(folderID, protocol.LocalDeviceID, []protocol.FileInfo{file}); err != nil {
				t.Fatal(err)
			}
			var n int
			if err := fdb.sql.Get(&n, `SELECT count(*) FROM files`); err != nil {
				t.Fatal(err)
			}
			if n != 2 {
				t.Errorf("got %d files after replacement, want 2", n)
			}
			if _, err := fdb.sql.Exec(`INSERT INTO files(device_idx,name_idx,type,modified,size,version_idx,deleted,local_flags)
    SELECT device_idx,name_idx,type,modified,size,version_idx,deleted,local_flags FROM files LIMIT 1`); err == nil {
				t.Error("accepted duplicate device/name")
			}

			// Changing the NULL predicate must retain remote sequence uniqueness.
			schemaExec(t, fdb.sql, `INSERT INTO file_names(name) VALUES ('duplicate-sequence')`)
			if _, err := fdb.sql.Exec(`INSERT INTO files(device_idx,remote_sequence,name_idx,type,modified,size,version_idx,deleted,local_flags)
                SELECT device_idx,remote_sequence,(SELECT idx FROM file_names WHERE name='duplicate-sequence'),type,modified,size,version_idx,deleted,local_flags
                FROM files WHERE remote_sequence IS NOT NULL LIMIT 1`); err == nil {
				t.Error("accepted duplicate remote sequence")
			}
			// Global and need flags follow changes in which device has the global
			// version, and whether that version is still needed locally.
			checkFlags := func(wantNeed int) {
				t.Helper()
				var global, needed int
				if err := fdb.stmt(`SELECT count(*) FROM files WHERE local_flags & {{.FlagLocalGlobal}} != 0`).Get(&global); err != nil {
					t.Fatal(err)
				}
				if err := fdb.stmt(`SELECT count(*) FROM files WHERE local_flags & {{.FlagLocalNeeded}} != 0`).Get(&needed); err != nil {
					t.Fatal(err)
				}
				if global != 1 || needed != wantNeed {
					t.Errorf("globals=%d, needed=%d; want globals=1, needed=%d", global, needed, wantNeed)
				}
			}
			checkFlags(0)
			file.Version = file.Version.Update(42)
			file.Sequence++
			if err := sdb.Update(folderID, remote, []protocol.FileInfo{file}); err != nil {
				t.Fatal(err)
			}
			checkFlags(1)
			if err := sdb.Update(folderID, protocol.LocalDeviceID, []protocol.FileInfo{file}); err != nil {
				t.Fatal(err)
			}
			checkFlags(0)
		})
	}
}

func schemaExec(t *testing.T, d *sqlx.DB, queries ...string) {
	t.Helper()
	for _, q := range queries {
		if _, err := d.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
}

func checkSchemaIndex(t *testing.T, d *sqlx.DB, table, index string, unique, partial bool, columns []string) {
	t.Helper()
	var props struct {
		Unique  bool
		Partial bool
	}
	if err := d.Get(&props, `SELECT "unique", partial FROM pragma_index_list(?) WHERE name=?`, table, index); err != nil {
		t.Fatal(err)
	}
	if props.Unique != unique || props.Partial != partial {
		t.Errorf("%s: got properties %+v, want unique=%v partial=%v", index, props, unique, partial)
	}
	var got []string
	if err := d.Select(&got, `SELECT name FROM pragma_index_info(?) ORDER BY seqno`, index); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, columns) {
		t.Errorf("%s columns %v, want %v", index, got, columns)
	}
}
