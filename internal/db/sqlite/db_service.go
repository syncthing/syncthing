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
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/syncthing/syncthing/internal/db"
	"github.com/syncthing/syncthing/internal/slogutil"
	"github.com/syncthing/syncthing/lib/protocol"
)

const (
	internalMetaPrefix     = "dbsvc"
	lastMaintKey           = "lastMaint"
	lastSuccessfulGCSeqKey = "lastSuccessfulGCSeq"

	// initial and minimum target of prefix chunk size (among 2**32), this will increase to adapt to the DB speed
	gcMinChunkSize  = 128 // this is chosen to allow reaching 2**32 which is a full scan in 6 minutes
	gcTargetRuntime = 250 * time.Millisecond // max time to spend on gc, per table, per run
)

func (s *DB) Service(maintenanceInterval time.Duration) db.DBService {
	return newService(s, maintenanceInterval)
}

type Service struct {
	sdb                 *DB
	maintenanceInterval time.Duration
	internalMeta        *db.Typed
	start               chan struct{}
}

func (s *Service) String() string {
	return fmt.Sprintf("sqlite.service@%p", s)
}

func newService(sdb *DB, maintenanceInterval time.Duration) *Service {
	return &Service{
		sdb:                 sdb,
		maintenanceInterval: maintenanceInterval,
		internalMeta:        db.NewTyped(sdb, internalMetaPrefix),
		start:               make(chan struct{}),
	}
}

func (s *Service) StartMaintenance() {
	select {
	case s.start <- struct{}{}:
	default:
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

	if s.maintenanceInterval == 0 {
		timer.Stop()
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		case <-s.start:
		}

		if err := s.periodic(ctx); err != nil {
			return wrap(err)
		}

		if s.maintenanceInterval != 0 {
			timer.Reset(s.maintenanceInterval)
			slog.DebugContext(ctx, "Next periodic run due", "after", s.maintenanceInterval)
		}

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
		// Get the current device sequence, for comparison in the next step.
		seq, err := fdb.GetDeviceSequence(protocol.LocalDeviceID)
		if err != nil {
			return wrap(err)
		}
		// Get the last successful GC sequence. If it's the same as the
		// current sequence, nothing has changed and we can skip the GC
		// entirely.
		meta := db.NewTyped(fdb, internalMetaPrefix)
		if prev, _, err := meta.Int64(lastSuccessfulGCSeqKey); err != nil {
			return wrap(err)
		} else if seq == prev {
			if fdb.hashCleanupCaughtUp() {
				slog.DebugContext(ctx, "Skipping unnecessary GC", "folder", fdb.folderID, "fdb", fdb.baseName)
			} else {
				slog.DebugContext(ctx, "Catching up on hash cleanups", "folder", fdb.folderID, "fdb", fdb.baseName)				// Blocks and blocklists to delete might still exist
				// as their GC returns without error on timeout
				// TODO remember where we started our cleanup and stop when a full pass was done
				if err := func() error {
					fdb.updateLock.Lock()
					defer fdb.updateLock.Unlock()

					if err := s.garbageCollectBlocklistsAndBlocksLocked(ctx, fdb, false); err != nil {
						return wrap(err)
					}
					return nil
				}(); err != nil {
					return wrap(err)
				}
			}
			return nil
		}

		// Run the GC steps, in a function to be able to use a deferred
		// unlock.
		if err := func() error {
			fdb.updateLock.Lock()
			defer fdb.updateLock.Unlock()

			if err := garbageCollectOldDeletedLocked(ctx, fdb); err != nil {
				return wrap(err)
			}
			if err := garbageCollectNamesAndVersions(ctx, fdb); err != nil {
				return wrap(err)
			}
			if err := s.garbageCollectBlocklistsAndBlocksLocked(ctx, fdb, true); err != nil {
				return wrap(err)
			}
			return tidy(ctx, fdb.sql)
		}(); err != nil {
			return wrap(err)
		}

		// Update the successful GC sequence.
		return wrap(meta.PutInt64(lastSuccessfulGCSeqKey, seq))
	}))
}

func (fdb *folderDB) hashCleanupCaughtUp() bool {
	return (fdb.targetBlocksStart == (1 << 32)) && (fdb.targetBlocklistsStart == (1 << 32))
}

func tidy(ctx context.Context, db *sqlx.DB) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return wrap(err)
	}
	defer conn.Close()
	_, _ = conn.ExecContext(ctx, `PRAGMA incremental_vacuum`)
	_, _ = conn.ExecContext(ctx, `PRAGMA journal_size_limit = 8388608`)
	_, _ = conn.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`)
	return nil
}

func garbageCollectNamesAndVersions(ctx context.Context, fdb *folderDB) error {
	l := slog.With("folder", fdb.folderID, "fdb", fdb.baseName)

	res, err := fdb.stmt(`
		DELETE FROM file_names
		WHERE NOT EXISTS (SELECT 1 FROM files f WHERE f.name_idx = idx)
	`).Exec()
	if err != nil {
		return wrap(err, "delete names")
	}
	if aff, err := res.RowsAffected(); err == nil {
		l.DebugContext(ctx, "Removed old file names", "affected", aff)
	}

	res, err = fdb.stmt(`
		DELETE FROM file_versions
		WHERE NOT EXISTS (SELECT 1 FROM files f WHERE f.version_idx = idx)
	`).Exec()
	if err != nil {
		return wrap(err, "delete versions")
	}
	if aff, err := res.RowsAffected(); err == nil {
		l.DebugContext(ctx, "Removed old file versions", "affected", aff)
	}

	return nil
}

func garbageCollectOldDeletedLocked(ctx context.Context, fdb *folderDB) error {
	l := slog.With("folder", fdb.folderID, "fdb", fdb.baseName)
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

func (s *Service) garbageCollectBlocklistsAndBlocksLocked(ctx context.Context, fdb *folderDB, device_seq_changed bool) error {
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

	// Both blocklists and blocks refer to blocklists_hash from the files table.
	for _, table := range []string{"blocklists", "blocks"} {
		// Find where the last GC stopped and what is the recommended range to process
		prefixKey := "next" + table + "HashPrefix"
		chunkSizeKey := table + "ChunkSize"

		// if not yet set, returns 0 which is what we need to init the process
		// these are int32 values mapped to the first 32 bits of the blocklist_hash values
		nextPrefix64, _, _ := s.internalMeta.Int64(prefixKey)
		chunkSize64, _, _ := s.internalMeta.Int64(chunkSizeKey)
		nextPrefix := int(nextPrefix64)
		chunkSize := max(gcMinChunkSize, int(chunkSize64))

		l := slog.With("folder", fdb.folderID, "fdb", fdb.baseName, "table", table,
			"prefix", nextPrefix, "chunksize", chunkSize)

		if !device_seq_changed {
			// Did we caught up for this table
			if (table == "blocks") && (fdb.targetBlocksStart == (1 << 32)) {
				l.DebugContext(ctx, "GC already completed")
				break
			}
			if (table == "blocklists") && (fdb.targetBlocklistsStart == (1 << 32)) {
				l.DebugContext(ctx, "GC already completed")
				break
			}
		}

		// TODO: blobRange was inspired by the previous random implementation, cleanups still to do
		br := blobRange{nextPrefix, chunkSize}

		// The limit column must be an indexed column with a mostly random distribution of blobs.
		// That's the blocklist_hash column for blocklists, and the hash column for blocks.
		limitColumn := table + ".blocklist_hash"
		if table == "blocks" {
			limitColumn = "blocks.hash"
		}

		t0 := time.Now()
		limitCondition := br.SQL(limitColumn)
		q := fmt.Sprintf(`
				DELETE FROM %s
				WHERE %s AND NOT EXISTS (
					SELECT 1 FROM files WHERE files.blocklist_hash = %s.blocklist_hash
				)`, table, limitCondition, table)


		if res, err := tx.ExecContext(ctx, q); err != nil {
			l.DebugContext(ctx, "GC failed", "table", table, "runtime", time.Since(t0), "error", err)
			return wrap(err, "delete from "+table)
		} else {
			l.DebugContext(ctx, "GC query result", "runtime", time.Since(t0), "result", slogutil.Expensive(func() any {
				rows, err := res.RowsAffected()
				if err != nil {
					return slogutil.Error(err)
				}
				return slog.Int64("rows", rows)
			}))
		}

		// Did we overshoot the target runtime ?
		actualChunkSize := br.actualChunkSize()
		if d := time.Since(t0); d > gcTargetRuntime {
			// Reduce chunkSize (note : minimum is enforced when loading value)
			chunkSize = actualChunkSize / 2
			l.DebugContext(ctx, "GC too aggressive, reducing speed", "table", table, "new_chunk_size", chunkSize)
		} else if (d < (gcTargetRuntime / 2)) && (actualChunkSize == chunkSize) && (chunkSize < (1 << 32)) {
			// Increase chunkSize based on the difference between max GC runtime and actual runtime
			// target 3/4 of the max
			// max speedup is 32 which makes allows reaching the maxChunkSize in 6 passes
			// 32 = 2 ** 5, min is 128 = 2 ** 7
			speedup := min((3 * float64(gcTargetRuntime)) / (4 * float64(d)), 32.0)
			chunkSize = min(int(float64(chunkSize) * speedup), 1 << 32)
			l.DebugContext(ctx, "GC slow, increasing speed", "table", table, "new_chunk_size", chunkSize)
		}
		// Store the next range
		newbr := br.next(chunkSize)
		s.internalMeta.PutInt64(prefixKey, int64(newbr.start))
		s.internalMeta.PutInt64(chunkSizeKey, int64(newbr.size))


		// If the seq changed we record the beginning ot the last processed range
		if device_seq_changed {
			if table == "blocks" {
				fdb.targetBlocksStart = nextPrefix64
			} else if table == "blocklists" {
				fdb.targetBlocklistsStart = nextPrefix64
			}
		} else {
			// the seq didn't change we must advance until we completed a full scan of the prefixes
			// which happens when a processed range covers our beginning recorded above
			if table == "blocks" {
				if br.include(int(fdb.targetBlocksStart)) {
					// Mark this table done
					fdb.targetBlocksStart = 1 << 32
				}
			} else if table == "blocklists" {
				if br.include(int(fdb.targetBlocklistsStart)) {
					// Mark this table done
					fdb.targetBlocklistsStart = 1 << 32
				}
			}
		}
	}

	return wrap(tx.Commit())
}

// blobRange defines a range for blob searching.
// it is initialized with a chunk size and computes the appropriate end
type blobRange struct {
	start, size int
}

func (r blobRange) end() int {
	stop := r.start + r.size
	if stop >= (1 << 32) {
		return (1 << 32)
	} else {
		return stop
	}
}

func (r blobRange) next(size int) blobRange {
	start := r.end()
	if start == (1 << 32) {
		start = 0
	}
	return blobRange{start, size}
}

func (r blobRange) include(position int) bool {
	if (position >= r.start) && (position < r.end()) {
		return true
	} else {
		return false
	}
}

// return the actual size being processed (the last chunk is usually shorter than chunkSize)
func (r blobRange) actualChunkSize() int {
	prefixesRemaining := (1 << 32) - r.start
	if (r.size > prefixesRemaining) {
		return prefixesRemaining
	} else {
		return r.size
	}
}

// SQL returns the SQL where clause for the given range, e.g.
// `column >= x'49249248' AND column < x'6db6db6c'`
func (r blobRange) SQL(name string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s >= x'%08X'", name, r.start)
	end := r.end()
	if end != (1 << 32) {
		sb.WriteString(" AND ")
		fmt.Fprintf(&sb, "%s < x'%08X'", name, end)
	}
	return sb.String()
}
