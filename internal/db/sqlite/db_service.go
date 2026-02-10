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
	"database/sql"

	"github.com/jmoiron/sqlx"
	"github.com/syncthing/syncthing/internal/db"
	"github.com/syncthing/syncthing/internal/slogutil"
	"github.com/syncthing/syncthing/lib/protocol"
)

const (
	internalMetaPrefix        = "dbsvc"
	lastMaintKey              = "lastMaint"
	lastSuccessfulGCSeqKey    = "lastSuccessfulGCSeq"
	mainDBMaintenanceInterval = 24 * time.Hour
	// initial and minimum target of prefix chunk size (among 2**32), this will increase to adapt to the DB speed
	gcMinChunkSize  = 128 // this is chosen to allow reaching 2**32 which is a full scan in 6 minutes
	gcTargetRuntime = 250 * time.Millisecond // max time to spend on gc, per table, per run
	vacuumPages     = 256 // pages are 4k with current SQLite this is 1M worth vaccumed
)

func (s *DB) Service(maintenanceInterval time.Duration) db.DBService {
	return newService(s, maintenanceInterval)
}

type Service struct {
	sdb                 *DB
	maintenanceInterval time.Duration
	nextMainDBMaintenance time.Time
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
		// Maybe superfluous, 1min wait is to spread start load
		nextMainDBMaintenance: time.Now().Add(time.Minute),
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

	defer func() { slog.DebugContext(ctx, "Periodic done", "duration", time.Since(t0)) }()

	// We reuse the periodic maintenance of folders which is done at short intervals to trigger
	// the main DB maintenance at long intervals (alternatively it could use its own Timer)
	if s.nextMainDBMaintenance.Before(time.Now()) {
		// Triggers the truncate checkpoint on the main DB
		// the main DB is very small it doesn't need frequent vacuum/checkpoints
		s.sdb.updateLock.Lock()
		err := tidy(ctx, s.sdb.sql, s.sdb.baseName, true)
		s.sdb.updateLock.Unlock()
		s.nextMainDBMaintenance = time.Now().Add(mainDBMaintenanceInterval)
		if err != nil { return err }
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
			// No change in DB, but incremental cleanups might have to finish their slow walk
			if fdb.hashCleanupCaughtUp() && fdb.namesCleanupCaughtUp() && fdb.versionsCleanupCaughtUp() {
				slog.DebugContext(ctx, "Skipping unnecessary GC", "folder", fdb.folderID, "fdb", fdb.baseName)
			} else {
				if !fdb.hashCleanupCaughtUp() {
					slog.DebugContext(ctx, "Catching up on hash cleanups", "folder",
						          fdb.folderID, "fdb", fdb.baseName)
					if err := func() error {
						fdb.updateLock.Lock()
						defer fdb.updateLock.Unlock()

						err := s.garbageCollectBlocklistsAndBlocksLocked(ctx, fdb, false)
						if err != nil {	return wrap(err) }
						return nil
					}(); err != nil { return wrap(err) }
				}
				if !fdb.namesCleanupCaughtUp() {
					slog.DebugContext(ctx, "Catching up on names cleanups", "folder",
						          fdb.folderID, "fdb", fdb.baseName)
					if err := func() error {
						fdb.updateLock.Lock()
						defer fdb.updateLock.Unlock()

						err := garbageCollectNamesOrVersions(ctx, fdb, "file_names", false)
						if err != nil {	return wrap(err) }
						return nil
					}(); err != nil { return wrap(err) }
				}
				if !fdb.versionsCleanupCaughtUp() {
					slog.DebugContext(ctx, "Catching up on versions cleanups", "folder",
						          fdb.folderID, "fdb", fdb.baseName)
					if err := func() error {
						fdb.updateLock.Lock()
						defer fdb.updateLock.Unlock()

						err := garbageCollectNamesOrVersions(ctx, fdb, "file_versions", false)
						if err != nil {	return wrap(err) }
						return nil
					}(); err != nil { return wrap(err) }
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
			if err := garbageCollectNamesOrVersions(ctx, fdb, "file_names", true); err != nil {
				return wrap(err)
			}
			if err := garbageCollectNamesOrVersions(ctx, fdb, "file_versions", true); err != nil {
				return wrap(err)
			}
			if err := s.garbageCollectBlocklistsAndBlocksLocked(ctx, fdb, true); err != nil {
				return wrap(err)
			}
			if fdb.nextCheckpoint.Before(time.Now()) {
				val := tidy(ctx, fdb.sql, fdb.baseName, true)
				fdb.nextCheckpoint = time.Now().Add(fdb.checkpointInterval)
				return val
			} else {
				return tidy(ctx, fdb.sql, fdb.baseName, false)
			}
		}(); err != nil { return wrap(err) }

		// Update the successful GC sequence.
		return wrap(meta.PutInt64(lastSuccessfulGCSeqKey, seq))
	}))
}

func (fdb *folderDB) hashCleanupCaughtUp() bool {
	return (fdb.coverage_full_at["blocks"] == (1 << 32)) && (fdb.coverage_full_at["blocklists"] == (1 << 32))
}
func (fdb *folderDB) namesCleanupCaughtUp() bool {
	return fdb.coverage_full_at["file_names"] == (1 << 62)
}
func (fdb *folderDB) versionsCleanupCaughtUp() bool {
	return fdb.coverage_full_at["file_versions"] == (1 << 62)
}

func tidy(ctx context.Context, db *sqlx.DB, name string, do_truncate_checkpoint bool) error {
	t0 := time.Now()
	defer func() { slog.DebugContext(ctx, "tidy", "database", name, "runtime", time.Since(t0)) }()

	conn, err := db.Conn(ctx)
	if err != nil { return wrap(err) }
	defer conn.Close()

	// Don't try to free too many pages at once by passing a maximum
	_, _ = conn.ExecContext(ctx, fmt.Sprintf(`PRAGMA incremental_vacuum(%d)`, vacuumPages))
	if do_truncate_checkpoint {
		// This is potentially really slow on a folderDB and is called after taking the updateLock
		_, _ = conn.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`)
	}
	return nil
}

func garbageCollectNamesOrVersions(ctx context.Context, fdb *folderDB, table string, device_seq_changed bool) error {
	chunkStart := fdb.cursor_values[table]
	chunkSize := fdb.chunk_sizes[table]
	l := slog.With("folder", fdb.folderID, "fdb", fdb.baseName, "table", table, "start", chunkStart, "chunk_size", chunkSize)

	t0 := time.Now()
	t1 := time.Now()
	defer func() {
		l.DebugContext(ctx, "GC runtime for " + table, "Total", time.Since(t0), "Chunk limits fetch", t1.Sub(t0),
			"Delete", time.Since(t1) )
	}()

	var chunkEnd sql.NullInt64
	err := fdb.stmt(`SELECT MAX(idx) FROM (SELECT idx FROM ` + table + ` WHERE idx >= ?
                         ORDER BY idx LIMIT ?)`).Get(&chunkEnd, chunkStart, chunkSize)
	if err != nil {
		l.WarnContext(ctx, table + ": max from chunk", "error", err)
		return wrap(err, "delete " + table)
	}
	l.DebugContext(ctx, table + " chunk end", "MAX result", chunkEnd)
	if !chunkEnd.Valid {
		// We reached the end of the idx range and found nothing
		l.DebugContext(ctx, table + ": MAX from chunk is NULL, end of table reached")
		fdb.cursor_values[table] = 0
		lastValid := chunkStart - 1
		if lastValid < 0 {
			// Empty table, coverage is always full
			fdb.coverage_full_at[table] = 1 << 62
		} else {
			if device_seq_changed {
				fdb.coverage_full_at[table] = lastValid
			} else {
				// the entry might have been GCed
				// if the lastValid idx entry was < to our target
				if fdb.coverage_full_at[table] > lastValid {
					fdb.coverage_full_at[table] = 1 << 62
				} // else still waiting to reach it by looping at the start
			}
		}
		// No need to adapt chunkSize, we didn't process anything
		return nil
	}
	intChunkEnd := chunkEnd.Int64
	var actualChunkSize int
	err = fdb.stmt(`SELECT COUNT(idx) FROM ` + table +
		       ` WHERE idx >= ? AND idx <= ?`).Get(&actualChunkSize, chunkStart, intChunkEnd)
	if err != nil {
		l.WarnContext(ctx, table + ": count from chunk", "error", err)
		return wrap(err, "delete " + table)
	}
	l.DebugContext(ctx, table + " actual chunk size", "result", actualChunkSize)

	t1 = time.Now()
	idx_column := "name_idx"
	if table == "file_versions" { idx_column = "version_idx" }
	res, err := fdb.stmt(`
		DELETE FROM ` + table + `
		WHERE idx IN (SELECT idx FROM ` + table + ` WHERE idx > ?
                              ORDER BY idx LIMIT ?)
                AND NOT EXISTS (SELECT 1 FROM files f WHERE f.` + idx_column + ` = idx)
	`).Exec(chunkStart, chunkSize)
	if err != nil {
		l.WarnContext(ctx, table + "delete failed", "error", err)
		return wrap(err, table + " DELETE")
	}
	if aff, err := res.RowsAffected(); err == nil {
		l.DebugContext(ctx, table + " DELETE", "affected", aff)
	}

	newChunkSize := adaptChunkSize(chunkSize, actualChunkSize, time.Since(t0), l, ctx)
	if (newChunkSize != 0) { chunkSize = newChunkSize }

	fdb.chunk_sizes[table] = chunkSize
	fdb.cursor_values[table] = intChunkEnd + 1

	// If the seq changed we record the end ot the last processed range
	if device_seq_changed {
		fdb.coverage_full_at[table] = intChunkEnd
	} else {
		// which idx do we target
		full_at := fdb.coverage_full_at[table]
		if (full_at >= chunkStart) && (full_at <= intChunkEnd)  {
			fdb.coverage_full_at[table] = 1 << 62
		}
	}
	return nil
}

func garbageCollectOldDeletedLocked(ctx context.Context, fdb *folderDB) error {
	l := slog.With("folder", fdb.folderID, "fdb", fdb.baseName, "retention", fdb.deleteRetention)
	t0 := time.Now()
	defer func() { l.DebugContext(ctx, "GC deleted files", "runtime", time.Since(t0)) }()

	if fdb.deleteRetention <= 0 {
		l.DebugContext(ctx, "Delete retention is infinite, skipping cleanup")
		return nil
	}

	// Remove deleted files that are marked as not needed (we have processed
	// them) and they were deleted more than MaxDeletedFileAge ago.
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
	tGlobal := time.Now()
	defer func() { slog.DebugContext(ctx, "GC blocks/blocklists", "runtime", time.Since(tGlobal)) }()

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
		// if not yet set, returns 0 which is what we need to init the process
		// these are int32 values mapped to the first 32 bits of the blocklist_hash values
		nextPrefix := int(fdb.cursor_values[table])
		chunkSize := fdb.chunk_sizes[table]

		l := slog.With("folder", fdb.folderID, "fdb", fdb.baseName, "table", table,
			"prefix", nextPrefix, "chunksize", chunkSize)

		if !device_seq_changed {
			// Did we caught up for this table
			if fdb.coverage_full_at[table] == (1 << 32) {
				l.DebugContext(ctx, "GC already completed")
				break
			}
		}

		// TODO: blobRange was inspired by the previous random implementation, cleanups still to do
		br := blobRange{nextPrefix, chunkSize}

		// The limit column must be an indexed column with a mostly random distribution of blobs.
		// That's the blocklist_hash column for blocklists, and the hash column for blocks.
		limitColumn := table + "." + fdb.cursor_columns[table]
		t0 := time.Now()
		limitCondition := br.SQL(limitColumn)
		// NOTE: the blocklists table is noticeably faster to process than the blocks table
		// blocks might need to be processed differently or have an index on blocklist_hash to iterate on
		// blocklist_hash instead
		q := fmt.Sprintf(`
				DELETE FROM %s
				WHERE %s AND NOT EXISTS (
					SELECT 1 FROM files WHERE files.blocklist_hash = %s.blocklist_hash
				)`, table, limitCondition, table)


		if res, err := tx.ExecContext(ctx, q); err != nil {
			l.DebugContext(ctx, "GC failed", "runtime", time.Since(t0), "error", err)
			return wrap(err, "delete from "+table)
		} else {
			l.DebugContext(ctx, "GC query result", "runtime", time.Since(t0), "result", slogutil.Expensive(func() any {
				rows, err := res.RowsAffected()
				if err != nil {
					return slogutil.Error(err)
				}
				return slog.Int64("affected_rows", rows)
			}))
		}

		newChunkSize := adaptChunkSize(chunkSize, br.actualChunkSize(), time.Since(t0), l, ctx)

		if newChunkSize != 0 { chunkSize = newChunkSize }

		// Store the next range
		newbr := br.next(chunkSize)
		fdb.cursor_values[table] = int64(newbr.start)
		fdb.chunk_sizes[table] = newbr.size

		// If the seq changed we record the beginning ot the last processed range
		if device_seq_changed {
			fdb.coverage_full_at[table] = int64(nextPrefix)
		} else {
			// the seq didn't change we must advance until we completed a full scan of the prefixes
			// which happens when a processed range covers our beginning recorded above
			if br.include(int(fdb.coverage_full_at[table])) {
				fdb.coverage_full_at[table] = 1 << 32
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

// actualChunkSize reflects the case where pagination couldn't fetch the asked chunkSize
// max chunk size is hard coded to 1<<32 which is fine for pagination and neeed for the iterations over hash values
// returns 0 if no change is needed
func adaptChunkSize(chunkSize int, actualChunkSize int, process_duration time.Duration, l *slog.Logger,
	            ctx context.Context) int {
	newChunkSize := 0
	// Did we overshoot the target runtime ?
	if process_duration > gcTargetRuntime {
		newChunkSize = max(chunkSize / 2, 128)
		l.DebugContext(ctx, "GC too aggressive, reducing speed", "new_chunk_size", newChunkSize)
	} else if (process_duration < (gcTargetRuntime / 2)) && (actualChunkSize == chunkSize) &&
		(chunkSize < (1 << 32)) {
		// Increase chunkSize based on the difference between max GC runtime and actual runtime
		// target 3/4 of the max
		// max speedup is 32 which makes allows reaching 1 << 32 in 6 passes
		// 32 = 2 ** 5, min is 128 = 2 ** 7
		speedup := min((3 * float64(gcTargetRuntime)) / (4 * float64(process_duration)), 32.0)
		newChunkSize = min(int(float64(chunkSize) * speedup), 1 << 32)
		l.DebugContext(ctx, "GC slow, increasing speed", "new_chunk_size", newChunkSize)
	}
	return newChunkSize
}
