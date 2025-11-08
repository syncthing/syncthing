// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package sqlite

import (
	"context"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"slices"
	"sync"

	"github.com/jmoiron/sqlx"
	"github.com/syncthing/syncthing/lib/protocol"
)

// At ten million blocks we start sharding the block database
const splitCutoff = 10_000_000

type blocksDB struct {
	pathBase   string
	shards     map[string]*blocksDBShard
	splitlevel int
}

type blocksDBShard struct {
	level int
	split bool
	base  *baseDB
	tx    *sqlx.Tx
}

func openBlocksDB(path string) (*blocksDB, error) {
	bdb := &blocksDB{
		pathBase: path,
		shards:   map[string]*blocksDBShard{},
	}
	shards, err := filepath.Glob(preExtSuffix(bdb.pathBase, "*"))
	if err != nil {
		return nil, wrap(err)
	}
	for _, shardName := range shards {
		suffix := getExtSuffix(shardName)
		if suffix == "" {
			continue
		}
		dbs, err := openBlocksDBShard(shardName, len(suffix))
		if err != nil {
			return nil, wrap(err)
		}
		bdb.shards[suffix] = dbs
		bdb.splitlevel = max(bdb.splitlevel, len(suffix))
	}
	for name := range bdb.shards {
		if name == "" {
			continue
		}
		upperName := name[:len(name)-1]
		bdb.shards[upperName].split = true
	}
	return bdb, nil
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

	bdb := &blocksDBShard{level: level, base: base}

	return bdb, nil
}

func (bdb *blocksDB) insertBlocksLocked(mainTx *sqlx.Tx, blocklistHash []byte, blocks []protocol.BlockInfo) error {
	if len(blocks) == 0 {
		return nil
	}
	if bdb.splitlevel == 0 {
		// No splitting yet, use main tx
		return insertBlocksLockedTx(mainTx, blocklistHash, blocks)
	}

	// Segment into corresponding sharts
	segs := make(map[string][]protocol.BlockInfo)
	for _, b := range blocks {
		segName := hex.EncodeToString(b.Hash[:bdb.splitlevel/2+1])[:bdb.splitlevel]
		segs[segName] = append(segs[segName], b)
	}

	var wg sync.WaitGroup
	errChan := make(chan error, 1)
	concurrency := make(chan struct{}, 16)
	for seg, blocks := range segs {
		shard, ok := bdb.shards[seg]
		if !ok {
			sh, err := openBlocksDBShard(preExtSuffix(bdb.pathBase, seg), len(seg))
			if err != nil {
				return err
			}
			bdb.shards[seg] = sh
			shard = sh
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			concurrency <- struct{}{}
			defer func() { <-concurrency }()
			if err := shard.insertBlocksLocked(blocklistHash, blocks); err != nil {
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

func (bdb *blocksDB) allShardsCheckpoint(ctx context.Context, query string) error {
	var wg sync.WaitGroup
	errChan := make(chan error, 1)
	concurrency := make(chan struct{}, 16)
	for _, shard := range bdb.shards {
		wg.Add(1)
		go func() {
			defer wg.Done()
			concurrency <- struct{}{}
			defer func() { <-concurrency }()

			conn, err := shard.base.sql.Conn(ctx)
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

func (bdb *blocksDB) checkSplitLevel(mainTx *sqlx.Tx) {
	if bdb.splitlevel == 0 {
		if shouldSplit(mainTx) {
			bdb.splitlevel++
		}
		return
	}

	for _, shard := range bdb.shards {
		if shard.tx == nil {
			continue
		}
		if shouldSplit(shard.tx) {
			shard.split = true
			bdb.splitlevel = max(bdb.splitlevel, shard.level+1)
		}
	}
}

func (bdb *blocksDB) Commit() error {
	var firstErr error
	for _, shard := range bdb.shards {
		if err := shard.commit(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (bdb *blocksDB) Rollback() error {
	var firstErr error
	for _, shard := range bdb.shards {
		if err := shard.rollback(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (dbs *blocksDBShard) insertBlocksLocked(blocklistHash []byte, blocks []protocol.BlockInfo) error {
	if len(blocks) == 0 {
		return nil
	}

	if dbs.tx == nil {
		tx, err := dbs.base.sql.BeginTxx(context.Background(), nil)
		if err != nil {
			return err
		}
		dbs.tx = tx
	}

	return insertBlocksLockedTx(dbs.tx, blocklistHash, blocks)
}

func insertBlocksLockedTx(tx *sqlx.Tx, blocklistHash []byte, blocks []protocol.BlockInfo) error {
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

	// Very large block lists (>8000 blocks) result in "too many variables"
	// error. Chunk it to a reasonable size.
	for chunk := range slices.Chunk(bs, 1000) {
		if _, err := tx.NamedExec(`
			INSERT OR IGNORE INTO blocks (hash, blocklist_hash, idx, offset, size)
			VALUES (:hash, :blocklist_hash, :idx, :offset, :size)
		`, chunk); err != nil {
			return wrap(err)
		}
	}

	return nil
}

func (dbs *blocksDBShard) commit() error {
	if dbs.tx == nil {
		return nil
	}
	tx := dbs.tx
	dbs.tx = nil
	return wrap(tx.Commit())
}

func shouldSplit(tx *sqlx.Tx) bool {
	var blocks int64
	if err := tx.QueryRowx(`SELECT count(*) FROM blocks`).Scan(&blocks); err != nil {
		return false
	}

	return blocks > splitCutoff
}

func (dbs *blocksDBShard) rollback() error {
	if dbs.tx == nil {
		return nil
	}
	tx := dbs.tx
	dbs.tx = nil
	return wrap(tx.Rollback())
}
