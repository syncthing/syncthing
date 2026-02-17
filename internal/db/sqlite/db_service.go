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
	lastSuccessfulGCSeqKey    = "lastSuccessfulGCSeq"
	mainDBMaintenanceInterval = 24 * time.Hour
	// initial and minimum target of prefix chunk size (among 2**32), this will increase to adapt to the DB speed
	gcMinChunkSize  = 128 // this is chosen to allow reaching 2**32 which is a full scan in 6 minutes
	gcMaxChunkSize  = 1 << 32 // Should be able to cover all tables in a single pass
	gcTargetRuntime = 250 * time.Millisecond // max time to spend on gc, per table, per run
	vacuumPages     = 256 // pages are 4k with current SQLite this is 1M worth vaccumed
	// don't wait more than that to check if GC work is needed, useful when adding a new Folder
	// do we need a larger period on Mobile to conserve battery ?
	maxIncrementalGCPeriod = 5 * time.Minute
	// gcTargetRuntime is 20 times less and used for 5 cleanups for each folder
	minIncrementalGCPeriod = 5 * time.Second
	rowCountsValidFor = time.Hour
)

func (s *DB) Service(maintenanceInterval time.Duration) db.DBService {
	return newService(s, maintenanceInterval)
}

type Service struct {
	sdb                   *DB
	// This is the default maintenance Interval
	// folders can increase or decrease it for their own needs
	maintenanceInterval   time.Duration
	nextMainDBMaintenance time.Time
	internalMeta          *db.Typed
	start                 chan chan error
}

func (s *Service) String() string {
	return fmt.Sprintf("sqlite.service@%p", s)
}

func newService(sdb *DB, maintenanceInterval time.Duration) *Service {
	return &Service{
		sdb:                 sdb,
		maintenanceInterval: maintenanceInterval,
		internalMeta:        db.NewTyped(sdb, internalMetaPrefix),
		start:               make(chan chan error),
		// Maybe superfluous, 1min wait is to spread start load
		nextMainDBMaintenance: time.Now().Add(time.Minute),
	}
}

func (s *Service) StartMaintenance() <-chan error {
	finishChan := make(chan error, 1)
	select {
	case s.start <- finishChan:
	default:
	}
	return finishChan
}

func (s *Service) Serve(ctx context.Context) error {
	// Run periodic maintenance shortly after start
	timer := time.NewTimer(10 * time.Second)
	if s.maintenanceInterval == 0 { timer.Stop() }

	for {
		var finishChan chan error
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		case finishChan = <-s.start:
		}

		err := s.periodic(ctx)
		if finishChan != nil {
			finishChan <- err
		}

		wait := time.Until(s.nextFolderMaintenance())
		if err != nil {
			timer.Reset(wait)
			slog.WarnContext(ctx, "Periodic run failed", "err", err)
			return wrap(err)
		}

		if s.maintenanceInterval != 0 {
			timer.Reset(wait)
			slog.DebugContext(ctx, "Next periodic run due", "after", wait)
		}
	}
}

func (s *Service) periodic(ctx context.Context) error {
	// We reuse the periodic maintenance of folders which is done at short intervals to trigger
	// the main DB maintenance at long intervals (alternatively it could use its own Timer)
	if s.nextMainDBMaintenance.Before(time.Now()) {
		// Log this to make clear the tidy debug logs are for the main DB
		slog.DebugContext(ctx, "Main DB: VACCUUM and WAL TRUNCATE")
		// Triggers the truncate checkpoint on the main DB
		// the main DB is very small it doesn't need frequent vacuum/checkpoints
		s.sdb.updateLock.Lock()
		err := tidy(ctx, s.sdb.sql, s.sdb.baseName, true)
		s.sdb.updateLock.Unlock()
		s.nextMainDBMaintenance = time.Now().Add(mainDBMaintenanceInterval)
		if err != nil { return err }
	}

	return wrap(s.sdb.forEachFolder(func(fdb *folderDB) error {
		// Don't process until it is our time
		if fdb.nextCleanup.After(time.Now()) { return nil }

		t0 := time.Now()
		db_update_detected := false
		work_done := false
		// Adjust the cleanup frequency based on the expected work to be done
		defer func() {
			if db_update_detected || !fdb.cleanupsCaughtUp() {
				fdb.setNextCleanupFromConstraints(ctx, s)
			} else {
				fdb.nextCleanup = time.Now().Add(maxIncrementalGCPeriod)
			}
			if work_done { slog.DebugContext(ctx, "Cleanups", "Total runtime", time.Since(t0)) }
		}()

		// Get the current device sequence, for comparison in the next step.
		seq, err := fdb.GetDeviceSequence(protocol.LocalDeviceID)
		if err != nil { return wrap(err) }
		slog.DebugContext(ctx, fmt.Sprintf("## %s CLEANUPS ##", fdb.logID()))

		// Get the last successful GC sequence. If it's the same as the
		// current sequence, nothing has changed and we can skip the GC
		// once all table rows have been processed
		// NOTE: the coverage isn't perfect, on large installations deleted files can take time to be GC and
		// the other tables are being processed before the hashes disappear. This is auto-corrected though
		// and the missing cleanups should be limited to a small portion of hashes as the other tables are far
		// costlier/longer to clean
		meta := db.NewTyped(fdb, internalMetaPrefix)
		if prev, _, err := meta.Int64(lastSuccessfulGCSeqKey); err != nil {
			return wrap(err)
		} else if db_update_detected = (seq != prev); !db_update_detected {
			// No change in DB, but incremental cleanups might have to finish their slow walk
			if !fdb.cleanupsCaughtUp() {
				work_done = true
				if !fdb.filesCleanupCaughtUp() {
					slog.DebugContext(ctx, "Catching up on files cleanups", "folder(db)", fdb.logID())
					if err := func() error {
						fdb.updateLock.Lock()
						defer fdb.updateLock.Unlock()

						err := garbageCollectOldDeletedLocked(ctx, fdb, false)
						if err != nil {	return wrap(err) }
						return nil
					}(); err != nil { return wrap(err) }
				}
				if !fdb.hashCleanupCaughtUp() {
					slog.DebugContext(ctx, "Catching up on hash cleanups", "folder(db)", fdb.logID())
					if err := func() error {
						fdb.updateLock.Lock()
						defer fdb.updateLock.Unlock()

						err := s.garbageCollectBlocklistsAndBlocksLocked(ctx, fdb, false)
						if err != nil {	return wrap(err) }
						return nil
					}(); err != nil { return wrap(err) }
				}
				if !fdb.namesCleanupCaughtUp() {
					slog.DebugContext(ctx, "Catching up on names cleanups", "folder(db)", fdb.logID())
					if err := func() error {
						fdb.updateLock.Lock()
						defer fdb.updateLock.Unlock()

						err := garbageCollectNamesOrVersions(ctx, fdb, "file_names", false)
						if err != nil {	return wrap(err) }
						return nil
					}(); err != nil { return wrap(err) }
				}
				if !fdb.versionsCleanupCaughtUp() {
					slog.DebugContext(ctx, "Catching up on versions cleanups", "folder(db)", fdb.logID())
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

			work_done = true
			if err := garbageCollectOldDeletedLocked(ctx, fdb, true); err != nil {
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
			if fdb.nextTruncate.Before(time.Now()) {
				val := tidy(ctx, fdb.sql, fdb.baseName, true)
				fdb.nextTruncate = time.Now().Add(fdb.truncateInterval)
				return val
			} else {
				return tidy(ctx, fdb.sql, fdb.baseName, false)
			}
		}(); err != nil { return wrap(err) }

		// Update the successful GC sequence.
		return wrap(meta.PutInt64(lastSuccessfulGCSeqKey, seq))
	}))
}

func (s *Service) nextFolderMaintenance() time.Time {
	// Use a sensible max value in case a new Folder is created after this
	earliestMaintenance := time.Now().Add(maxIncrementalGCPeriod)
	s.sdb.forEachFolder(func(fdb *folderDB) error {
		nextCleanup := fdb.nextCleanup
		if nextCleanup.Before(earliestMaintenance) {
			earliestMaintenance = nextCleanup
		}
		return nil
	})
	return earliestMaintenance
}

// Try to target a full cleanup in the configured maintenanceInterval
// but not exceeding the max frequency set by minIncrementalGCPeriod
// the computed chunk size for each table is used as the base of the computation
// its value is computed to avoid incremental GC exceeding gcTargetRuntime
func (fdb *folderDB) setNextCleanupFromConstraints(ctx context.Context, s *Service) {
	fdb.updateCountWhenNeeded()
	chunkInterval := maxIncrementalGCPeriod
	interval_ms := s.maintenanceInterval.Milliseconds()
	for _, table := range []string{"files", "file_names", "file_versions"} {
		target_interval := fdb.chunkIntervalFor(table, interval_ms)
		slog.DebugContext(ctx, "Interval for table", table, target_interval)
		if (chunkInterval > target_interval) { chunkInterval = target_interval }
	}
	for _, table := range []string{"blocks", "blocklists"} {
		target_interval := fdb.chunkIntervalFor(table, interval_ms)
		slog.DebugContext(ctx, "Interval for table", table, target_interval)
		if (chunkInterval > target_interval) { chunkInterval = target_interval }
	}
	// reduce the interval to account for the tables' cleanups
	chunkInterval -= 5 * gcTargetRuntime
	if chunkInterval < minIncrementalGCPeriod { chunkInterval = minIncrementalGCPeriod }
	fdb.nextCleanup = time.Now().Add(chunkInterval)
	slog.DebugContext(ctx, "Interval result", "interval", chunkInterval)
}

func (fdb *folderDB) chunkIntervalFor(table string, interval_ms int64) time.Duration {
	total_range, ok := fdb.countEstimation[table]
	// Exception for hash based ranges
	if !ok { total_range = (1 << 32) }
	// Avoid a divide by 0
	if total_range == 0 { total_range = 1 }
	return time.Duration(interval_ms * fdb.chunkSizes[table] / total_range)
}

func (fdb *folderDB) updateCountWhenNeeded() {
	for _, table := range []string{"files", "file_names", "file_versions"} {
		if !time.Now().After(fdb.countValidUntil[table]) { break }
		var rows int64
		query := fmt.Sprintf(`SELECT CAST(SUBSTR(stat, 1, INSTR(stat, ' ') - 1) AS INTEGER)"
                                      FROM sqlite_stat1 WHERE tbl = '%s' LIMIT 1`, table)
		err := fdb.stmt(query).Get(&rows, table);
		if err != nil { return }
		oldcount := fdb.countEstimation[table]
		fdb.countEstimation[table] = rows
		fdb.countValidUntil[table] = time.Now().Add(rowCountsValidFor)
		slog.Debug("Table row count", table + "old", oldcount, "new", rows)
	}
}

func (fdb *folderDB) cleanupsCaughtUp() bool {
	return (fdb.filesCleanupCaughtUp() && fdb.hashCleanupCaughtUp() && fdb.namesCleanupCaughtUp() &&
		fdb.versionsCleanupCaughtUp())
}
func (fdb *folderDB) filesCleanupCaughtUp() bool {
	return fdb.coverageFullAt["files"] == sqliteInt64CursorValueWhenDone
}
func (fdb *folderDB) hashCleanupCaughtUp() bool {
	return (fdb.coverageFullAt["blocks"] == hashPrefixCeiling) &&
		(fdb.coverageFullAt["blocklists"] == hashPrefixCeiling)
}
func (fdb *folderDB) namesCleanupCaughtUp() bool {
	return fdb.coverageFullAt["file_names"] == sqliteInt64CursorValueWhenDone
}
func (fdb *folderDB) versionsCleanupCaughtUp() bool {
	return fdb.coverageFullAt["file_versions"] == sqliteInt64CursorValueWhenDone
}

func tidy(ctx context.Context, db *sqlx.DB, name string, do_truncate_checkpoint bool) error {
	t0 := time.Now()
	t1 := time.Now()
	defer func() {
		if do_truncate_checkpoint {
			slog.DebugContext(ctx, "tidy runtime", "database", name, "VACCUM", t1.Sub(t0),
				"TRUNCATE", time.Since(t1))
		} else {
			slog.DebugContext(ctx, "tidy runtime", "database", name, "VACCUM", t1.Sub(t0))
		}
	}()

	conn, err := db.Conn(ctx)
	if err != nil { return wrap(err) }
	defer conn.Close()

	// Don't try to free too many pages at once by passing a maximum
	_, _ = conn.ExecContext(ctx, fmt.Sprintf(`PRAGMA incremental_vacuum(%d)`, vacuumPages))
	t1 = time.Now()
	// This is potentially really slow on a folderDB and is called after taking the updateLock
	if do_truncate_checkpoint { _, _ = conn.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`) }
	return nil
}

func garbageCollectNamesOrVersions(ctx context.Context, fdb *folderDB, table string, device_seq_changed bool) error {
	chunkStart := fdb.cursorValues[table]
	chunkSize := fdb.chunkSizes[table]
	partialChunk := false

	l := slog.With("folder(db)", fdb.logID(), "table", table, "start", chunkStart, "chunk_size", chunkSize)

	t0 := time.Now()
	t1 := time.Now()
	defer func() {
		l.DebugContext(ctx, "GC runtime", "Total", time.Since(t0), "Chunk limits fetch", t1.Sub(t0),
			"Delete", time.Since(t1) )
	}()

	var chunkEnd sql.NullInt64
	// Try to fetch the end of a full chunk
	_ = fdb.stmt(`SELECT idx FROM ` + table + ` WHERE idx >= ?
                      ORDER BY idx LIMIT 1 OFFSET ?`).Get(&chunkEnd, chunkStart, chunkSize - 1)
	// Error err is normal (no rows in resultset expected)
	if !chunkEnd.Valid {
		// End not found, partial chunk or end already reached
		// Find the end of the idx range to get a partial chunk
		err := fdb.stmt(`SELECT MAX(idx) FROM ` + table).Get(&chunkEnd)
		if err != nil {
			l.WarnContext(ctx, table + " MAX(idx) failed", "error", err)
			return wrap(err, table + " DELETE")
		}
		partialChunk = true
	}
	intChunkEnd := chunkEnd.Int64 // 0 unless valid
	l.DebugContext(ctx, "chunk_end", "full chunk", !partialChunk, "last idx", intChunkEnd)
	t1 = time.Now()

	if chunkEnd.Valid {
		// Next chunk start position
		if partialChunk {
			fdb.cursorValues[table] = 0
		} else {
			fdb.cursorValues[table] = intChunkEnd + 1
		}

		idx_column := "name_idx"
		if table == "file_versions" { idx_column = "version_idx" }
		res, err := fdb.stmt(`DELETE FROM ` + table + `
                                      WHERE idx >= ? AND idx <= ?
                                      AND NOT EXISTS (SELECT 1 FROM files f WHERE f.` + idx_column + ` = idx)
	                             `).Exec(chunkStart, intChunkEnd)
		if err != nil {
			l.WarnContext(ctx, "delete failed", "error", err)
			return wrap(err, table + " DELETE")
		}
		if aff, err := res.RowsAffected(); err == nil {
			l.DebugContext(ctx, "DELETE", "affected", aff)
		}

		// Pass information about unknown chunk size (0) when needed
		var actualChunkSize int64
		if partialChunk { actualChunkSize = 0 } else { actualChunkSize = chunkSize }
		newChunkSize := adaptChunkSize(chunkSize, actualChunkSize, time.Since(t0), l, ctx)
		if (newChunkSize != 0) { fdb.chunkSizes[table] = newChunkSize }
	}

	// If the seq changed we record the end of the last processed range
	if device_seq_changed {
		if chunkEnd.Valid {
			fdb.coverageFullAt[table] = intChunkEnd
		} // else no chunk processed, last end is still applicable
	} else {
		// which idx do we target
		full_at := fdb.coverageFullAt[table]
		if (full_at >= chunkStart) && ((full_at <= intChunkEnd) || partialChunk) {
			fdb.coverageFullAt[table] = sqliteInt64CursorValueWhenDone
		}
	}
	return nil
}

// TODO: factor code with GC for Names/Versions
func garbageCollectOldDeletedLocked(ctx context.Context, fdb *folderDB, device_seq_changed bool) error {
	chunkStart := fdb.cursorValues["files"]
	chunkSize := fdb.chunkSizes["files"]
	partialChunk := false
	l := slog.With("folder(db)", fdb.logID(), "table", "files", "start", chunkStart, "chunk_size", chunkSize,
		"retention", fdb.deleteRetention)
	if fdb.deleteRetention <= 0 {
		l.DebugContext(ctx, "Delete retention is infinite, skipping cleanup")
		return nil
	}

	t0 := time.Now()
	t1 := time.Now()
	defer func() {
		l.DebugContext(ctx, "GC runtime for deleted files", "Total", time.Since(t0),
			"Chunk limits fetch", t1.Sub(t0), "Delete", time.Since(t1))
	}()

	var chunkEnd sql.NullInt64
	// Try to fetch the end of a full chunk
	_ = fdb.stmt(`SELECT sequence FROM files WHERE sequence >= ?
                      ORDER BY sequence LIMIT 1 OFFSET ?`).Get(&chunkEnd, chunkStart, chunkSize - 1)
	// Error err is normal (no rows in resultset expected)
	if !chunkEnd.Valid {
		// End not found, partial chunk or end already reached
		// Find the end of the idx range to get a partial chunk
		err := fdb.stmt(`SELECT MAX(sequence) FROM files`).Get(&chunkEnd)
		if err != nil {
			l.WarnContext(ctx, "MAX(sequence) failed", "error", err)
			return wrap(err, "DELETE")
		}
		partialChunk = true
	}
	intChunkEnd := chunkEnd.Int64 // 0 unless valid
	l.DebugContext(ctx, "chunk_end", "full chunk", !partialChunk, "last seq", intChunkEnd)
	t1 = time.Now()

	if chunkEnd.Valid {
		intChunkEnd = chunkEnd.Int64
		// Next chunk start position
		if partialChunk {
			fdb.cursorValues["files"] = 0
		} else {
			fdb.cursorValues["files"] = intChunkEnd + 1
		}

		// Remove deleted files that are marked as not needed (we have processed
		// them) and they were deleted more than MaxDeletedFileAge ago.
		res, err := fdb.stmt(`
		DELETE FROM files
		WHERE deleted AND sequence >= ? AND sequence <= ? AND modified < ?
                AND local_flags & {{.FlagLocalNeeded}} == 0
	        `).Exec(chunkStart, intChunkEnd, time.Now().Add(-fdb.deleteRetention).UnixNano())
		if err != nil {
			l.WarnContext(ctx, "files DELETE failed", "error", err)
			return wrap(err, "files DELETE")
		}
		if aff, err := res.RowsAffected(); err == nil {
			l.DebugContext(ctx, "files DELETE", "affected", aff)
		}

		// Pass information about unknown chunk size (0) when needed
		var actualChunkSize int64
		if partialChunk { actualChunkSize = 0 } else { actualChunkSize = chunkSize }
		newChunkSize := adaptChunkSize(chunkSize, actualChunkSize, time.Since(t0), l, ctx)
		if (newChunkSize != 0) { fdb.chunkSizes["files"] = newChunkSize }
	}

	// If the seq changed we record the end of the last processed range
	if device_seq_changed {
		if chunkEnd.Valid {
			fdb.coverageFullAt["files"] = intChunkEnd
		} // else no chunk processed, last end is still applicable
	} else {
		// which idx do we target
		full_at := fdb.coverageFullAt["files"]
		if (full_at >= chunkStart) && ((full_at <= intChunkEnd) || partialChunk) {
			fdb.coverageFullAt["files"] = sqliteInt64CursorValueWhenDone
		}
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
		nextPrefix := fdb.cursorValues[table]
		chunkSize := fdb.chunkSizes[table]

		// General case
		l := slog.With("folder(db)", fdb.logID(), "table", table, "prefix", nextPrefix, "chunksize", chunkSize)
		// Shorter log when doing a full scan
		if (nextPrefix == 0) && (chunkSize == hashPrefixCeiling) {
			l = slog.With("FULL SCAN folder(db)", fdb.logID(), "table", table)
		}

		if !device_seq_changed {
			// Did we caught up for this table
			if fdb.coverageFullAt[table] == hashPrefixCeiling {
				l.DebugContext(ctx, "GC already completed")
				break
			}
		}

		// TODO: blobRange was inspired by the previous random implementation, cleanups still to do
		br := blobRange{nextPrefix, chunkSize}

		// The limit column must be an indexed column with a mostly random distribution of blobs.
		// That's the blocklist_hash column for blocklists, and the hash column for blocks.
		limitColumn := table + "." + folderTableCursorColumns[table]
		t0 := time.Now()
		limitCondition := br.SQL(limitColumn)
		// NOTE: the blocklists table is noticeably faster to process than the blocks table
		// blocks might need to be processed differently or have an index on blocklist_hash to iterate on
		// blocklist_hash instead
		q := fmt.Sprintf(`
				DELETE FROM %s
				WHERE %s NOT EXISTS (
					SELECT 1 FROM files WHERE files.blocklist_hash = %s.blocklist_hash
				)`, table, limitCondition, table)


		if res, err := tx.ExecContext(ctx, q); err != nil {
			l.DebugContext(ctx, "GC failed", "runtime", time.Since(t0), "error", err)
			return wrap(err, "delete from "+table)
		} else {
			l.DebugContext(ctx, "GC query result", "runtime", time.Since(t0), "result",
				slogutil.Expensive(func() any {
				rows, err := res.RowsAffected()
				if err != nil { return slogutil.Error(err) }
				return slog.Int64("affected_rows", rows)
			}))
		}

		newChunkSize := adaptChunkSize(chunkSize, br.actualChunkSize(), time.Since(t0), l, ctx)

		if newChunkSize != 0 { chunkSize = newChunkSize }

		// Store the next range
		newbr := br.next(chunkSize)
		fdb.cursorValues[table] = int64(newbr.start)
		fdb.chunkSizes[table] = newbr.size

		// If the seq changed we record the beginning ot the last processed range
		if device_seq_changed {
			fdb.coverageFullAt[table] = int64(nextPrefix)
		} else {
			// the seq didn't change we must advance until we completed a full scan of the prefixes
			// which happens when a processed range covers our beginning recorded above
			if br.include(fdb.coverageFullAt[table]) {
				fdb.coverageFullAt[table] = hashPrefixCeiling
			}
		}
	}

	return wrap(tx.Commit())
}

// blobRange defines a range for blob searching.
// it is initialized with a chunk size and computes the appropriate end
type blobRange struct {
	start, size int64
}

func (r blobRange) end() int64 {
	stop := r.start + r.size
	if stop >= hashPrefixCeiling {
		return hashPrefixCeiling
	} else {
		return stop
	}
}

func (r blobRange) next(size int64) blobRange {
	start := r.end()
	if start == hashPrefixCeiling {
		start = 0
	}
	return blobRange{start, size}
}

func (r blobRange) include(position int64) bool {
	if (position >= r.start) && (position < r.end()) {
		return true
	} else {
		return false
	}
}

// return the actual size being processed (the last chunk is usually shorter than chunkSize)
func (r blobRange) actualChunkSize() int64 {
	prefixesRemaining := hashPrefixCeiling - r.start
	if (r.size > prefixesRemaining) {
		return prefixesRemaining
	} else {
		return r.size
	}
}

// SQL returns the SQL where clause for the given range, e.g.
// `column >= x'49249248' AND column < x'6db6db6c'`
// AND is postfixed when needed for combination with next condition
func (r blobRange) SQL(name string) string {
	// Full range" no condition (no need for a postfixed AND either)
	if (r.end() == hashPrefixCeiling) && (r.start == 0) {
		return ""
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s >= x'%08X' AND ", name, r.start)
	if r.end() != hashPrefixCeiling {
		fmt.Fprintf(&sb, "%s < x'%08X'", name, r.end())
		sb.WriteString(" AND ")
	}
	return sb.String()
}

// actualChunkSize reflects the case where pagination couldn't fetch the asked chunkSize
// max chunk size is hard coded to 1<<32 which is fine for pagination and needed for the iterations over hash values
// returns 0 if no change is needed
// if actualChunkSize < chunkSize don't speed up
func adaptChunkSize(chunkSize int64, actualChunkSize int64, process_duration time.Duration, l *slog.Logger,
	            ctx context.Context) int64 {
	newChunkSize := int64(0)
	// Did we overshoot the target runtime ?
	if process_duration > gcTargetRuntime {
		newChunkSize = max(chunkSize / 2, gcMinChunkSize)
		l.DebugContext(ctx, "GC too aggressive, slowing down", "new_chunk_size", newChunkSize)
	} else if (process_duration < (gcTargetRuntime / 2)) && (actualChunkSize == chunkSize) &&
		(chunkSize < gcMaxChunkSize) {
		// Increase chunkSize based on the difference between max GC runtime and actual runtime
		// target 3/4 of the max
		// max speedup is 32 which makes allows reaching gcMaxChunkSize in 6 passes
		// 32 = 2 ** 5, gcMinChunkSize = 128 = 2 ** 7
		speedup := min((3 * float64(gcTargetRuntime)) / (4 * float64(process_duration)), 32.0)
		newChunkSize = min(int64(float64(chunkSize) * speedup), gcMaxChunkSize)
		l.DebugContext(ctx, "GC slow, speeding up", "new_chunk_size", newChunkSize)
	}
	return newChunkSize
}
