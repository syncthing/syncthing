package sqlite

import (
	"context"
	"time"
)

const dbMaintenanceInterval = time.Hour

func (s *DB) Serve(ctx context.Context) error {
	// Run periodic garbage collection
	timer := time.NewTimer(dbMaintenanceInterval / 2)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}

		if err := s.periodic(ctx); err != nil {
			return wrap(err)
		}

		timer.Reset(dbMaintenanceInterval)
	}
}

func (s *DB) periodic(ctx context.Context) error {
	t0 := time.Now()
	l.Debugln("Periodic start")

	s.updateLock.Lock()
	defer s.updateLock.Unlock()

	t1 := time.Now()
	defer func() { l.Debugln("Periodic done in", time.Since(t1), "+", t1.Sub(t0)) }()

	if err := s.garbageCollectBlocklistsAndBlocksLocked(ctx); err != nil {
		return wrap(err)
	}

	_, _ = s.sql.ExecContext(ctx, `ANALYZE`)
	_, _ = s.sql.ExecContext(ctx, `PRAGMA optimize`)
	_, _ = s.sql.ExecContext(ctx, `PRAGMA incremental_vacuum`)
	_, _ = s.sql.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`)

	return nil
}

func (s *DB) garbageCollectBlocklistsAndBlocksLocked(ctx context.Context) error {
	// Remove all blocklists not referred to by any files and, by extension,
	// any blocks not referred to by a blocklist. This is an expensive
	// operation when run normally, especially if there are a lot of blocks
	// to collect.
	//
	// We make this orders of magnitude faster by disabling foreign keys for
	// the transaction and doing the cleanup manually. This requires using
	// an explicit connection and disabling foreign keys before starting the
	// transaction. We make sure to clean up on the way out.

	conn, err := s.sql.Connx(ctx)
	if err != nil {
		return wrap(err)
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys = 0`); err != nil {
		return wrap(err)
	}
	defer func() {
		_, _ = conn.ExecContext(context.Background(), `PRAGMA foreign_keys = 1`)
	}()

	tx, err := conn.BeginTxx(ctx, nil)
	if err != nil {
		return wrap(err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
		DELETE FROM blocklists
		WHERE blocklist_hash NOT IN (
			SELECT blocklist_hash FROM files
		)`); err != nil {
		return wrap(err, "delete blocklists")
	}

	if _, err := tx.ExecContext(ctx, `
		DELETE FROM blocks
		WHERE blocklist_hash NOT IN (
			SELECT blocklist_hash FROM blocklists
		)`); err != nil {
		return wrap(err, "delete blocks")
	}

	return wrap(tx.Commit())
}
