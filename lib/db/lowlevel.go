// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package db

import (
	"bytes"
	"encoding/binary"
	"os"
	"time"

	"github.com/syncthing/syncthing/lib/db/backend"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/sync"
	"github.com/willf/bloom"
)

const (
	// We set the bloom filter capacity to handle 100k individual block lists
	// with a false positive probability of 1% for the first pass. Once we know
	// how many block lists we have we will use that number instead, if it's
	// more than 100k. For fewer than 100k block lists we will just get better
	// false positive rate instead.
	blockGCBloomCapacity          = 100000
	blockGCBloomFalsePositiveRate = 0.01 // 1%
	blockGCDefaultInterval        = 13 * time.Hour
	blockGCTimeKey                = "lastBlockGCTime"
)

var blockGCInterval = blockGCDefaultInterval

func init() {
	if dur, err := time.ParseDuration(os.Getenv("STGCBLOCKSEVERY")); err == nil {
		blockGCInterval = dur
	}
}

// Lowlevel is the lowest level database interface. It has a very simple
// purpose: hold the actual backend database, and the in-memory state
// that belong to that database. In the same way that a single on disk
// database can only be opened once, there should be only one Lowlevel for
// any given backend.
type Lowlevel struct {
	backend.Backend
	folderIdx  *smallIndex
	deviceIdx  *smallIndex
	keyer      keyer
	gcMut      sync.RWMutex
	gcKeyCount int
	gcStop     chan struct{}
}

func NewLowlevel(backend backend.Backend) *Lowlevel {
	db := &Lowlevel{
		Backend:   backend,
		folderIdx: newSmallIndex(backend, []byte{KeyTypeFolderIdx}),
		deviceIdx: newSmallIndex(backend, []byte{KeyTypeDeviceIdx}),
		gcMut:     sync.NewRWMutex(),
		gcStop:    make(chan struct{}),
	}
	db.keyer = newDefaultKeyer(db.folderIdx, db.deviceIdx)
	go db.gcRunner()
	return db
}

func (db *Lowlevel) Close() error {
	close(db.gcStop)
	return db.Backend.Close()
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

	t, err := db.newReadWriteTransaction()
	if err != nil {
		return err
	}
	defer t.close()

	var dk, gk, keyBuf []byte
	devID := protocol.DeviceIDFromBytes(device)
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
			continue
		}

		if ok {
			meta.removeFile(devID, ef)
		}
		meta.addFile(devID, f)

		l.Debugf("insert; folder=%q device=%v %v", folder, devID, f)
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

		if err := t.Checkpoint(func() error {
			return meta.toDB(t, folder)
		}); err != nil {
			return err
		}
	}

	if err := meta.toDB(t, folder); err != nil {
		return err
	}

	return t.Commit()
}

// updateLocalFiles adds fileinfos to the db, and updates the global versionlist,
// metadata, sequence and blockmap buckets.
func (db *Lowlevel) updateLocalFiles(folder []byte, fs []protocol.FileInfo, meta *metadataTracker) error {
	db.gcMut.RLock()
	defer db.gcMut.RUnlock()

	t, err := db.newReadWriteTransaction()
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
			continue
		}

		if ok {
			if !ef.IsDirectory() && !ef.IsDeleted() && !ef.IsInvalid() {
				for _, block := range ef.Blocks {
					keyBuf, err = db.keyer.GenerateBlockMapKey(keyBuf, folder, block.Hash, name)
					if err != nil {
						return err
					}
					if err := t.Delete(keyBuf); err != nil {
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

		if !f.IsDirectory() && !f.IsDeleted() && !f.IsInvalid() {
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
		}

		if err := t.Checkpoint(func() error {
			return meta.toDB(t, folder)
		}); err != nil {
			return err
		}
	}

	if err := meta.toDB(t, folder); err != nil {
		return err
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
	k1, err := db.keyer.GenerateSequenceKey(nil, folder, 0)
	if err != nil {
		return err
	}
	if err := t.deleteKeyPrefix(k1.WithoutSequence()); err != nil {
		return err
	}

	// Remove all items related to the given folder from the global bucket
	k2, err := db.keyer.GenerateGlobalVersionKey(nil, folder, nil)
	if err != nil {
		return err
	}
	if err := t.deleteKeyPrefix(k2.WithoutName()); err != nil {
		return err
	}

	// Remove all needs related to the folder
	k3, err := db.keyer.GenerateNeedFileKey(nil, folder, nil)
	if err != nil {
		return err
	}
	if err := t.deleteKeyPrefix(k3.WithoutName()); err != nil {
		return err
	}

	// Remove the blockmap of the folder
	k4, err := db.keyer.GenerateBlockMapKey(nil, folder, nil, nil)
	if err != nil {
		return err
	}
	if err := t.deleteKeyPrefix(k4.WithoutHashAndName()); err != nil {
		return err
	}

	return t.Commit()
}

func (db *Lowlevel) dropDeviceFolder(device, folder []byte, meta *metadataTracker) error {
	db.gcMut.RLock()
	defer db.gcMut.RUnlock()

	t, err := db.newReadWriteTransaction()
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
	if err := dbi.Error(); err != nil {
		return err
	}
	dbi.Release()

	if bytes.Equal(device, protocol.LocalDeviceID[:]) {
		key, err := db.keyer.GenerateBlockMapKey(nil, folder, nil, nil)
		if err != nil {
			return err
		}
		if err := t.deleteKeyPrefix(key.WithoutHashAndName()); err != nil {
			return err
		}
	}
	return t.Commit()
}

func (db *Lowlevel) checkGlobals(folder []byte, meta *metadataTracker) error {
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
	for dbi.Next() {
		vl, ok := unmarshalVersionList(dbi.Value())
		if !ok {
			continue
		}

		// Check the global version list for consistency. An issue in previous
		// versions of goleveldb could result in reordered writes so that
		// there are global entries pointing to no longer existing files. Here
		// we find those and clear them out.

		name := db.keyer.NameFromGlobalVersionKey(dbi.Key())
		var newVL VersionList
		for i, version := range vl.Versions {
			dk, err = db.keyer.GenerateDeviceFileKey(dk, folder, version.Device, name)
			if err != nil {
				return err
			}
			_, err := t.Get(dk)
			if backend.IsNotFound(err) {
				continue
			}
			if err != nil {
				return err
			}
			newVL.Versions = append(newVL.Versions, version)

			if i == 0 {
				if fi, ok, err := t.getFileByKey(dk); err != nil {
					return err
				} else if ok {
					meta.addFile(protocol.GlobalDeviceID, fi)
				}
			}
		}

		if len(newVL.Versions) != len(vl.Versions) {
			if err := t.Put(dbi.Key(), mustMarshal(&newVL)); err != nil {
				return err
			}
		}
	}
	if err := dbi.Error(); err != nil {
		return err
	}

	l.Debugf("db check completed for %q", folder)
	return t.Commit()
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

func (db *Lowlevel) gcRunner() {
	t := time.NewTimer(db.timeUntil(blockGCTimeKey, blockGCInterval))
	defer t.Stop()
	for {
		select {
		case <-db.gcStop:
			return
		case <-t.C:
			if err := db.gcBlocks(); err != nil {
				l.Warnln("Database block GC failed:", err)
			}
			db.recordTime(blockGCTimeKey)
			t.Reset(db.timeUntil(blockGCTimeKey, blockGCInterval))
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

func (db *Lowlevel) gcBlocks() error {
	// The block GC uses a bloom filter to track used block lists. This means
	// iterating over all items, adding their block lists to the filter, then
	// iterating over the block lists and removing those that don't match the
	// filter. The filter will give false positives so we will keep around one
	// percent of block lists that we don't really need (at most).
	//
	// Block GC needs to run when there are no modifications to the FileInfos or
	// block lists.

	db.gcMut.Lock()
	defer db.gcMut.Unlock()

	t, err := db.newReadWriteTransaction()
	if err != nil {
		return err
	}
	defer t.Release()

	// Set up the bloom filter with the initial capacity and false positive
	// rate, or higher capacity if we've done this before and seen lots of block
	// lists.

	capacity := blockGCBloomCapacity
	if db.gcKeyCount > capacity {
		capacity = db.gcKeyCount
	}
	filter := bloom.NewWithEstimates(uint(capacity), blockGCBloomFalsePositiveRate)

	// Iterate the FileInfos, unmarshal the blocks hashes and add them to
	// the filter.

	it, err := db.NewPrefixIterator([]byte{KeyTypeDevice})
	if err != nil {
		return err
	}
	for it.Next() {
		var bl BlocksHashOnly
		if err := bl.Unmarshal(it.Value()); err != nil {
			return err
		}
		if len(bl.BlocksHash) > 0 {
			filter.Add(bl.BlocksHash)
		}
	}
	it.Release()
	if err := it.Error(); err != nil {
		return err
	}

	// Iterate over block lists, removing keys with hashes that don't match
	// the filter.

	it, err = db.NewPrefixIterator([]byte{KeyTypeBlockList})
	if err != nil {
		return err
	}
	matched := 0
	for it.Next() {
		key := blockListKey(it.Key())
		if filter.Test(key.BlocksHash()) {
			matched++
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
	db.gcKeyCount = matched

	if err := t.Commit(); err != nil {
		return err
	}

	return db.Compact()
}

func unmarshalVersionList(data []byte) (VersionList, bool) {
	var vl VersionList
	if err := vl.Unmarshal(data); err != nil {
		l.Debugln("unmarshal error:", err)
		return VersionList{}, false
	}
	if len(vl.Versions) == 0 {
		l.Debugln("empty version list")
		return VersionList{}, false
	}
	return vl, true
}

// unchanged checks if two files are the same and thus don't need to be updated.
// Local flags or the invalid bit might change without the version
// being bumped.
func unchanged(nf, ef FileIntf) bool {
	return ef.FileVersion().Equal(nf.FileVersion()) && ef.IsInvalid() == nf.IsInvalid() && ef.FileLocalFlags() == nf.FileLocalFlags()
}
