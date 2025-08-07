// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package sqlite

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/syncthing/syncthing/internal/db"
	"github.com/syncthing/syncthing/internal/slogutil"
	"github.com/thejerf/suture/v4"
)

const (
	internalMetaPrefix     = "dbsvc"
	lastMaintKey           = "lastMaint"
	defaultDeleteRetention = 180 * 24 * time.Hour
	minDeleteRetention     = 24 * time.Hour
)

func (s *DB) Service(maintenanceInterval time.Duration) suture.Service {
	return newService(s, maintenanceInterval)
}

type Service struct {
	sdb                 *DB
	maintenanceInterval time.Duration
	internalMeta        *db.Typed
}

func (s *Service) String() string {
	return fmt.Sprintf("sqlite.service@%p", s)
}

func newService(sdb *DB, maintenanceInterval time.Duration) *Service {
	return &Service{
		sdb:                 sdb,
		maintenanceInterval: maintenanceInterval,
		internalMeta:        db.NewTyped(sdb, internalMetaPrefix),
	}
}

func (s *Service) Serve(ctx context.Context) error {
	// Run periodic maintenance

	// Figure out when we last ran maintenance and schedule accordingly. If
	// it was never, do it now.
	lastMaint, _, _ := s.internalMeta.Time(lastMaintKey)
	nextMaint := lastMaint.Add(s.maintenanceInterval)
	wait := time.Until(nextMaint)
	if wait < 0 {
		wait = time.Minute
	}
	slog.DebugContext(ctx, "Next periodic run due", "after", wait)

	timer := time.NewTimer(wait)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}

		if err := s.periodic(ctx); err != nil {
			return wrap(err)
		}

		timer.Reset(s.maintenanceInterval)
		slog.DebugContext(ctx, "Next periodic run due", "after", s.maintenanceInterval)
		_ = s.internalMeta.PutTime(lastMaintKey, time.Now())
	}
}

func (s *Service) periodic(ctx context.Context) error {
	t0 := time.Now()
	slog.DebugContext(ctx, "Periodic start")

	t1 := time.Now()
	defer func() { slog.DebugContext(ctx, "Periodic done in", "t1", time.Since(t1), "t0t1", t1.Sub(t0)) }()

	s.sdb.updateLock.Lock()
	err := tidy(ctx, s.sdb.sql)
	s.sdb.updateLock.Unlock()
	if err != nil {
		return err
	}

	return wrap(s.sdb.forEachFolder(func(fdb *folderDB) error {
		fdb.updateLock.Lock()
		defer fdb.updateLock.Unlock()

		if err := garbageCollectOldDeletedLocked(ctx, fdb); err != nil {
			return wrap(err)
		}
		if err := garbageCollectBlocklistsAndBlocksLocked(ctx, fdb); err != nil {
			return wrap(err)
		}
		return tidy(ctx, fdb.sql)
	}))
}

func tidy(ctx context.Context, db *sqlx.DB) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return wrap(err)
	}
	defer conn.Close()
	_, _ = conn.ExecContext(ctx, `ANALYZE`)
	_, _ = conn.ExecContext(ctx, `PRAGMA optimize`)
	_, _ = conn.ExecContext(ctx, `PRAGMA incremental_vacuum`)
	_, _ = conn.ExecContext(ctx, `PRAGMA journal_size_limit = 8388608`)
	_, _ = conn.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`)
	return nil
}

func garbageCollectOldDeletedLocked(ctx context.Context, fdb *folderDB) error {
	l := slog.With("fdb", fdb.baseDB)
	if fdb.deleteRetention <= 0 {
		slog.DebugContext(ctx, "Delete retention is infinite, skipping cleanup")
		return nil
	}

	// Remove deleted files that are marked as not needed (we have processed
	// them) and they were deleted more than MaxDeletedFileAge ago.
	l.DebugContext(ctx, "Forgetting deleted files", "retention", fdb.deleteRetention)
	res, err := fdb.stmt(`
		DELETE FROM files
		WHERE deleted AND modified < ? AND local_flags & {{.FlagLocalNeeded}} == 0
	`).Exec(time.Now().Add(-fdb.deleteRetention).UnixNano())
	if err != nil {
		return wrap(err)
	}
	if aff, err := res.RowsAffected(); err == nil {
		l.DebugContext(ctx, "Removed old deleted file records", "affected", aff)
	}
	return nil
}

func garbageCollectBlocklistsAndBlocksLocked(ctx context.Context, fdb *folderDB) error {
	// Remove all blocklists not referred to by any files and, by extension,
	// any blocks not referred to by a blocklist. This is an expensive
	// operation when run normally, especially if there are a lot of blocks
	// to collect.
	//
	// We make this orders of magnitude faster by disabling foreign keys for
	// the transaction and doing the cleanup manually. This requires using
	// an explicit connection and disabling foreign keys before starting the
	// transaction. We make sure to clean up on the way out.

	conn, err := fdb.sql.Connx(ctx)
	if err != nil {
		return wrap(err)
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys = 0`); err != nil {
		return wrap(err)
	}
	defer func() { //nolint:contextcheck
		_, _ = conn.ExecContext(context.Background(), `PRAGMA foreign_keys = 1`)
	}()

	tx, err := conn.BeginTxx(ctx, nil)
	if err != nil {
		return wrap(err)
	}
	defer tx.Rollback() //nolint:errcheck

	if res, err := tx.ExecContext(ctx, `
		DELETE FROM blocklists
		WHERE NOT EXISTS (
			SELECT 1 FROM files WHERE files.blocklist_hash = blocklists.blocklist_hash
		)`); err != nil {
		return wrap(err, "delete blocklists")
	} else {
		slog.DebugContext(ctx, "Blocklist GC", "fdb", fdb.baseName, "result", slogutil.Expensive(func() any {
			rows, err := res.RowsAffected()
			if err != nil {
				return slogutil.Error(err)
			}
			return slog.Int64("rows", rows)
		}))
	}

	if res, err := tx.ExecContext(ctx, `
		DELETE FROM blocks
		WHERE NOT EXISTS (
			SELECT 1 FROM blocklists WHERE blocklists.blocklist_hash = blocks.blocklist_hash
		)`); err != nil {
		return wrap(err, "delete blocks")
	} else {
		slog.DebugContext(ctx, "Blocks GC", "fdb", fdb.baseName, "result", slogutil.Expensive(func() any {
			rows, err := res.RowsAffected()
			if err != nil {
				return slogutil.Error(err)
			}
			return slog.Int64("rows", rows)
		}))
	}

	return wrap(tx.Commit())
}
