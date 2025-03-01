package sqlite

import (
	"context"
	"time"
)

func (s *DB) Serve(ctx context.Context) error {
	// Run periodic garbage collection
	timer := time.NewTimer(0)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}

		if err := s.periodic(ctx); err != nil {
			return err
		}

		timer.Reset(time.Hour)
	}
}

func (s *DB) periodic(ctx context.Context) error {
	s.updateLock.Lock()
	defer s.updateLock.Unlock()

	if err := s.garbageCollectBlocklistsAndBlocksLocked(ctx); err != nil {
		return err
	}

	_, _ = s.sql.Exec(`ANALYZE`)

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
		return wrap("garbage collect blocklists", err)
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys = 0`); err != nil {
		return wrap("garbage collect blocklists", err)
	}
	defer func() {
		_, _ = conn.ExecContext(context.Background(), `PRAGMA foreign_keys = 1`)
	}()

	tx, err := conn.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`
		DELETE FROM blocklists
		WHERE blocklist_hash NOT IN (
			SELECT blocklist_hash FROM files
		);`); err != nil {
		return wrap("garbage collect blocklists", err)
	}

	if _, err := tx.Exec(`
		DELETE FROM blocks
		WHERE blocklist_hash NOT IN (
			SELECT blocklist_hash FROM blocklists
		);`); err != nil {
		return wrap("garbage collect blocklists", err)
	}

	return wrap("garbage collect blocklists", tx.Commit())
}
