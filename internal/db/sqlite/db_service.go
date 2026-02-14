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
	gcMaxChunkSize  = hashPrefixCeiling
	gcTargetRuntime = 250 * time.Millisecond // max time to spend on gc, per table, per run
	vacuumPages     = 256 // pages are 4k with current SQLite this is 1M worth vaccumed
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
	// Start running periodic maintenance shortly after start
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

		if err != nil {
			wait := time.Until(s.nextFolderMaintenance())
			timer.Reset(wait)
			slog.WarnContext(ctx, "Periodic run failed", "err", err)
			return wrap(err)
		}

		if s.maintenanceInterval != 0 {
			wait := time.Until(s.nextFolderMaintenance())
			timer.Reset(wait)
			slog.DebugContext(ctx, "Next periodic run due", "after", wait)
		}
		time.Sleep(5 * time.Second)
	}
}

func (s *Service) periodic(ctx context.Context) error {

	// We reuse the periodic maintenance of folders which is done at short intervals to trigger
	// the main DB maintenance at long intervals (alternatively it could use its own Timer)
	if s.nextMainDBMaintenance.Before(time.Now()) {
		t0 := time.Now()
		defer func() { slog.DebugContext(ctx, "Main DB maintenance done", "duration", time.Since(t0)) }()
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
				fdb.cleanupIsUrgent(s)
			} else {
				fdb.cleanupAtNormalRate(s)
			}
			if work_done { slog.DebugContext(ctx, "Cleanups", "runtime", time.Since(t0)) }
		}()

		// Get the current device sequence, for comparison in the next step.
		seq, err := fdb.GetDeviceSequence(protocol.LocalDeviceID)
		if err != nil { return wrap(err) }
		// Get the last successful GC sequence. If it's the same as the
		// current sequence, nothing has changed and we can skip the GC
		// once all table rows have been processed
		meta := db.NewTyped(fdb, internalMetaPrefix)
		if prev, _, err := meta.Int64(lastSuccessfulGCSeqKey); err != nil {
			return wrap(err)
		} else if db_update_detected = (seq != prev); !db_update_detected {
			// No change in DB, but incremental cleanups might have to finish their slow walk
			if !fdb.cleanupsCaughtUp() {
				work_done = true
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

			work_done = true
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
	earliestMaintenance := time.Now().Add(s.maintenanceInterval)
	s.sdb.forEachFolder(func(fdb *folderDB) error {
		nextCleanup := fdb.nextCleanup
		if nextCleanup.Before(earliestMaintenance) {
			earliestMaintenance = nextCleanup
		}
		return nil
	})
	return earliestMaintenance
}

func (fdb *folderDB) cleanupAtNormalRate(s *Service) {
	fdb.nextCleanup = time.Now().Add(s.maintenanceInterval)
}
func (fdb *folderDB) cleanupIsUrgent(s *Service) {
	fdb.nextCleanup = time.Now().Add(s.maintenanceInterval/10)
}
func (fdb *folderDB) cleanupsCaughtUp() bool {
	return fdb.hashCleanupCaughtUp() && fdb.namesCleanupCaughtUp() && fdb.versionsCleanupCaughtUp()
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
	defer func() { slog.DebugContext(ctx, "tidy", "database", name, "runtime", time.Since(t0),
		                        "truncate", do_truncate_checkpoint) }()

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
	chunkStart := fdb.cursorValues[table]
	chunkSize := fdb.chunkSizes[table]
	partialChunk := false
	intChunkEnd := int64(0) // will be set if chunkEnd.Valid

	l := slog.With("folder", fdb.folderID, "fdb", fdb.baseName, "table", table, "start", chunkStart, "chunk_size", chunkSize)

	t0 := time.Now()
	t1 := time.Now()
	defer func() {
		l.DebugContext(ctx, "GC runtime for " + table, "Total", time.Since(t0), "Chunk limits fetch", t1.Sub(t0),
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
			l.WarnContext(ctx, table + "MAX(idx) failed", "error", err)
			return wrap(err, table + " DELETE")
		}
		partialChunk = true
	}
	l.DebugContext(ctx, table + " chunk", "last idx", chunkEnd, "full chunk", !partialChunk)

	if chunkEnd.Valid {
		intChunkEnd = chunkEnd.Int64
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
			l.WarnContext(ctx, table + "delete failed", "error", err)
			return wrap(err, table + " DELETE")
		}
		if aff, err := res.RowsAffected(); err == nil {
			l.DebugContext(ctx, table + " DELETE", "affected", aff)
		}

		// Pass information about unknown chunk size (0) when needed
		var actualChunkSize int
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
		l.DebugContext(ctx, "files: removed old deleted records", "affected", aff)
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
		nextPrefix := int(fdb.cursorValues[table])
		chunkSize := fdb.chunkSizes[table]

		// General case
		l := slog.With("folder/table", fdb.folderID + "/" + table, "prefix", nextPrefix, "chunksize", chunkSize,
			"fdb", fdb.baseName)
		// Shorter log when doing a full scan
		if (nextPrefix == 0) && (chunkSize == hashPrefixCeiling) {
			l = slog.With("FULLscan on folder/table", fdb.folderID + "/" + table, "fdb", fdb.baseName)
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
		fdb.cursorValues[table] = int64(newbr.start)
		fdb.chunkSizes[table] = newbr.size

		// If the seq changed we record the beginning ot the last processed range
		if device_seq_changed {
			fdb.coverageFullAt[table] = int64(nextPrefix)
		} else {
			// the seq didn't change we must advance until we completed a full scan of the prefixes
			// which happens when a processed range covers our beginning recorded above
			if br.include(int(fdb.coverageFullAt[table])) {
				fdb.coverageFullAt[table] = hashPrefixCeiling
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
	if stop >= hashPrefixCeiling {
		return hashPrefixCeiling
	} else {
		return stop
	}
}

func (r blobRange) next(size int) blobRange {
	start := r.end()
	if start == hashPrefixCeiling {
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
func adaptChunkSize(chunkSize int, actualChunkSize int, process_duration time.Duration, l *slog.Logger,
	            ctx context.Context) int {
	newChunkSize := 0
	// Did we overshoot the target runtime ?
	if process_duration > gcTargetRuntime {
		newChunkSize = max(chunkSize / 2, gcMinChunkSize)
		l.DebugContext(ctx, "GC too aggressive, reducing speed", "new_chunk_size", newChunkSize)
	} else if (process_duration < (gcTargetRuntime / 2)) && (actualChunkSize == chunkSize) &&
		(chunkSize < gcMaxChunkSize) {
		// Increase chunkSize based on the difference between max GC runtime and actual runtime
		// target 3/4 of the max
		// max speedup is 32 which makes allows reaching gcMaxChunkSize in 6 passes
		// 32 = 2 ** 5, gcMinChunkSize = 128 = 2 ** 7
		speedup := min((3 * float64(gcTargetRuntime)) / (4 * float64(process_duration)), 32.0)
		newChunkSize = min(int(float64(chunkSize) * speedup), gcMaxChunkSize)
		l.DebugContext(ctx, "GC slow, increasing speed", "new_chunk_size", newChunkSize)
	}
	return newChunkSize
}
