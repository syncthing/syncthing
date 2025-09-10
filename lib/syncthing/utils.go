// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package syncthing

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/syncthing/syncthing/internal/db"
	"github.com/syncthing/syncthing/internal/db/olddb"
	"github.com/syncthing/syncthing/internal/db/olddb/backend"
	"github.com/syncthing/syncthing/internal/db/sqlite"
	"github.com/syncthing/syncthing/internal/slogutil"
	"github.com/syncthing/syncthing/lib/build"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/locations"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/tlsutil"
)

func EnsureDir(dir string, mode fs.FileMode) error {
	fs := fs.NewFilesystem(fs.FilesystemTypeBasic, dir)
	err := fs.MkdirAll(".", mode)
	if err != nil {
		return err
	}

	if fi, err := fs.Stat("."); err == nil {
		// Apparently the stat may fail even though the mkdirall passed. If it
		// does, we'll just assume things are in order and let other things
		// fail (like loading or creating the config...).
		currentMode := fi.Mode() & 0o777
		if currentMode != mode {
			err := fs.Chmod(".", mode)
			// This can fail on crappy filesystems, nothing we can do about it.
			if err != nil {
				slog.Warn("Failed to correct directory permissions", slogutil.Error(err))
			}
		}
	}
	return nil
}

func LoadOrGenerateCertificate(certFile, keyFile string) (tls.Certificate, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return GenerateCertificate(certFile, keyFile)
	}
	return cert, nil
}

func GenerateCertificate(certFile, keyFile string) (tls.Certificate, error) {
	slog.Info("Generating key and certificate", "cn", tlsDefaultCommonName)
	return tlsutil.NewCertificate(certFile, keyFile, tlsDefaultCommonName, deviceCertLifetimeDays, false)
}

func DefaultConfig(path string, myID protocol.DeviceID, evLogger events.Logger, skipPortProbing bool) (config.Wrapper, error) {
	newCfg := config.New(myID)

	if skipPortProbing {
		slog.Info("Using default network port numbers instead of probing for free ports")
		// Record address override initially
		newCfg.GUI.RawAddress = newCfg.GUI.Address()
	} else if err := newCfg.ProbeFreePorts(); err != nil {
		return nil, err
	}

	return config.Wrap(path, newCfg, myID, evLogger), nil
}

// LoadConfigAtStartup loads an existing config. If it doesn't yet exist, it
// creates a default one. Otherwise it checks the version, and archives and
// upgrades the config if necessary or returns an error, if the version
// isn't compatible.
func LoadConfigAtStartup(path string, cert tls.Certificate, evLogger events.Logger, allowNewerConfig, skipPortProbing bool) (config.Wrapper, error) {
	myID := protocol.NewDeviceID(cert.Certificate[0])
	cfg, originalVersion, err := config.Load(path, myID, evLogger)
	if fs.IsNotExist(err) {
		cfg, err = DefaultConfig(path, myID, evLogger, skipPortProbing)
		if err != nil {
			return nil, fmt.Errorf("failed to generate default config: %w", err)
		}
		err = cfg.Save()
		if err != nil {
			return nil, fmt.Errorf("failed to save default config: %w", err)
		}
		slog.Info("Default config saved; edit to taste (with Syncthing stopped) or use the GUI", slogutil.FilePath(cfg.ConfigPath()))
	} else if errors.Is(err, io.EOF) {
		return nil, errors.New("failed to load config: unexpected end of file. Truncated or empty configuration?")
	} else if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	if originalVersion != config.CurrentVersion {
		if originalVersion > config.CurrentVersion && !allowNewerConfig {
			return nil, fmt.Errorf("config file version (%d) is newer than supported version (%d); if this is expected, use --allow-newer-config to override", originalVersion, config.CurrentVersion)
		}
		err = archiveAndSaveConfig(cfg, originalVersion)
		if err != nil {
			return nil, fmt.Errorf("config archive: %w", err)
		}
	}

	return cfg, nil
}

func archiveAndSaveConfig(cfg config.Wrapper, originalVersion int) error {
	// Copy the existing config to an archive copy
	archivePath := cfg.ConfigPath() + fmt.Sprintf(".v%d", originalVersion)
	slog.Info("Archiving a copy of old config file format", slogutil.FilePath(archivePath))
	if err := copyFile(cfg.ConfigPath(), archivePath); err != nil {
		return err
	}

	// Do a regular atomic config sve
	return cfg.Save()
}

func copyFile(src, dst string) error {
	bs, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	if err := os.WriteFile(dst, bs, 0o600); err != nil {
		// Attempt to clean up
		os.Remove(dst)
		return err
	}

	return nil
}

// Opens a database
func OpenDatabase(path string, deleteRetention time.Duration) (db.DB, error) {
	sql, err := sqlite.Open(path, sqlite.WithDeleteRetention(deleteRetention))
	if err != nil {
		return nil, err
	}

	sdb := db.MetricsWrap(sql)

	return sdb, nil
}

// Attempts migration of the old (LevelDB-based) database type to the new (SQLite-based) type
// This will attempt to provide a temporary API server during the migration, if `apiAddr` is not empty.
func TryMigrateDatabase(ctx context.Context, deleteRetention time.Duration) error {
	oldDBDir := locations.Get(locations.LegacyDatabase)
	if _, err := os.Lstat(oldDBDir); err != nil {
		// No old database
		return nil
	}

	be, err := backend.OpenLevelDBRO(oldDBDir)
	if err != nil {
		// Apparently, not a valid old database
		return nil
	}
	defer be.Close()

	sdb, err := sqlite.OpenForMigration(locations.Get(locations.Database))
	if err != nil {
		return err
	}
	defer sdb.Close()

	miscDB := db.NewMiscDB(sdb)
	if when, ok, err := miscDB.Time("migrated-from-leveldb-at"); err == nil && ok {
		slog.Error("Old-style database present but already migrated; please manually move or remove.", slog.Any("migratedAt", when), slogutil.FilePath(oldDBDir))
		return nil
	}

	slog.Info("Migrating old-style database to SQLite; this may take a while...")
	t0 := time.Now()

	ll, err := olddb.NewLowlevel(be)
	if err != nil {
		return err
	}

	totFiles, totBlocks := 0, 0
	for _, folder := range ll.ListFolders() {
		// Start a writer routine
		fis := make(chan protocol.FileInfo, 50)
		var writeErr error
		var wg sync.WaitGroup
		wg.Add(1)
		writerDone := make(chan struct{})
		go func() {
			defer wg.Done()
			defer close(writerDone)
			var batch []protocol.FileInfo
			files, blocks := 0, 0
			t0 := time.Now()
			t1 := time.Now()

			if writeErr = sdb.DropFolder(folder); writeErr != nil {
				slog.Error("Failed database drop", slogutil.Error(writeErr))
				return
			}

			for fi := range fis {
				batch = append(batch, fi)
				files++
				blocks += len(fi.Blocks)
				if len(batch) == 1000 {
					writeErr = sdb.Update(folder, protocol.LocalDeviceID, batch)
					if writeErr != nil {
						slog.Error("Failed database write", slogutil.Error(writeErr))
						return
					}
					batch = batch[:0]
					if time.Since(t1) > 10*time.Second {
						d := time.Since(t0) + 1
						t1 = time.Now()
						slog.Info("Still migrating folder", "folder", folder, "files", files, "blocks", blocks, "duration", d.Truncate(time.Second), "blocksrate", float64(blocks)/d.Seconds(), "filesrate", float64(files)/d.Seconds())
					}
				}
			}
			if len(batch) > 0 {
				writeErr = sdb.Update(folder, protocol.LocalDeviceID, batch)
			}
			d := time.Since(t0) + 1
			slog.Info("Migrated folder", "folder", folder, "files", files, "blocks", blocks, "duration", d.Truncate(time.Second), "filesrate", float64(files)/d.Seconds())
			totFiles += files
			totBlocks += blocks
		}()

		// Iterate the existing files
		fs, err := olddb.NewFileSet(folder, ll)
		if err != nil {
			return err
		}
		snap, err := fs.Snapshot()
		if err != nil {
			return err
		}
		_ = snap.WithHaveSequence(0, func(fi protocol.FileInfo) bool {
			if deleteRetention > 0 && fi.Deleted && time.Since(fi.ModTime()) > deleteRetention {
				// Skip deleted files that match the garbage collection
				// criteria in the database
				return true
			}
			select {
			case fis <- fi:
				return true
			case <-writerDone:
				return false
			}
		})
		close(fis)
		snap.Release()

		// Wait for writes to complete
		wg.Wait()
		if writeErr != nil {
			return writeErr
		}
	}

	slog.Info("Migrating virtual mtimes...")
	if err := ll.IterateMtimes(sdb.PutMtime); err != nil {
		slog.Warn("Failed to migrate mtimes", slogutil.Error(err))
	}

	_ = miscDB.PutTime("migrated-from-leveldb-at", time.Now())
	_ = miscDB.PutString("migrated-from-leveldb-by", build.LongVersion)

	_ = be.Close()
	_ = os.Rename(oldDBDir, oldDBDir+"-migrated")

	slog.Info("Migration complete", "files", totFiles, "blocks", totBlocks/1000, "duration", time.Since(t0).Truncate(time.Second))
	return nil
}
