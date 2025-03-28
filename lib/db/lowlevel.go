// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package db

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/maphash"
	"os"
	"regexp"
	"time"

	"github.com/greatroar/blobloom"
	"github.com/thejerf/suture/v4"
	"google.golang.org/protobuf/proto"

	"github.com/syncthing/syncthing/internal/gen/dbproto"
	"github.com/syncthing/syncthing/lib/db/backend"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/stringutil"
	"github.com/syncthing/syncthing/lib/svcutil"
	"github.com/syncthing/syncthing/lib/sync"
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
	evLogger           events.Logger

	blockFilter   *bloomFilter
	versionFilter *bloomFilter
}

func NewLowlevel(backend backend.Backend, evLogger events.Logger, opts ...Option) (*Lowlevel, error) {
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
		evLogger:           evLogger,
	}
	for _, opt := range opts {
		opt(db)
	}
	db.keyer = newDefaultKeyer(db.folderIdx, db.deviceIdx)
	db.Add(svcutil.AsService(db.gcRunner, "db.Lowlevel/gcRunner"))
	if path := db.needsRepairPath(); path != "" {
		if _, err := os.Lstat(path); err == nil {
			l.Infoln("Database was marked for repair - this may take a while")
			if err := db.checkRepair(); err != nil {
				db.handleFailure(err)
				return nil, err
			}
			os.Remove(path)
		}
	}
	return db, nil
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
		keyBuf, err = t.updateGlobal(gk, keyBuf, folder, device, f, meta)
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

		blocksHashSame := ok && bytes.Equal(ef.BlocksHash, f.BlocksHash)
		if ok {
			keyBuf, err = db.removeLocalBlockAndSequenceInfo(keyBuf, folder, name, ef, !blocksHashSame, &t)
			if err != nil {
				return err
			}
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
		keyBuf, err = t.updateGlobal(gk, keyBuf, folder, protocol.LocalDeviceID[:], f, meta)
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

func (db *Lowlevel) removeLocalFiles(folder []byte, nameStrs []string, meta *metadataTracker) error {
	db.gcMut.RLock()
	defer db.gcMut.RUnlock()

	t, err := db.newReadWriteTransaction(meta.CommitHook(folder))
	if err != nil {
		return err
	}
	defer t.close()

	var dk, gk, buf []byte
	for _, nameStr := range nameStrs {
		name := []byte(nameStr)
		dk, err = db.keyer.GenerateDeviceFileKey(dk, folder, protocol.LocalDeviceID[:], name)
		if err != nil {
			return err
		}

		ef, ok, err := t.getFileByKey(dk)
		if err != nil {
			return err
		}
		if !ok {
			l.Debugf("remove (local); folder=%q %v: file doesn't exist", folder, nameStr)
			continue
		}

		buf, err = db.removeLocalBlockAndSequenceInfo(buf, folder, name, ef, true, &t)
		if err != nil {
			return err
		}

		meta.removeFile(protocol.LocalDeviceID, ef)

		gk, err = db.keyer.GenerateGlobalVersionKey(gk, folder, name)
		if err != nil {
			return err
		}
		buf, err = t.removeFromGlobal(gk, buf, folder, protocol.LocalDeviceID[:], name, meta)
		if err != nil {
			return err
		}

		err = t.Delete(dk)
		if err != nil {
			return err
		}

		if err := t.Checkpoint(); err != nil {
			return err
		}
	}

	return t.Commit()
}

func (db *Lowlevel) removeLocalBlockAndSequenceInfo(keyBuf, folder, name []byte, ef protocol.FileInfo, removeFromBlockListMap bool, t *readWriteTransaction) ([]byte, error) {
	var err error
	if len(ef.Blocks) != 0 && !ef.IsInvalid() && ef.Size > 0 {
		for _, block := range ef.Blocks {
			keyBuf, err = db.keyer.GenerateBlockMapKey(keyBuf, folder, block.Hash, name)
			if err != nil {
				return nil, err
			}
			if err := t.Delete(keyBuf); err != nil {
				return nil, err
			}
		}
		if removeFromBlockListMap {
			keyBuf, err := db.keyer.GenerateBlockListMapKey(keyBuf, folder, ef.BlocksHash, name)
			if err != nil {
				return nil, err
			}
			if err = t.Delete(keyBuf); err != nil {
				return nil, err
			}
		}
	}

	keyBuf, err = db.keyer.GenerateSequenceKey(keyBuf, folder, ef.SequenceNo())
	if err != nil {
		return nil, err
	}
	if err := t.Delete(keyBuf); err != nil {
		return nil, err
	}
	l.Debugf("removing sequence; folder=%q sequence=%v %v", folder, ef.SequenceNo(), ef.FileName())
	return keyBuf, nil
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

func (db *Lowlevel) checkGlobals(folderStr string) (int, error) {
	t, err := db.newReadWriteTransaction()
	if err != nil {
		return 0, err
	}
	defer t.close()

	folder := []byte(folderStr)
	key, err := db.keyer.GenerateGlobalVersionKey(nil, folder, nil)
	if err != nil {
		return 0, err
	}
	dbi, err := t.NewPrefixIterator(key.WithoutName())
	if err != nil {
		return 0, err
	}
	defer dbi.Release()

	fixed := 0
	var dk []byte
	ro := t.readOnlyTransaction
	for dbi.Next() {
		var vl dbproto.VersionList
		if err := proto.Unmarshal(dbi.Value(), &vl); err != nil || len(vl.Versions) == 0 {
			if err := t.Delete(dbi.Key()); err != nil && !backend.IsNotFound(err) {
				return 0, err
			}
			continue
		}

		// Check the global version list for consistency. An issue in previous
		// versions of goleveldb could result in reordered writes so that
		// there are global entries pointing to no longer existing files. Here
		// we find those and clear them out.

		name := db.keyer.NameFromGlobalVersionKey(dbi.Key())
		newVL := &dbproto.VersionList{}
		var changed, changedHere bool
		for _, fv := range vl.Versions {
			changedHere, err = checkGlobalsFilterDevices(dk, folder, name, fv.Devices, newVL, ro)
			if err != nil {
				return 0, err
			}
			changed = changed || changedHere

			changedHere, err = checkGlobalsFilterDevices(dk, folder, name, fv.InvalidDevices, newVL, ro)
			if err != nil {
				return 0, err
			}
			changed = changed || changedHere
		}

		if len(newVL.Versions) == 0 {
			if err := t.Delete(dbi.Key()); err != nil && !backend.IsNotFound(err) {
				return 0, err
			}
			fixed++
		} else if changed {
			if err := t.Put(dbi.Key(), mustMarshal(newVL)); err != nil {
				return 0, err
			}
			fixed++
		}
	}
	dbi.Release()
	if err := dbi.Error(); err != nil {
		return 0, err
	}

	l.Debugf("global db check completed for %v", folder)
	return fixed, t.Commit()
}

func checkGlobalsFilterDevices(dk, folder, name []byte, devices [][]byte, vl *dbproto.VersionList, t readOnlyTransaction) (bool, error) {
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
		_, _, _, _, _, _, err = vlUpdate(vl, folder, device, f, t)
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
		return 0, nil //nolint: nilerr
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

func (db *Lowlevel) dropIndexIDs() error {
	t, err := db.newReadWriteTransaction()
	if err != nil {
		return err
	}
	defer t.close()
	if err := t.deleteKeyPrefix([]byte{KeyTypeIndexID}); err != nil {
		return err
	}
	return t.Commit()
}

// dropOtherDeviceIndexIDs drops all index IDs for devices other than the
// local device. This means we will resend our indexes to all other devices,
// but they don't have to resend to us.
func (db *Lowlevel) dropOtherDeviceIndexIDs() error {
	t, err := db.newReadWriteTransaction()
	if err != nil {
		return err
	}
	defer t.close()
	if err := t.deleteKeyPrefixMatching([]byte{KeyTypeIndexID}, func(key []byte) bool {
		dev, _ := t.keyer.DeviceFromIndexIDKey(key)
		return !bytes.Equal(dev, protocol.LocalDeviceID[:])
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

func (db *Lowlevel) gcIndirect(ctx context.Context) (err error) {
	// The indirection GC uses bloom filters to track used block lists and
	// versions. This means iterating over all items, adding their hashes to
	// the filter, then iterating over the indirected items and removing
	// those that don't match the filter. The filter will give false
	// positives so we will keep around one percent of things that we don't
	// really need (at most).
	//
	// Indirection GC needs to run when there are no modifications to the
	// FileInfos or indirected items.

	l.Debugln("Starting database GC")

	// Create a new set of bloom filters, while holding the gcMut which
	// guarantees that no other modifications are happening concurrently.

	db.gcMut.Lock()
	capacity := indirectGCBloomCapacity
	if db.gcKeyCount > capacity {
		capacity = db.gcKeyCount
	}
	db.blockFilter = newBloomFilter(capacity)
	db.versionFilter = newBloomFilter(capacity)
	db.gcMut.Unlock()

	defer func() {
		// Forget the bloom filters on the way out.
		db.gcMut.Lock()
		db.blockFilter = nil
		db.versionFilter = nil
		db.gcMut.Unlock()
	}()

	var discardedBlocks, matchedBlocks, discardedVersions, matchedVersions int

	t, err := db.newReadWriteTransaction()
	if err != nil {
		return err
	}
	defer t.Release()

	// Set up the bloom filters with the initial capacity and false positive
	// rate, or higher capacity if we've done this before and seen lots of
	// items. For simplicity's sake we track just one count, which is the
	// highest of the various indirected items.

	// Iterate the FileInfos, unmarshal the block and version hashes and
	// add them to the filter.

	// This happens concurrently with normal database modifications, though
	// those modifications will now also add their blocks and versions to
	// the bloom filters.

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

		var hashes dbproto.IndirectionHashesOnly
		if err := proto.Unmarshal(it.Value(), &hashes); err != nil {
			return err
		}
		db.recordIndirectionHashes(&hashes)
	}
	it.Release()
	if err := it.Error(); err != nil {
		return err
	}

	// For the next phase we grab the GC lock again and hold it for the rest
	// of the method call. Now there can't be any further modifications to
	// the database or the bloom filters.

	db.gcMut.Lock()
	defer db.gcMut.Unlock()

	// Only print something if the process takes more than "a moment".
	logWait := make(chan struct{})
	logTimer := time.AfterFunc(10*time.Second, func() {
		l.Infoln("Database GC in progress - many Syncthing operations will be unresponsive until it's finished")
		close(logWait)
	})
	defer func() {
		if logTimer.Stop() {
			return
		}
		<-logWait // Make sure messages are sent in order.
		l.Infof("Database GC complete (discarded/remaining: %v/%v blocks, %v/%v versions)",
			discardedBlocks, matchedBlocks, discardedVersions, matchedVersions)
	}()

	// Iterate over block lists, removing keys with hashes that don't match
	// the filter.

	it, err = t.NewPrefixIterator([]byte{KeyTypeBlockList})
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

		key := blockListKey(it.Key())
		if db.blockFilter.has(key.Hash()) {
			matchedBlocks++
			continue
		}
		if err := t.Delete(key); err != nil {
			return err
		}
		discardedBlocks++
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
	for it.Next() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		key := versionKey(it.Key())
		if db.versionFilter.has(key.Hash()) {
			matchedVersions++
			continue
		}
		if err := t.Delete(key); err != nil {
			return err
		}
		discardedVersions++
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

	l.Debugf("Finished GC (discarded/remaining: %v/%v blocks, %v/%v versions)", discardedBlocks, matchedBlocks, discardedVersions, matchedVersions)

	return nil
}

func (db *Lowlevel) recordIndirectionHashesForFile(f *protocol.FileInfo) {
	db.recordIndirectionHashes(&dbproto.IndirectionHashesOnly{BlocksHash: f.BlocksHash, VersionHash: f.VersionHash})
}

func (db *Lowlevel) recordIndirectionHashes(hs *dbproto.IndirectionHashesOnly) {
	// must be called with gcMut held (at least read-held)
	if db.blockFilter != nil && len(hs.BlocksHash) > 0 {
		db.blockFilter.add(hs.BlocksHash)
	}
	if db.versionFilter != nil && len(hs.VersionHash) > 0 {
		db.versionFilter.add(hs.VersionHash)
	}
}

func newBloomFilter(capacity int) *bloomFilter {
	return &bloomFilter{
		f: blobloom.NewSyncOptimized(blobloom.Config{
			Capacity: uint64(capacity),
			FPRate:   indirectGCBloomFalsePositiveRate,
			MaxBits:  8 * indirectGCBloomMaxBytes,
		}),
		seed: maphash.MakeSeed(),
	}
}

type bloomFilter struct {
	f    *blobloom.SyncFilter
	seed maphash.Seed
}

func (b *bloomFilter) add(id []byte)      { b.f.Add(b.hash(id)) }
func (b *bloomFilter) has(id []byte) bool { return b.f.Has(b.hash(id)) }

// Hash function for the bloomfilter: maphash of the SHA-256.
//
// The randomization in maphash should ensure that we get different collisions
// across runs, so colliding keys are not kept indefinitely.
func (b *bloomFilter) hash(id []byte) uint64 {
	if len(id) != sha256.Size {
		panic("bug: bloomFilter.hash passed something not a SHA256 hash")
	}
	var h maphash.Hash
	h.SetSeed(b.seed)
	_, _ = h.Write(id)
	return h.Sum64()
}

// checkRepair checks folder metadata and sequences for miscellaneous errors.
func (db *Lowlevel) checkRepair() error {
	db.gcMut.RLock()
	defer db.gcMut.RUnlock()
	for _, folder := range db.ListFolders() {
		if _, err := db.getMetaAndCheckGCLocked(folder); err != nil {
			return err
		}
	}
	return nil
}

func (db *Lowlevel) getMetaAndCheck(folder string) (*metadataTracker, error) {
	db.gcMut.RLock()
	defer db.gcMut.RUnlock()

	return db.getMetaAndCheckGCLocked(folder)
}

func (db *Lowlevel) getMetaAndCheckGCLocked(folder string) (*metadataTracker, error) {
	fixed, err := db.checkLocalNeed([]byte(folder))
	if err != nil {
		return nil, fmt.Errorf("checking local need: %w", err)
	}
	if fixed != 0 {
		l.Infof("Repaired %d local need entries for folder %v in database", fixed, folder)
	}

	fixed, err = db.checkGlobals(folder)
	if err != nil {
		return nil, fmt.Errorf("checking globals: %w", err)
	}
	if fixed != 0 {
		l.Infof("Repaired %d global entries for folder %v in database", fixed, folder)
	}

	oldMeta := newMetadataTracker(db.keyer, db.evLogger)
	_ = oldMeta.fromDB(db, []byte(folder)) // Ignore error, it leads to index id reset too
	meta, err := db.recalcMeta(folder)
	if err != nil {
		return nil, fmt.Errorf("recalculating metadata: %w", err)
	}

	fixed, err = db.repairSequenceGCLocked(folder, meta)
	if err != nil {
		return nil, fmt.Errorf("repairing sequences: %w", err)
	}
	if fixed != 0 {
		l.Infof("Repaired %d sequence entries for folder %v in database", fixed, folder)
		meta, err = db.recalcMeta(folder)
		if err != nil {
			return nil, fmt.Errorf("recalculating metadata: %w", err)
		}
	}

	if err := db.checkSequencesUnchanged(folder, oldMeta, meta); err != nil {
		return nil, fmt.Errorf("checking for changed sequences: %w", err)
	}

	return meta, nil
}

func (db *Lowlevel) loadMetadataTracker(folder string) (*metadataTracker, error) {
	meta := newMetadataTracker(db.keyer, db.evLogger)
	if err := meta.fromDB(db, []byte(folder)); err != nil {
		if errors.Is(err, errMetaInconsistent) {
			l.Infof("Stored folder metadata for %q is inconsistent; recalculating", folder)
		} else {
			l.Infof("No stored folder metadata for %q; recalculating", folder)
		}
		return db.getMetaAndCheck(folder)
	}

	curSeq := meta.Sequence(protocol.LocalDeviceID)
	if metaOK, err := db.verifyLocalSequence(curSeq, folder); err != nil {
		return nil, fmt.Errorf("verifying sequences: %w", err)
	} else if !metaOK {
		l.Infof("Stored folder metadata for %q is out of date after crash; recalculating", folder)
		return db.getMetaAndCheck(folder)
	}

	if age := time.Since(meta.Created()); age > db.recheckInterval {
		l.Infof("Stored folder metadata for %q is %v old; recalculating", folder, stringutil.NiceDurationString(age))
		return db.getMetaAndCheck(folder)
	}

	return meta, nil
}

func (db *Lowlevel) recalcMeta(folderStr string) (*metadataTracker, error) {
	folder := []byte(folderStr)

	meta := newMetadataTracker(db.keyer, db.evLogger)

	t, err := db.newReadWriteTransaction(meta.CommitHook(folder))
	if err != nil {
		return nil, err
	}
	defer t.close()

	var deviceID protocol.DeviceID
	err = t.withAllFolderTruncated(folder, func(device []byte, f protocol.FileInfo) bool {
		copy(deviceID[:], device)
		meta.addFile(deviceID, f)
		return true
	})
	if err != nil {
		return nil, err
	}

	err = t.withGlobal(folder, nil, true, func(f protocol.FileInfo) bool {
		meta.addFile(protocol.GlobalDeviceID, f)
		return true
	})
	if err != nil {
		return nil, err
	}

	meta.emptyNeeded(protocol.LocalDeviceID)
	err = t.withNeed(folder, protocol.LocalDeviceID[:], true, func(f protocol.FileInfo) bool {
		meta.addNeeded(protocol.LocalDeviceID, f)
		return true
	})
	if err != nil {
		return nil, err
	}
	for _, device := range meta.devices() {
		meta.emptyNeeded(device)
		err = t.withNeed(folder, device[:], true, func(f protocol.FileInfo) bool {
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
func (db *Lowlevel) verifyLocalSequence(curSeq int64, folder string) (bool, error) {
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
		return false, err
	}
	ok := true
	if err := t.withHaveSequence([]byte(folder), curSeq+1, func(_ protocol.FileInfo) bool {
		ok = false // we got something, which we should not have
		return false
	}); err != nil {
		return false, err
	}
	t.close()

	return ok, nil
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
			// Delete local items with invalid indirected blocks/versions.
			// They will be rescanned.
			var ierr *blocksIndirectionError
			if ok := errors.As(err, &ierr); ok && backend.IsNotFound(err) {
				intf, err = t.unmarshalTrunc(it.Value(), true)
				if err != nil {
					return 0, err
				}
				name := []byte(intf.FileName())
				gk, err := t.keyer.GenerateGlobalVersionKey(nil, folder, name)
				if err != nil {
					return 0, err
				}
				_, err = t.removeFromGlobal(gk, nil, folder, protocol.LocalDeviceID[:], name, nil)
				if err != nil {
					return 0, err
				}
				sk, err = db.keyer.GenerateSequenceKey(sk, folder, intf.SequenceNo())
				if err != nil {
					return 0, err
				}
				if err := t.Delete(sk); err != nil {
					return 0, err
				}
				if err := t.Delete(it.Key()); err != nil {
					return 0, err
				}
			}
			return 0, err
		}
		if sk, err = t.keyer.GenerateSequenceKey(sk, folder, intf.Sequence); err != nil {
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
			intf.Sequence = meta.nextLocalSeq()
			if sk, err = t.keyer.GenerateSequenceKey(sk, folder, intf.Sequence); err != nil {
				return 0, err
			}
			if err := t.Put(sk, it.Key()); err != nil {
				return 0, err
			}
			if err := t.putFile(it.Key(), intf); err != nil {
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
	itErr := t.withNeedIteratingGlobal(folder, protocol.LocalDeviceID[:], true, func(fi protocol.FileInfo) bool {
		for !needDone && needName < fi.Name {
			repaired++
			if err = t.Delete(dbi.Key()); err != nil && !backend.IsNotFound(err) {
				return false
			}
			l.Debugln("check local need: removing", needName)
			next()
		}
		if needName == fi.Name {
			next()
		} else {
			repaired++
			key, err = t.keyer.GenerateNeedFileKey(key, folder, []byte(fi.Name))
			if err != nil {
				return false
			}
			if err = t.Put(key, nil); err != nil {
				return false
			}
			l.Debugln("check local need: adding", fi.Name)
		}
		return true
	})
	if err != nil {
		return 0, err
	}
	if itErr != nil {
		return 0, itErr
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

// checkSequencesUnchanged resets delta indexes for any device where the
// sequence changed.
func (db *Lowlevel) checkSequencesUnchanged(folder string, oldMeta, meta *metadataTracker) error {
	t, err := db.newReadWriteTransaction()
	if err != nil {
		return err
	}
	defer t.close()

	var key []byte
	deleteIndexID := func(devID protocol.DeviceID) error {
		key, err = db.keyer.GenerateIndexIDKey(key, devID[:], []byte(folder))
		if err != nil {
			return err
		}
		return t.Delete(key)
	}

	if oldMeta.Sequence(protocol.LocalDeviceID) != meta.Sequence(protocol.LocalDeviceID) {
		if err := deleteIndexID(protocol.LocalDeviceID); err != nil {
			return err
		}
		l.Infof("Local sequence for folder %v changed while repairing - dropping delta indexes", folder)
	}

	oldDevices := oldMeta.devices()
	oldSequences := make(map[protocol.DeviceID]int64, len(oldDevices))
	for _, devID := range oldDevices {
		oldSequences[devID] = oldMeta.Sequence(devID)
	}
	for _, devID := range meta.devices() {
		oldSeq := oldSequences[devID]
		delete(oldSequences, devID)
		// A lower sequence number just means we will receive some indexes again.
		if oldSeq >= meta.Sequence(devID) {
			if oldSeq > meta.Sequence(devID) {
				db.evLogger.Log(events.Failure, "lower remote sequence after recalculating metadata")
			}
			continue
		}
		db.evLogger.Log(events.Failure, "higher remote sequence after recalculating metadata")
		if err := deleteIndexID(devID); err != nil {
			return err
		}
		l.Infof("Sequence of device %v for folder %v changed while repairing - dropping delta indexes", devID.Short(), folder)
	}
	for devID := range oldSequences {
		if err := deleteIndexID(devID); err != nil {
			return err
		}
		l.Debugf("Removed indexID of device %v for folder %v which isn't present anymore", devID.Short(), folder)
	}

	return t.Commit()
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

func (db *Lowlevel) checkErrorForRepair(err error) {
	if errors.Is(err, errEntryFromGlobalMissing) || errors.Is(err, errEmptyGlobal) {
		// Inconsistency error, mark db for repair on next start.
		if path := db.needsRepairPath(); path != "" {
			if fd, err := os.Create(path); err == nil {
				fd.Close()
			}
		}
	}
}

func (db *Lowlevel) handleFailure(err error) {
	db.checkErrorForRepair(err)
	if shouldReportFailure(err) {
		db.evLogger.Log(events.Failure, err.Error())
	}
}

var ldbPathRe = regexp.MustCompile(`(open|write|read) .+[\\/].+[\\/]index[^\\/]+[\\/][^\\/]+: `)

func shouldReportFailure(err error) bool {
	return !ldbPathRe.MatchString(err.Error())
}
