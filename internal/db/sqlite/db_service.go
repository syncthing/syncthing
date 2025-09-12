// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package sqlite

import (
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"math/rand"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/syncthing/syncthing/internal/db"
	"github.com/syncthing/syncthing/internal/slogutil"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/thejerf/suture/v4"
)

const (
	internalMetaPrefix     = "dbsvc"
	lastMaintKey           = "lastMaint"
	lastSuccessfulGCSeqKey = "lastSuccessfulGCSeq"

	gcMinChunks  = 5
	gcChunkSize  = 100_000         // approximate number of rows to process in a single gc query
	gcMaxRuntime = 5 * time.Minute // max time to spend on gc, per table, per run
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
			slog.DebugContext(ctx, "Skipping unnecessary GC", "folder", fdb.folderID, "fdb", fdb.baseName)
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
			if err := garbageCollectBlocklistsAndBlocksLocked(ctx, fdb); err != nil {
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

	// Both blocklists and blocks refer to blocklists_hash from the files table.
	for _, table := range []string{"blocklists", "blocks"} {
		// Count the number of rows
		var rows int64
		if err := tx.GetContext(ctx, &rows, `SELECT count(*) FROM `+table); err != nil {
			return wrap(err)
		}

		chunks := max(gcMinChunks, rows/gcChunkSize)
		l := slog.With("folder", fdb.folderID, "fdb", fdb.baseName, "table", table, "rows", rows, "chunks", chunks)

		// Process rows in chunks up to a given time limit. We always use at
		// least gcMinChunks chunks, then increase the number as the number of rows
		// exceeds gcMinChunks*gcChunkSize.
		t0 := time.Now()
		for i, br := range randomBlobRanges(int(chunks)) {
			if d := time.Since(t0); d > gcMaxRuntime {
				l.InfoContext(ctx, "GC was interrupted due to exceeding time limit", "processed", i, "runtime", time.Since(t0))
				break
			}

			// The limit column must be an indexed column with a mostly random distribution of blobs.
			// That's the blocklist_hash column for blocklists, and the hash column for blocks.
			limitColumn := table + ".blocklist_hash"
			if table == "blocks" {
				limitColumn = "blocks.hash"
			}

			q := fmt.Sprintf(`
				DELETE FROM %s
				WHERE %s AND NOT EXISTS (
					SELECT 1 FROM files WHERE files.blocklist_hash = %s.blocklist_hash
				)`, table, br.SQL(limitColumn), table)

			if res, err := tx.ExecContext(ctx, q); err != nil {
				return wrap(err, "delete from "+table)
			} else {
				l.DebugContext(ctx, "GC query result", "processed", i, "runtime", time.Since(t0), "result", slogutil.Expensive(func() any {
					rows, err := res.RowsAffected()
					if err != nil {
						return slogutil.Error(err)
					}
					return slog.Int64("rows", rows)
				}))
			}
		}
	}

	return wrap(tx.Commit())
}

// blobRange defines a range for blob searching. A range is open ended if
// start or end is nil.
type blobRange struct {
	start, end []byte
}

// SQL returns the SQL where clause for the given range, e.g.
// `column >= x'49249248' AND column < x'6db6db6c'`
func (r blobRange) SQL(name string) string {
	var sb strings.Builder
	if r.start != nil {
		fmt.Fprintf(&sb, "%s >= x'%x'", name, r.start)
	}
	if r.start != nil && r.end != nil {
		sb.WriteString(" AND ")
	}
	if r.end != nil {
		fmt.Fprintf(&sb, "%s < x'%x'", name, r.end)
	}
	return sb.String()
}

// randomBlobRanges returns n blobRanges in random order
func randomBlobRanges(n int) []blobRange {
	ranges := blobRanges(n)
	rand.Shuffle(len(ranges), func(i, j int) { ranges[i], ranges[j] = ranges[j], ranges[i] })
	return ranges
}

// blobRanges returns n blobRanges
func blobRanges(n int) []blobRange {
	// We use three byte (24 bit) prefixes to get fairly granular ranges and easy bit
	// conversions.
	rangeSize := (1 << 24) / n
	ranges := make([]blobRange, 0, n)
	var prev []byte
	for i := range n {
		var pref []byte
		if i < n-1 {
			end := (i + 1) * rangeSize
			pref = intToBlob(end)
		}
		ranges = append(ranges, blobRange{prev, pref})
		prev = pref
	}
	return ranges
}

func intToBlob(n int) []byte {
	var pref [4]byte
	binary.BigEndian.PutUint32(pref[:], uint32(n)) //nolint:gosec
	// first byte is always zero and not part of the range
	return pref[1:]
}
