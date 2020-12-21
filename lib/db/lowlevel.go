// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package db

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"regexp"
	"time"

	"github.com/dchest/siphash"
	"github.com/greatroar/blobloom"
	"github.com/syncthing/syncthing/lib/db/backend"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/rand"
	"github.com/syncthing/syncthing/lib/sha256"
	"github.com/syncthing/syncthing/lib/svcutil"
	"github.com/syncthing/syncthing/lib/sync"
	"github.com/syncthing/syncthing/lib/util"
	"github.com/thejerf/suture/v4"
)

const (
	// We set the bloom filter capacity to handle 100k individual items with
	// a false positive probability of 1% for the first pass. Once we know
	// how many items we have we will use that number instead, if it's more
	// than 100k. For fewer than 100k items we will just get better false
	// positive rate instead.
	indirectGCBloomCapacity          = 100000
	indirectGCBloomFalsePositiveRate = 0.01     // 1%
	indirectGCBloomMaxBytes          = 32 << 20 // Use at most 32MiB memory, which covers our desired FP rate at 27 M items
	indirectGCDefaultInterval        = 13 * time.Hour
	indirectGCTimeKey                = "lastIndirectGCTime"

	// Use indirection for the block list when it exceeds this many entries
	blocksIndirectionCutoff = 3
	// Use indirection for the version vector when it exceeds this many entries
	versionIndirectionCutoff = 10

	recheckDefaultInterval = 30 * 24 * time.Hour

	needsRepairSuffix = ".needsrepair"
)

// Lowlevel is the lowest level database interface. It has a very simple
// purpose: hold the actual backend database, and the in-memory state
// that belong to that database. In the same way that a single on disk
// database can only be opened once, there should be only one Lowlevel for
// any given backend.
type Lowlevel struct {
	*suture.Supervisor
	backend.Backend
	folderIdx          *smallIndex
	deviceIdx          *smallIndex
	keyer              keyer
	gcMut              sync.RWMutex
	gcKeyCount         int
	indirectGCInterval time.Duration
	recheckInterval    time.Duration
	oneFileSetCreated  chan struct{}
}

func NewLowlevel(backend backend.Backend, opts ...Option) *Lowlevel {
	// Only log restarts in debug mode.
	spec := svcutil.SpecWithDebugLogger(l)
	db := &Lowlevel{
		Supervisor:         suture.New("db.Lowlevel", spec),
		Backend:            backend,
		folderIdx:          newSmallIndex(backend, []byte{KeyTypeFolderIdx}),
		deviceIdx:          newSmallIndex(backend, []byte{KeyTypeDeviceIdx}),
		gcMut:              sync.NewRWMutex(),
		indirectGCInterval: indirectGCDefaultInterval,
		recheckInterval:    recheckDefaultInterval,
		oneFileSetCreated:  make(chan struct{}),
	}
	for _, opt := range opts {
		opt(db)
	}
	db.keyer = newDefaultKeyer(db.folderIdx, db.deviceIdx)
	db.Add(svcutil.AsService(db.gcRunner, "db.Lowlevel/gcRunner"))
	if path := db.needsRepairPath(); path != "" {
		if _, err := os.Lstat(path); err == nil {
			l.Infoln("Database was marked for repair - this may take a while")
			db.checkRepair()
			os.Remove(path)
		}
	}
	return db
}

type Option func(*Lowlevel)

// WithRecheckInterval sets the time interval in between metadata recalculations
// and consistency checks.
func WithRecheckInterval(dur time.Duration) Option {
	return func(db *Lowlevel) {
		if dur > 0 {
			db.recheckInterval = dur
		}
	}
}

// WithIndirectGCInterval sets the time interval in between GC runs.
func WithIndirectGCInterval(dur time.Duration) Option {
	return func(db *Lowlevel) {
		if dur > 0 {
			db.indirectGCInterval = dur
		}
	}
}

// ListFolders returns the list of folders currently in the database
func (db *Lowlevel) ListFolders() []string {
	return db.folderIdx.Values()
}

// updateRemoteFiles adds a list of fileinfos to the database and updates the
// global versionlist and metadata.
func (db *Lowlevel) updateRemoteFiles(folder, device []byte, fs []protocol.FileInfo, meta *metadataTracker) error {
	db.gcMut.RLock()
	defer db.gcMut.RUnlock()

	t, err := db.newReadWriteTransaction(meta.CommitHook(folder))
	if err != nil {
		return err
	}
	defer t.close()

	var dk, gk, keyBuf []byte
	devID, err := protocol.DeviceIDFromBytes(device)
	if err != nil {
		return err
	}
	for _, f := range fs {
		name := []byte(f.Name)
		dk, err = db.keyer.GenerateDeviceFileKey(dk, folder, device, name)
		if err != nil {
			return err
		}

		ef, ok, err := t.getFileTrunc(dk, true)
		if err != nil {
			return err
		}
		if ok && unchanged(f, ef) {
			l.Debugf("not inserting unchanged (remote); folder=%q device=%v %v", folder, devID, f)
			continue
		}

		if ok {
			meta.removeFile(devID, ef)
		}
		meta.addFile(devID, f)

		l.Debugf("insert (remote); folder=%q device=%v %v", folder, devID, f)
		if err := t.putFile(dk, f); err != nil {
			return err
		}

		gk, err = db.keyer.GenerateGlobalVersionKey(gk, folder, name)
		if err != nil {
			return err
		}
		keyBuf, _, err = t.updateGlobal(gk, keyBuf, folder, device, f, meta)
		if err != nil {
			return err
		}

		if err := t.Checkpoint(); err != nil {
			return err
		}
	}

	return t.Commit()
}

// updateLocalFiles adds fileinfos to the db, and updates the global versionlist,
// metadata, sequence and blockmap buckets.
func (db *Lowlevel) updateLocalFiles(folder []byte, fs []protocol.FileInfo, meta *metadataTracker) error {
	db.gcMut.RLock()
	defer db.gcMut.RUnlock()

	t, err := db.newReadWriteTransaction(meta.CommitHook(folder))
	if err != nil {
		return err
	}
	defer t.close()

	var dk, gk, keyBuf []byte
	blockBuf := make([]byte, 4)
	for _, f := range fs {
		name := []byte(f.Name)
		dk, err = db.keyer.GenerateDeviceFileKey(dk, folder, protocol.LocalDeviceID[:], name)
		if err != nil {
			return err
		}

		ef, ok, err := t.getFileByKey(dk)
		if err != nil {
			return err
		}
		if ok && unchanged(f, ef) {
			l.Debugf("not inserting unchanged (local); folder=%q %v", folder, f)
			continue
		}
		blocksHashSame := ok && bytes.Equal(ef.BlocksHash, f.BlocksHash)

		if ok {
			if len(ef.Blocks) != 0 && !ef.IsInvalid() && ef.Size > 0 {
				for _, block := range ef.Blocks {
					keyBuf, err = db.keyer.GenerateBlockMapKey(keyBuf, folder, block.Hash, name)
					if err != nil {
						return err
					}
					if err := t.Delete(keyBuf); err != nil {
						return err
					}
				}
				if !blocksHashSame {
					keyBuf, err := db.keyer.GenerateBlockListMapKey(keyBuf, folder, ef.BlocksHash, name)
					if err != nil {
						return err
					}
					if err = t.Delete(keyBuf); err != nil {
						return err
					}
				}
			}

			keyBuf, err = db.keyer.GenerateSequenceKey(keyBuf, folder, ef.SequenceNo())
			if err != nil {
				return err
			}
			if err := t.Delete(keyBuf); err != nil {
				return err
			}
			l.Debugf("removing sequence; folder=%q sequence=%v %v", folder, ef.SequenceNo(), ef.FileName())
		}

		f.Sequence = meta.nextLocalSeq()

		if ok {
			meta.removeFile(protocol.LocalDeviceID, ef)
		}
		meta.addFile(protocol.LocalDeviceID, f)

		l.Debugf("insert (local); folder=%q %v", folder, f)
		if err := t.putFile(dk, f); err != nil {
			return err
		}

		gk, err = db.keyer.GenerateGlobalVersionKey(gk, folder, []byte(f.Name))
		if err != nil {
			return err
		}
		keyBuf, _, err = t.updateGlobal(gk, keyBuf, folder, protocol.LocalDeviceID[:], f, meta)
		if err != nil {
			return err
		}

		keyBuf, err = db.keyer.GenerateSequenceKey(keyBuf, folder, f.Sequence)
		if err != nil {
			return err
		}
		if err := t.Put(keyBuf, dk); err != nil {
			return err
		}
		l.Debugf("adding sequence; folder=%q sequence=%v %v", folder, f.Sequence, f.Name)

		if len(f.Blocks) != 0 && !f.IsInvalid() && f.Size > 0 {
			for i, block := range f.Blocks {
				binary.BigEndian.PutUint32(blockBuf, uint32(i))
				keyBuf, err = db.keyer.GenerateBlockMapKey(keyBuf, folder, block.Hash, name)
				if err != nil {
					return err
				}
				if err := t.Put(keyBuf, blockBuf); err != nil {
					return err
				}
			}
			if !blocksHashSame {
				keyBuf, err := db.keyer.GenerateBlockListMapKey(keyBuf, folder, f.BlocksHash, name)
				if err != nil {
					return err
				}
				if err = t.Put(keyBuf, nil); err != nil {
					return err
				}
			}
		}

		if err := t.Checkpoint(); err != nil {
			return err
		}
	}

	return t.Commit()
}

func (db *Lowlevel) dropFolder(folder []byte) error {
	db.gcMut.RLock()
	defer db.gcMut.RUnlock()

	t, err := db.newReadWriteTransaction()
	if err != nil {
		return err
	}
	defer t.close()

	// Remove all items related to the given folder from the device->file bucket
	k0, err := db.keyer.GenerateDeviceFileKey(nil, folder, nil, nil)
	if err != nil {
		return err
	}
	if err := t.deleteKeyPrefix(k0.WithoutNameAndDevice()); err != nil {
		return err
	}

	// Remove all sequences related to the folder
	k1, err := db.keyer.GenerateSequenceKey(k0, folder, 0)
	if err != nil {
		return err
	}
	if err := t.deleteKeyPrefix(k1.WithoutSequence()); err != nil {
		return err
	}

	// Remove all items related to the given folder from the global bucket
	k2, err := db.keyer.GenerateGlobalVersionKey(k1, folder, nil)
	if err != nil {
		return err
	}
	if err := t.deleteKeyPrefix(k2.WithoutName()); err != nil {
		return err
	}

	// Remove all needs related to the folder
	k3, err := db.keyer.GenerateNeedFileKey(k2, folder, nil)
	if err != nil {
		return err
	}
	if err := t.deleteKeyPrefix(k3.WithoutName()); err != nil {
		return err
	}

	// Remove the blockmap of the folder
	k4, err := db.keyer.GenerateBlockMapKey(k3, folder, nil, nil)
	if err != nil {
		return err
	}
	if err := t.deleteKeyPrefix(k4.WithoutHashAndName()); err != nil {
		return err
	}

	k5, err := db.keyer.GenerateBlockListMapKey(k4, folder, nil, nil)
	if err != nil {
		return err
	}
	if err := t.deleteKeyPrefix(k5.WithoutHashAndName()); err != nil {
		return err
	}

	return t.Commit()
}

func (db *Lowlevel) dropDeviceFolder(device, folder []byte, meta *metadataTracker) error {
	db.gcMut.RLock()
	defer db.gcMut.RUnlock()

	t, err := db.newReadWriteTransaction(meta.CommitHook(folder))
	if err != nil {
		return err
	}
	defer t.close()

	key, err := db.keyer.GenerateDeviceFileKey(nil, folder, device, nil)
	if err != nil {
		return err
	}
	dbi, err := t.NewPrefixIterator(key)
	if err != nil {
		return err
	}
	defer dbi.Release()

	var gk, keyBuf []byte
	for dbi.Next() {
		name := db.keyer.NameFromDeviceFileKey(dbi.Key())
		gk, err = db.keyer.GenerateGlobalVersionKey(gk, folder, name)
		if err != nil {
			return err
		}
		keyBuf, err = t.removeFromGlobal(gk, keyBuf, folder, device, name, meta)
		if err != nil {
			return err
		}
		if err := t.Delete(dbi.Key()); err != nil {
			return err
		}
		if err := t.Checkpoint(); err != nil {
			return err
		}
	}
	dbi.Release()
	if err := dbi.Error(); err != nil {
		return err
	}

	if bytes.Equal(device, protocol.LocalDeviceID[:]) {
		key, err := db.keyer.GenerateBlockMapKey(nil, folder, nil, nil)
		if err != nil {
			return err
		}
		if err := t.deleteKeyPrefix(key.WithoutHashAndName()); err != nil {
			return err
		}
		key2, err := db.keyer.GenerateBlockListMapKey(key, folder, nil, nil)
		if err != nil {
			return err
		}
		if err := t.deleteKeyPrefix(key2.WithoutHashAndName()); err != nil {
			return err
		}
	}
	return t.Commit()
}

func (db *Lowlevel) checkGlobals(folder []byte) error {
	t, err := db.newReadWriteTransaction()
	if err != nil {
		return err
	}
	defer t.close()

	key, err := db.keyer.GenerateGlobalVersionKey(nil, folder, nil)
	if err != nil {
		return err
	}
	dbi, err := t.NewPrefixIterator(key.WithoutName())
	if err != nil {
		return err
	}
	defer dbi.Release()

	var dk []byte
	ro := t.readOnlyTransaction
	for dbi.Next() {
		var vl VersionList
		if err := vl.Unmarshal(dbi.Value()); err != nil || vl.Empty() {
			if err := t.Delete(dbi.Key()); err != nil && !backend.IsNotFound(err) {
				return err
			}
			continue
		}

		// Check the global version list for consistency. An issue in previous
		// versions of goleveldb could result in reordered writes so that
		// there are global entries pointing to no longer existing files. Here
		// we find those and clear them out.

		name := db.keyer.NameFromGlobalVersionKey(dbi.Key())
		newVL := &VersionList{}
		var changed, changedHere bool
		for _, fv := range vl.RawVersions {
			changedHere, err = checkGlobalsFilterDevices(dk, folder, name, fv.Devices, newVL, ro)
			if err != nil {
				return err
			}
			changed = changed || changedHere

			changedHere, err = checkGlobalsFilterDevices(dk, folder, name, fv.InvalidDevices, newVL, ro)
			if err != nil {
				return err
			}
			changed = changed || changedHere
		}

		if newVL.Empty() {
			if err := t.Delete(dbi.Key()); err != nil && !backend.IsNotFound(err) {
				return err
			}
		} else if changed {
			if err := t.Put(dbi.Key(), mustMarshal(newVL)); err != nil {
				return err
			}
		}
	}
	dbi.Release()
	if err := dbi.Error(); err != nil {
		return err
	}

	l.Debugf("db check completed for %q", folder)
	return t.Commit()
}

func checkGlobalsFilterDevices(dk, folder, name []byte, devices [][]byte, vl *VersionList, t readOnlyTransaction) (bool, error) {
	var changed bool
	var err error
	for _, device := range devices {
		dk, err = t.keyer.GenerateDeviceFileKey(dk, folder, device, name)
		if err != nil {
			return false, err
		}
		f, ok, err := t.getFileTrunc(dk, false)
		if err != nil {
			return false, err
		}
		if !ok {
			changed = true
			continue
		}
		_, _, _, _, _, _, err = vl.update(folder, device, f, t)
		if err != nil {
			return false, err
		}
	}
	return changed, nil
}

func (db *Lowlevel) getIndexID(device, folder []byte) (protocol.IndexID, error) {
	key, err := db.keyer.GenerateIndexIDKey(nil, device, folder)
	if err != nil {
		return 0, err
	}
	cur, err := db.Get(key)
	if backend.IsNotFound(err) {
		return 0, nil
	} else if err != nil {
		return 0, err
	}

	var id protocol.IndexID
	if err := id.Unmarshal(cur); err != nil {
		return 0, nil
	}

	return id, nil
}

func (db *Lowlevel) setIndexID(device, folder []byte, id protocol.IndexID) error {
	bs, _ := id.Marshal() // marshalling can't fail
	key, err := db.keyer.GenerateIndexIDKey(nil, device, folder)
	if err != nil {
		return err
	}
	return db.Put(key, bs)
}

func (db *Lowlevel) dropFolderIndexIDs(folder []byte) error {
	t, err := db.newReadWriteTransaction()
	if err != nil {
		return err
	}
	defer t.close()

	if err := t.deleteKeyPrefixMatching([]byte{KeyTypeIndexID}, func(key []byte) bool {
		keyFolder, ok := t.keyer.FolderFromIndexIDKey(key)
		if !ok {
			l.Debugf("Deleting IndexID with missing FolderIdx: %v", key)
			return true
		}
		return bytes.Equal(keyFolder, folder)
	}); err != nil {
		return err
	}
	return t.Commit()
}

func (db *Lowlevel) dropMtimes(folder []byte) error {
	key, err := db.keyer.GenerateMtimesKey(nil, folder)
	if err != nil {
		return err
	}
	return db.dropPrefix(key)
}

func (db *Lowlevel) dropFolderMeta(folder []byte) error {
	key, err := db.keyer.GenerateFolderMetaKey(nil, folder)
	if err != nil {
		return err
	}
	return db.dropPrefix(key)
}

func (db *Lowlevel) dropPrefix(prefix []byte) error {
	t, err := db.newReadWriteTransaction()
	if err != nil {
		return err
	}
	defer t.close()

	if err := t.deleteKeyPrefix(prefix); err != nil {
		return err
	}
	return t.Commit()
}

func (db *Lowlevel) gcRunner(ctx context.Context) error {
	// Calculate the time for the next GC run. Even if we should run GC
	// directly, give the system a while to get up and running and do other
	// stuff first. (We might have migrations and stuff which would be
	// better off running before GC.)
	next := db.timeUntil(indirectGCTimeKey, db.indirectGCInterval)
	if next < time.Minute {
		next = time.Minute
	}

	t := time.NewTimer(next)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			if err := db.gcIndirect(ctx); err != nil {
				l.Warnln("Database indirection GC failed:", err)
			}
			db.recordTime(indirectGCTimeKey)
			t.Reset(db.timeUntil(indirectGCTimeKey, db.indirectGCInterval))
		}
	}
}

// recordTime records the current time under the given key, affecting the
// next call to timeUntil with the same key.
func (db *Lowlevel) recordTime(key string) {
	miscDB := NewMiscDataNamespace(db)
	_ = miscDB.PutInt64(key, time.Now().Unix()) // error wilfully ignored
}

// timeUntil returns how long we should wait until the next interval, or
// zero if it should happen directly.
func (db *Lowlevel) timeUntil(key string, every time.Duration) time.Duration {
	miscDB := NewMiscDataNamespace(db)
	lastTime, _, _ := miscDB.Int64(key) // error wilfully ignored
	nextTime := time.Unix(lastTime, 0).Add(every)
	sleepTime := time.Until(nextTime)
	if sleepTime < 0 {
		sleepTime = 0
	}
	return sleepTime
}

func (db *Lowlevel) gcIndirect(ctx context.Context) error {
	// The indirection GC uses bloom filters to track used block lists and
	// versions. This means iterating over all items, adding their hashes to
	// the filter, then iterating over the indirected items and removing
	// those that don't match the filter. The filter will give false
	// positives so we will keep around one percent of things that we don't
	// really need (at most).
	//
	// Indirection GC needs to run when there are no modifications to the
	// FileInfos or indirected items.

	db.gcMut.Lock()
	defer db.gcMut.Unlock()

	t, err := db.newReadWriteTransaction()
	if err != nil {
		return err
	}
	defer t.Release()

	// Set up the bloom filters with the initial capacity and false positive
	// rate, or higher capacity if we've done this before and seen lots of
	// items. For simplicity's sake we track just one count, which is the
	// highest of the various indirected items.

	capacity := indirectGCBloomCapacity
	if db.gcKeyCount > capacity {
		capacity = db.gcKeyCount
	}
	blockFilter := newBloomFilter(capacity)
	versionFilter := newBloomFilter(capacity)

	// Iterate the FileInfos, unmarshal the block and version hashes and
	// add them to the filter.

	it, err := t.NewPrefixIterator([]byte{KeyTypeDevice})
	if err != nil {
		return err
	}
	defer it.Release()
	for it.Next() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		var hashes IndirectionHashesOnly
		if err := hashes.Unmarshal(it.Value()); err != nil {
			return err
		}
		if len(hashes.BlocksHash) > 0 {
			blockFilter.add(hashes.BlocksHash)
		}
		if len(hashes.VersionHash) > 0 {
			versionFilter.add(hashes.VersionHash)
		}
	}
	it.Release()
	if err := it.Error(); err != nil {
		return err
	}

	// Iterate over block lists, removing keys with hashes that don't match
	// the filter.

	it, err = t.NewPrefixIterator([]byte{KeyTypeBlockList})
	if err != nil {
		return err
	}
	defer it.Release()
	matchedBlocks := 0
	for it.Next() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		key := blockListKey(it.Key())
		if blockFilter.has(key.Hash()) {
			matchedBlocks++
			continue
		}
		if err := t.Delete(key); err != nil {
			return err
		}
	}
	it.Release()
	if err := it.Error(); err != nil {
		return err
	}

	// Iterate over version lists, removing keys with hashes that don't match
	// the filter.

	it, err = db.NewPrefixIterator([]byte{KeyTypeVersion})
	if err != nil {
		return err
	}
	matchedVersions := 0
	for it.Next() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		key := versionKey(it.Key())
		if versionFilter.has(key.Hash()) {
			matchedVersions++
			continue
		}
		if err := t.Delete(key); err != nil {
			return err
		}
	}
	it.Release()
	if err := it.Error(); err != nil {
		return err
	}

	// Remember the number of unique keys we kept until the next pass.
	db.gcKeyCount = matchedBlocks
	if matchedVersions > matchedBlocks {
		db.gcKeyCount = matchedVersions
	}

	if err := t.Commit(); err != nil {
		return err
	}

	return db.Compact()
}

func newBloomFilter(capacity int) bloomFilter {
	var buf [16]byte
	io.ReadFull(rand.Reader, buf[:])

	return bloomFilter{
		f: blobloom.NewOptimized(blobloom.Config{
			Capacity: uint64(capacity),
			FPRate:   indirectGCBloomFalsePositiveRate,
			MaxBits:  8 * indirectGCBloomMaxBytes,
		}),

		k0: binary.LittleEndian.Uint64(buf[:8]),
		k1: binary.LittleEndian.Uint64(buf[8:]),
	}
}

type bloomFilter struct {
	f      *blobloom.Filter
	k0, k1 uint64 // Random key for SipHash.
}

func (b *bloomFilter) add(id []byte)      { b.f.Add(b.hash(id)) }
func (b *bloomFilter) has(id []byte) bool { return b.f.Has(b.hash(id)) }

// Hash function for the bloomfilter: SipHash of the SHA-256.
//
// The randomization in SipHash means we get different collisions across
// runs and colliding keys are not kept indefinitely.
func (b *bloomFilter) hash(id []byte) uint64 {
	if len(id) != sha256.Size {
		panic("bug: bloomFilter.hash passed something not a SHA256 hash")
	}
	return siphash.Hash(b.k0, b.k1, id)
}

// checkRepair checks folder metadata and sequences for miscellaneous errors.
func (db *Lowlevel) checkRepair() {
	for _, folder := range db.ListFolders() {
		_ = db.getMetaAndCheck(folder)
	}
}

func (db *Lowlevel) getMetaAndCheck(folder string) *metadataTracker {
	db.gcMut.RLock()
	defer db.gcMut.RUnlock()

	var err error
	defer func() {
		if err != nil && !backend.IsClosed(err) {
			l.Warnf("Fatal error: %v", err)
			obfuscateAndPanic(err)
		}
	}()

	var fixed int
	fixed, err = db.checkLocalNeed([]byte(folder))
	if err != nil {
		err = fmt.Errorf("checking local need: %w", err)
		return nil
	}
	if fixed != 0 {
		l.Infof("Repaired %d local need entries for folder %v in database", fixed, folder)
	}

	meta, err := db.recalcMeta(folder)
	if err != nil {
		err = fmt.Errorf("recalculating metadata: %w", err)
		return nil
	}

	fixed, err = db.repairSequenceGCLocked(folder, meta)
	if err != nil {
		err = fmt.Errorf("repairing sequences: %w", err)
		return nil
	}
	if fixed != 0 {
		l.Infof("Repaired %d sequence entries for folder %v in database", fixed, folder)
	}

	return meta
}

func (db *Lowlevel) loadMetadataTracker(folder string) *metadataTracker {
	meta := newMetadataTracker(db.keyer)
	if err := meta.fromDB(db, []byte(folder)); err != nil {
		if err == errMetaInconsistent {
			l.Infof("Stored folder metadata for %q is inconsistent; recalculating", folder)
		} else {
			l.Infof("No stored folder metadata for %q; recalculating", folder)

		}
		return db.getMetaAndCheck(folder)
	}

	curSeq := meta.Sequence(protocol.LocalDeviceID)
	if metaOK := db.verifyLocalSequence(curSeq, folder); !metaOK {
		l.Infof("Stored folder metadata for %q is out of date after crash; recalculating", folder)
		return db.getMetaAndCheck(folder)
	}

	if age := time.Since(meta.Created()); age > db.recheckInterval {
		l.Infof("Stored folder metadata for %q is %v old; recalculating", folder, util.NiceDurationString(age))
		return db.getMetaAndCheck(folder)
	}

	return meta
}

func (db *Lowlevel) recalcMeta(folderStr string) (*metadataTracker, error) {
	folder := []byte(folderStr)

	meta := newMetadataTracker(db.keyer)
	if err := db.checkGlobals(folder); err != nil {
		return nil, fmt.Errorf("checking globals: %w", err)
	}

	t, err := db.newReadWriteTransaction(meta.CommitHook(folder))
	if err != nil {
		return nil, err
	}
	defer t.close()

	var deviceID protocol.DeviceID
	err = t.withAllFolderTruncated(folder, func(device []byte, f FileInfoTruncated) bool {
		copy(deviceID[:], device)
		meta.addFile(deviceID, f)
		return true
	})
	if err != nil {
		return nil, err
	}

	err = t.withGlobal(folder, nil, true, func(f protocol.FileIntf) bool {
		meta.addFile(protocol.GlobalDeviceID, f)
		return true
	})

	meta.emptyNeeded(protocol.LocalDeviceID)
	err = t.withNeed(folder, protocol.LocalDeviceID[:], true, func(f protocol.FileIntf) bool {
		meta.addNeeded(protocol.LocalDeviceID, f)
		return true
	})
	if err != nil {
		return nil, err
	}
	for _, device := range meta.devices() {
		meta.emptyNeeded(device)
		err = t.withNeed(folder, device[:], true, func(f protocol.FileIntf) bool {
			meta.addNeeded(device, f)
			return true
		})
		if err != nil {
			return nil, err
		}
	}

	meta.SetCreated()
	if err := t.Commit(); err != nil {
		return nil, err
	}
	return meta, nil
}

// Verify the local sequence number from actual sequence entries. Returns
// true if it was all good, or false if a fixup was necessary.
func (db *Lowlevel) verifyLocalSequence(curSeq int64, folder string) bool {
	// Walk the sequence index from the current (supposedly) highest
	// sequence number and raise the alarm if we get anything. This recovers
	// from the occasion where we have written sequence entries to disk but
	// not yet written new metadata to disk.
	//
	// Note that we can have the same thing happen for remote devices but
	// there it's not a problem -- we'll simply advertise a lower sequence
	// number than we've actually seen and receive some duplicate updates
	// and then be in sync again.

	t, err := db.newReadOnlyTransaction()
	if err != nil {
		l.Warnf("Fatal error: %v", err)
		obfuscateAndPanic(err)
	}
	ok := true
	if err := t.withHaveSequence([]byte(folder), curSeq+1, func(fi protocol.FileIntf) bool {
		ok = false // we got something, which we should not have
		return false
	}); err != nil && !backend.IsClosed(err) {
		l.Warnf("Fatal error: %v", err)
		obfuscateAndPanic(err)
	}
	t.close()

	return ok
}

// repairSequenceGCLocked makes sure the sequence numbers in the sequence keys
// match those in the corresponding file entries. It returns the amount of fixed
// entries.
func (db *Lowlevel) repairSequenceGCLocked(folderStr string, meta *metadataTracker) (int, error) {
	t, err := db.newReadWriteTransaction(meta.CommitHook([]byte(folderStr)))
	if err != nil {
		return 0, err
	}
	defer t.close()

	fixed := 0

	folder := []byte(folderStr)

	// First check that every file entry has a matching sequence entry
	// (this was previously db schema upgrade to 9).

	dk, err := t.keyer.GenerateDeviceFileKey(nil, folder, protocol.LocalDeviceID[:], nil)
	if err != nil {
		return 0, err
	}
	it, err := t.NewPrefixIterator(dk.WithoutName())
	if err != nil {
		return 0, err
	}
	defer it.Release()

	var sk sequenceKey
	for it.Next() {
		intf, err := t.unmarshalTrunc(it.Value(), false)
		if err != nil {
			return 0, err
		}
		fi := intf.(protocol.FileInfo)
		if sk, err = t.keyer.GenerateSequenceKey(sk, folder, fi.Sequence); err != nil {
			return 0, err
		}
		switch dk, err = t.Get(sk); {
		case err != nil:
			if !backend.IsNotFound(err) {
				return 0, err
			}
			fallthrough
		case !bytes.Equal(it.Key(), dk):
			fixed++
			fi.Sequence = meta.nextLocalSeq()
			if sk, err = t.keyer.GenerateSequenceKey(sk, folder, fi.Sequence); err != nil {
				return 0, err
			}
			if err := t.Put(sk, it.Key()); err != nil {
				return 0, err
			}
			if err := t.putFile(it.Key(), fi); err != nil {
				return 0, err
			}
		}
		if err := t.Checkpoint(); err != nil {
			return 0, err
		}
	}
	if err := it.Error(); err != nil {
		return 0, err
	}

	it.Release()

	// Secondly check there's no sequence entries pointing at incorrect things.

	sk, err = t.keyer.GenerateSequenceKey(sk, folder, 0)
	if err != nil {
		return 0, err
	}

	it, err = t.NewPrefixIterator(sk.WithoutSequence())
	if err != nil {
		return 0, err
	}
	defer it.Release()

	for it.Next() {
		// Check that the sequence from the key matches the
		// sequence in the file.
		fi, ok, err := t.getFileTrunc(it.Value(), true)
		if err != nil {
			return 0, err
		}
		if ok {
			if seq := t.keyer.SequenceFromSequenceKey(it.Key()); seq == fi.SequenceNo() {
				continue
			}
		}
		// Either the file is missing or has a different sequence number
		fixed++
		if err := t.Delete(it.Key()); err != nil {
			return 0, err
		}
	}
	if err := it.Error(); err != nil {
		return 0, err
	}

	it.Release()

	return fixed, t.Commit()
}

// Does not take care of metadata - if anything is repaired, the need count
// needs to be recalculated.
func (db *Lowlevel) checkLocalNeed(folder []byte) (int, error) {
	repaired := 0

	t, err := db.newReadWriteTransaction()
	if err != nil {
		return 0, err
	}
	defer t.close()

	key, err := t.keyer.GenerateNeedFileKey(nil, folder, nil)
	if err != nil {
		return 0, err
	}
	dbi, err := t.NewPrefixIterator(key.WithoutName())
	if err != nil {
		return 0, err
	}
	defer dbi.Release()

	var needName string
	var needDone bool
	next := func() {
		needDone = !dbi.Next()
		if !needDone {
			needName = string(t.keyer.NameFromGlobalVersionKey(dbi.Key()))
		}
	}
	next()
	t.withNeedIteratingGlobal(folder, protocol.LocalDeviceID[:], true, func(fi protocol.FileIntf) bool {
		f := fi.(FileInfoTruncated)
		for !needDone && needName < f.Name {
			repaired++
			if err = t.Delete(dbi.Key()); err != nil && !backend.IsNotFound(err) {
				return false
			}
			l.Debugln("check local need: removing", needName)
			next()
		}
		if needName == f.Name {
			next()
		} else {
			repaired++
			key, err = t.keyer.GenerateNeedFileKey(key, folder, []byte(f.Name))
			if err != nil {
				return false
			}
			if err = t.Put(key, nil); err != nil {
				return false
			}
			l.Debugln("check local need: adding", f.Name)
		}
		return true
	})
	if err != nil {
		return 0, err
	}

	for !needDone {
		repaired++
		if err := t.Delete(dbi.Key()); err != nil && !backend.IsNotFound(err) {
			return 0, err
		}
		l.Debugln("check local need: removing", needName)
		next()
	}

	if err := dbi.Error(); err != nil {
		return 0, err
	}
	dbi.Release()

	if err = t.Commit(); err != nil {
		return 0, err
	}

	return repaired, nil
}

func (db *Lowlevel) needsRepairPath() string {
	path := db.Location()
	if path == "" {
		return ""
	}
	if path[len(path)-1] == fs.PathSeparator {
		path = path[:len(path)-1]
	}
	return path + needsRepairSuffix
}

// unchanged checks if two files are the same and thus don't need to be updated.
// Local flags or the invalid bit might change without the version
// being bumped.
func unchanged(nf, ef protocol.FileIntf) bool {
	return ef.FileVersion().Equal(nf.FileVersion()) && ef.IsInvalid() == nf.IsInvalid() && ef.FileLocalFlags() == nf.FileLocalFlags()
}

var ldbPathRe = regexp.MustCompile(`(open|write|read) .+[\\/].+[\\/]index[^\\/]+[\\/][^\\/]+: `)

func obfuscateAndPanic(err error) {
	panic(ldbPathRe.ReplaceAllString(err.Error(), "$1 x: "))
}
