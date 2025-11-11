// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package sqlite

import (
	"context"
	"fmt"
	"iter"
	"log/slog"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"

	"github.com/jmoiron/sqlx"
	"github.com/syncthing/syncthing/internal/db"
	"github.com/syncthing/syncthing/internal/itererr"
	"github.com/syncthing/syncthing/internal/slogutil"
	"github.com/syncthing/syncthing/lib/protocol"
)

const (
	nameSegmentPrefix = "blocks-"
	maxShardingLevel  = 8
)

type blocksDB struct {
	folderDB          *folderDB
	shardingLevel     int // current sharding level, zero for no sharding
	shardingThreshold int // number of entries in blocks table to trigger next level of sharding
	shards            map[string]*blocksDBShard
}

func openBlocksDB(folderDB *folderDB, shardingThreshold int) (*blocksDB, error) {
	bdb := &blocksDB{
		folderDB:          folderDB,
		shardingThreshold: shardingThreshold,
		shards:            map[string]*blocksDBShard{},
	}

	// Find any existing shard files
	shards, err := filepath.Glob(addInnerExt(folderDB.path, nameSegmentPrefix+"*"))
	if err != nil {
		return nil, wrap(err)
	}

	// Shard names are all ${folderDB}-blocks-${level}${prefix}. The level
	// is the number of bits that are used for the prefix, as one hex digit:
	// 1-f. In practice we currently set the upper limit at 8 bits (256
	// shards). The prefix is the bits in question, as one hex digit (0-f).

	for _, shardName := range shards {
		suffix := getInnerExt(shardName)
		if !strings.HasPrefix(suffix, nameSegmentPrefix) {
			continue
		}
		suffix = strings.TrimPrefix(suffix, nameSegmentPrefix)
		level := int(suffix[0] - '0')
		dbs, err := openBlocksDBShard(shardName, level)
		if err != nil {
			return nil, wrap(err)
		}
		bdb.shards[suffix] = dbs
		bdb.shardingLevel = max(bdb.shardingLevel, level)
		slog.Debug("Found database shard", slogutil.FilePath(filepath.Base(shardName)), slog.String("suffix", suffix), slog.Int("currentLevel", bdb.shardingLevel))
	}

	return bdb, nil
}

func (bdb *blocksDB) insertBlocksLocked(mainTx *sqlx.Tx, blocklistHash []byte, blocks []protocol.BlockInfo) error {
	if len(blocks) == 0 {
		return nil
	}

	// Rework the block list to a slice of maps, which is what we need to
	// pass to the low level SQL bulk insert function. We do this prior to
	// any segmenting etc as we need the original block indexes heres.

	bs := make([]map[string]any, len(blocks))
	for i, b := range blocks {
		bs[i] = map[string]any{
			"hash":           b.Hash,
			"blocklist_hash": blocklistHash,
			"idx":            i,
			"offset":         b.Offset,
			"size":           b.Size,
		}
	}

	if bdb.shardingLevel == 0 {
		// No sharding yet, use main tx as-is
		return insertBlocksLockedInTx(mainTx, bs)
	}

	// Segment into corresponding shards based on the current sharding level

	segs := make(map[string][]map[string]any)
	for _, b := range bs {
		hash := b["hash"].([]byte) //nolint:forcetypeassert
		segName := shardPrefix(bdb.shardingLevel, hash)
		segs[segName] = append(segs[segName], b)
	}

	// Commit each segment to its individual database shard. We do this with
	// concurrency up to the number of CPU cores.

	var wg sync.WaitGroup
	errChan := make(chan error, 1)
	for seg, blocks := range segs {
		shard, ok := bdb.shards[seg]
		if !ok {
			sh, err := openBlocksDBShard(addInnerExt(bdb.folderDB.path, nameSegmentPrefix+seg), bdb.shardingLevel)
			if err != nil {
				return err
			}
			bdb.shards[seg] = sh
			shard = sh
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := shard.insertBlocksLocked(blocks); err != nil {
				select {
				case errChan <- err:
				default:
				}
			}
		}()
	}
	wg.Wait()
	close(errChan)

	return <-errChan
}

func (bdb *blocksDB) allBlocksWithHash(hash []byte) ([]db.BlockMapEntry, error) {
	// Start with the main folder database, which may always contain some blocks

	blocks, err := itererr.Collect(allBlocksWithHashAndNamesInDB(bdb.folderDB.baseDB, hash))
	if err != nil {
		return nil, wrap(err)
	}

	// Then check each existing shard up to the max sharding level. If there
	// isn't one in the map, we haven't created that shard yet and don't
	// need to go looking there.

	for l := range bdb.shardingLevel {
		prefix := shardPrefix(l+1, hash) // l=0 is sharding level one
		if shard, ok := bdb.shards[prefix]; ok {
			tb, err := itererr.Collect(allBlocksWithHashInDB(shard.baseDB, hash))
			if err != nil {
				return nil, wrap(err)
			}
			blocks = append(blocks, tb...)
		}
	}
	return blocks, nil
}

func allBlocksWithHashAndNamesInDB(baseDB *baseDB, hash []byte) (iter.Seq[db.BlockMapEntry], func() error) {
	return iterStructs[db.BlockMapEntry](baseDB.stmt(`
		SELECT b.blocklist_hash as blocklisthash, b.idx as blockindex, b.offset, b.size, n.name as filename FROM files f
		INNER JOIN file_names n ON f.name_idx = n.idx
		LEFT JOIN blocks b ON f.blocklist_hash = b.blocklist_hash
		WHERE f.device_idx = {{.LocalDeviceIdx}} AND b.hash = ?
	`).Queryx(hash))
}

func allBlocksWithHashInDB(baseDB *baseDB, hash []byte) (iter.Seq[db.BlockMapEntry], func() error) {
	return iterStructs[db.BlockMapEntry](baseDB.stmt(`
		SELECT blocklist_hash as blocklisthash, idx as blockindex, offset, size, '' as filename FROM blocks
		WHERE hash = ?
	`).Queryx(hash))
}

func (bdb *blocksDB) allShardsCheckpoint(ctx context.Context, query string) error {
	var wg sync.WaitGroup
	errChan := make(chan error, 1)
	for _, shard := range bdb.shards {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := shard.baseDB.sql.Conn(ctx)
			if err != nil {
				return
			}
			defer conn.Close()
			_, _ = conn.ExecContext(ctx, `PRAGMA journal_size_limit = 8388608`)
			_, _ = conn.ExecContext(ctx, query)
		}()
	}
	wg.Wait()
	close(errChan)

	return <-errChan
}

func (bdb *blocksDB) updateShardingLevel() {
	if bdb.shardingLevel >= maxShardingLevel || 1<<bdb.shardingLevel >= runtime.NumCPU() {
		return
	}

	if bdb.shardingLevel == 0 {
		// If we're at level zero (no sharding) we check if the main folder
		// db is full, and if so increase the sharding level to one.
		if bdb.shouldSplit(bdb.folderDB.baseDB) {
			bdb.shardingLevel++
			slog.Debug("Increasing sharding level from base", "level", bdb.shardingLevel)
		}
		return
	}

	// If we're already doing sharding, check each of the highest-level
	// shards to see if they are full. (If there are intermediate levels
	// those will by definition likely already be full, so we should not
	// look at them.)
	for _, shard := range bdb.shards {
		if shard.level != bdb.shardingLevel {
			continue
		}
		if bdb.shouldSplit(shard.baseDB) {
			bdb.shardingLevel++
			slog.Debug("Increasing sharding level from shard", "level", bdb.shardingLevel)
			break
		}
	}
}

func (bdb *blocksDB) shouldSplit(baseDB *baseDB) bool {
	var blocks int64
	if err := baseDB.sql.QueryRowx(`SELECT count(*) FROM blocks`).Scan(&blocks); err != nil {
		return false
	}

	return blocks > int64(bdb.shardingThreshold)
}

func (bdb *blocksDB) Commit() error {
	// Commit all the shards, return the first error encountered

	var firstErr error
	for _, shard := range bdb.shards {
		if err := shard.commit(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (bdb *blocksDB) Rollback() error {
	// Rollback all the shards, return the first error encountered

	var firstErr error
	for _, shard := range bdb.shards {
		if err := shard.rollback(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

type blocksDBShard struct {
	level  int      // level of this shard (1, 2, etc.)
	tx     *sqlx.Tx // currently open write transaction, or nil
	baseDB *baseDB
}

func openBlocksDBShard(path string, level int) (*blocksDBShard, error) {
	pragmas := []string{
		"journal_mode = WAL",
		"optimize = 0x10002",
		"auto_vacuum = INCREMENTAL",
		fmt.Sprintf("application_id = %d", applicationIDFolder),
	}
	schemas := []string{
		"sql/schema/common/*",
		"sql/schema/blocks/*",
	}
	migrations := []string{
		"sql/migrations/common/*",
		"sql/migrations/blocks/*",
	}

	base, err := openBase(path, maxDBConns, pragmas, schemas, migrations)
	if err != nil {
		return nil, wrap(err, path)
	}
	_ = base

	bdb := &blocksDBShard{
		level:  level,
		baseDB: base,
	}

	return bdb, nil
}

func (dbs *blocksDBShard) insertBlocksLocked(blocks []map[string]any) error {
	if len(blocks) == 0 {
		return nil
	}

	// We are being called within a higher level folderdb transaction, but
	// we need to have one of our own. We do this implicitly and keep the
	// transaction around until Commit() or Rollback() is called. This gives
	// us the same life time as the folderdb transaction.

	if dbs.tx == nil {
		tx, err := dbs.baseDB.sql.BeginTxx(context.Background(), nil)
		if err != nil {
			return err
		}
		dbs.tx = tx
	}

	return insertBlocksLockedInTx(dbs.tx, blocks)
}

func (dbs *blocksDBShard) commit() error {
	if dbs.tx == nil {
		return nil
	}
	tx := dbs.tx
	dbs.tx = nil
	return wrap(tx.Commit())
}

func (dbs *blocksDBShard) rollback() error {
	if dbs.tx == nil {
		return nil
	}
	tx := dbs.tx
	dbs.tx = nil
	return wrap(tx.Rollback())
}

func shardPrefix(level int, hash []byte) string {
	mask := uint8(^(0xff >> level))
	value := hash[0] & mask
	return fmt.Sprintf("%x%02x", level, value)
}

func insertBlocksLockedInTx(tx *sqlx.Tx, blocks []map[string]any) error {
	// Very large block lists (>8000 blocks) result in "too many variables"
	// error. Chunk it to a reasonable size.
	for chunk := range slices.Chunk(blocks, 1000) {
		if _, err := tx.NamedExec(`
			INSERT OR IGNORE INTO blocks (hash, blocklist_hash, idx, offset, size)
			VALUES (:hash, :blocklist_hash, :idx, :offset, :size)
		`, chunk); err != nil {
			return wrap(err)
		}
	}

	return nil
}
