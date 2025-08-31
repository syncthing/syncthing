// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build forcecgo

package sqlite

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "github.com/mattn/go-sqlite3"
)

// TestDBInvalidColumnMigration tests database migration scenarios with invalid column present/absent
// This ensures backward compatibility with older database schemas and prevents "NOT NULL constraint failed: files.invalid" errors
func TestDBInvalidColumnMigration(t *testing.T) {
	// Test with database without invalid column (new schema)
	// This verifies that the new schema works correctly without the invalid column
	t.Run("NewSchemaWithoutInvalidColumn", func(t *testing.T) {
		testDBInvalidColumnNewSchema(t)
	})

	// Test with database with invalid column (old schema)
	// This verifies backward compatibility with older database schemas that still have the invalid column
	t.Run("OldSchemaWithInvalidColumn", func(t *testing.T) {
		testDBInvalidColumnOldSchema(t)
	})
}

// testDBInvalidColumnNewSchema tests file indexing operations with new schema (without invalid column)
// This ensures that the new schema works correctly and prevents database constraint errors
func testDBInvalidColumnNewSchema(t *testing.T) {
	tmpDir := t.TempDir()
	
	// Create database with new schema (without invalid column)
	// This simulates a fresh installation or properly migrated database
	db, err := Open(tmpDir)
	require.NoError(t, err)
	defer db.Close()

	// Get the folderDB
	folderDB, err := db.getFolderDB("test-folder", true)
	require.NoError(t, err)
	assert.NotNil(t, folderDB)

	// Test file indexing operations
	// This verifies that file operations work correctly with the new schema
	testFileIndexingOperations(t, folderDB)
}

// testDBInvalidColumnOldSchema tests file indexing operations with old schema (with invalid column)
// This ensures backward compatibility with older database schemas that still have the invalid column
func testDBInvalidColumnOldSchema(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "old-schema.db")

	// First create a database with the old schema that includes the invalid column
	// This simulates an older database that hasn't been fully migrated yet
	rawDB, err := openRawSQLiteDB(dbPath)
	require.NoError(t, err)

	// Create tables with the old schema that includes invalid column
	// This replicates the exact schema that would cause "NOT NULL constraint failed: files.invalid" errors
	_, err = rawDB.Exec(`
		CREATE TABLE devices (
			idx INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
			device_id TEXT NOT NULL UNIQUE
		);
		CREATE TABLE files (
			device_idx INTEGER NOT NULL REFERENCES devices(idx),
			remote_sequence INTEGER,
			name TEXT NOT NULL,
			type INTEGER NOT NULL,
			modified INTEGER NOT NULL,
			size INTEGER NOT NULL,
			version TEXT NOT NULL,
			deleted INTEGER NOT NULL,
			invalid INTEGER NOT NULL,  -- This is the old invalid column that causes constraint errors
			local_flags INTEGER NOT NULL,
			blocklist_hash BLOB,
			sequence INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT
		);
		CREATE TABLE folders (
			idx INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
			folder_id TEXT NOT NULL UNIQUE
		);
	`)
	require.NoError(t, err)
	rawDB.Close()

	// Now open it with our DB implementation
	// This tests that our code can handle databases with the old schema
	db, err := Open(tmpDir)
	require.NoError(t, err)
	defer db.Close()

	// Get the folderDB
	folderDB, err := db.getFolderDB("test-folder", true)
	require.NoError(t, err)
	assert.NotNil(t, folderDB)

	// Test file indexing operations
	// This verifies that file operations work correctly even with the old schema
	testFileIndexingOperations(t, folderDB)
}

// openRawSQLiteDB opens a raw SQLite database for testing
// This is used to create databases with specific schemas for testing purposes
func openRawSQLiteDB(path string) (*sqlx.DB, error) {
	pathURL := fmt.Sprintf("file:%s?_fk=true&_rt=true&_cache_size=-65536&_sync=1&_txlock=immediate", path)
	sqlDB, err := sqlx.Open("sqlite3", pathURL)
	if err != nil {
		return nil, err
	}
	return sqlDB, nil
}

// testFileIndexingOperations tests that file indexing works correctly
// This verifies that file operations work correctly regardless of schema version
func testFileIndexingOperations(t *testing.T, folderDB *folderDB) {
	// Test with valid file
	// This simulates a normal file that should be indexed without issues
	validFile := protocol.FileInfo{
		Name:       "test-file.txt",
		Type:       protocol.FileInfoTypeFile,
		Size:       1024,
		ModifiedS:  1000,
		ModifiedNs: 0,
		Version:    protocol.Vector{}.Update(1),
	}

	// Test with invalid file
	// This simulates an invalid file that should be handled correctly
	invalidFile := protocol.FileInfo{
		Name:       "invalid-file.txt",
		Type:       protocol.FileInfoTypeFile,
		Size:       2048,
		ModifiedS:  2000,
		ModifiedNs: 0,
		Version:    protocol.Vector{}.Update(2),
		LocalFlags: protocol.FlagLocalRemoteInvalid,
	}

	// Test updating files
	// This verifies that both valid and invalid files can be updated without constraint errors
	files := []protocol.FileInfo{validFile, invalidFile}
	err := folderDB.Update(protocol.LocalDeviceID, files)
	assert.NoError(t, err)

	// Test with remote device
	// This simulates file synchronization with a remote device
	remoteDeviceID, err := protocol.DeviceIDFromString("I6KAH76-66SLLLB-5PFXSOA-UFJCDZC-YAOMLEK-CP2GB32-BV5RQST-3PSROAU")
	require.NoError(t, err)

	remoteFile := protocol.FileInfo{
		Name:       "remote-file.txt",
		Type:       protocol.FileInfoTypeFile,
		Size:       4096,
		ModifiedS:  3000,
		ModifiedNs: 0,
		Version:    protocol.Vector{}.Update(3),
		Sequence:   1,
	}

	err = folderDB.Update(remoteDeviceID, []protocol.FileInfo{remoteFile})
	assert.NoError(t, err)

	// Verify files can be retrieved
	// This ensures that files are properly stored and can be retrieved
	fi, ok, err := folderDB.GetDeviceFile(protocol.LocalDeviceID, "test-file.txt")
	assert.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "test-file.txt", fi.Name)
	assert.Equal(t, int64(1024), fi.Size)

	// Check invalid file
	// This verifies that invalid files are properly handled
	fi, ok, err = folderDB.GetDeviceFile(protocol.LocalDeviceID, "invalid-file.txt")
	assert.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "invalid-file.txt", fi.Name)
	assert.Equal(t, protocol.FlagLocalRemoteInvalid, fi.LocalFlags)

	// Check remote file
	// This verifies that remote files are properly handled
	fi, ok, err = folderDB.GetDeviceFile(remoteDeviceID, "remote-file.txt")
	assert.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "remote-file.txt", fi.Name)
	assert.Equal(t, int64(4096), fi.Size)
}

// TestConnectionStability tests connection stability after file exchange processes
// This ensures that database operations don't cause connection drops or instability
func TestConnectionStability(t *testing.T) {
	tmpDir := t.TempDir()

	// Create database
	// This simulates a real-world scenario with ongoing file synchronization
	db, err := Open(tmpDir)
	require.NoError(t, err)
	defer db.Close()

	// Get the folderDB
	folderDB, err := db.getFolderDB("test-folder", true)
	require.NoError(t, err)
	assert.NotNil(t, folderDB)

	// Simulate multiple file exchange processes
	// This tests the stability of database operations under load
	for i := 0; i < 10; i++ {
		// Create some files
		files := make([]protocol.FileInfo, 10)
		for j := 0; j < 10; j++ {
			files[j] = protocol.FileInfo{
				Name:       filepath.Join("dir", fmt.Sprintf("file%d.txt", i*10+j)),
				Type:       protocol.FileInfoTypeFile,
				Size:       int64(100 + i*10 + j),
				ModifiedS:  int64(1000 + i*100 + j),
				ModifiedNs: 0,
				Version:    protocol.Vector{}.Update(protocol.ShortID(i*10 + j + 1)),
			}
		}

		// Update files
		// This simulates ongoing file synchronization
		err := folderDB.Update(protocol.LocalDeviceID, files)
		assert.NoError(t, err)

		// Verify files were stored correctly
		// This ensures data integrity during the synchronization process
		for j := 0; j < 10; j++ {
			filename := filepath.Join("dir", fmt.Sprintf("file%d.txt", i*10+j))
			fi, ok, err := folderDB.GetDeviceFile(protocol.LocalDeviceID, filename)
			assert.NoError(t, err)
			assert.True(t, ok)
			assert.Equal(t, filename, fi.Name)
			assert.Equal(t, int64(100+i*10+j), fi.Size)
		}
	}
}