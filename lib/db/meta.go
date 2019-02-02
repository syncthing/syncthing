// Copyright (C) 2017 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package db

import (
	"bytes"
	"math/bits"
	"time"

	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/sync"
)

// metadataTracker keeps metadata on a per device, per local flag basis.
type metadataTracker struct {
	mut     sync.RWMutex
	counts  CountsSet
	indexes map[metaKey]int // device ID + local flags -> index in counts
	dirty   bool
}

type metaKey struct {
	dev   protocol.DeviceID
	flags uint32
}

func newMetadataTracker() *metadataTracker {
	return &metadataTracker{
		mut:     sync.NewRWMutex(),
		indexes: make(map[metaKey]int),
	}
}

// Unmarshal loads a metadataTracker from the corresponding protobuf
// representation
func (m *metadataTracker) Unmarshal(bs []byte) error {
	if err := m.counts.Unmarshal(bs); err != nil {
		return err
	}

	// Initialize the index map
	for i, c := range m.counts.Counts {
		m.indexes[metaKey{protocol.DeviceIDFromBytes(c.DeviceID), c.LocalFlags}] = i
	}
	return nil
}

// Unmarshal returns the protobuf representation of the metadataTracker
func (m *metadataTracker) Marshal() ([]byte, error) {
	return m.counts.Marshal()
}

// toDB saves the marshalled metadataTracker to the given db, under the key
// corresponding to the given folder
func (m *metadataTracker) toDB(db *instance, folder []byte) error {
	key := db.keyer.GenerateFolderMetaKey(nil, folder)

	m.mut.RLock()
	defer m.mut.RUnlock()

	if !m.dirty {
		return nil
	}

	bs, err := m.Marshal()
	if err != nil {
		return err
	}
	err = db.Put(key, bs, nil)
	if err == nil {
		m.dirty = false
	}

	return err
}

// fromDB initializes the metadataTracker from the marshalled data found in
// the database under the key corresponding to the given folder
func (m *metadataTracker) fromDB(db *instance, folder []byte) error {
	key := db.keyer.GenerateFolderMetaKey(nil, folder)
	bs, err := db.Get(key, nil)
	if err != nil {
		return err
	}
	return m.Unmarshal(bs)
}

// countsPtr returns a pointer to the corresponding Counts struct, if
// necessary allocating one in the process
func (m *metadataTracker) countsPtr(dev protocol.DeviceID, flags uint32) *Counts {
	// must be called with the mutex held

	key := metaKey{dev, flags}
	idx, ok := m.indexes[key]
	if !ok {
		idx = len(m.counts.Counts)
		m.counts.Counts = append(m.counts.Counts, Counts{DeviceID: dev[:], LocalFlags: flags})
		m.indexes[key] = idx
	}
	return &m.counts.Counts[idx]
}

// addFile adds a file to the counts, adjusting the sequence number as
// appropriate
func (m *metadataTracker) addFile(dev protocol.DeviceID, f FileIntf) {
	m.mut.Lock()
	defer m.mut.Unlock()

	m.dirty = true

	m.updateSeqLocked(dev, f)

	if f.IsInvalid() && f.FileLocalFlags() == 0 {
		// This is a remote invalid file; it does not count.
		return
	}

	if flags := f.FileLocalFlags(); flags == 0 {
		// Account regular files in the zero-flags bucket.
		m.addFileLocked(dev, 0, f)
	} else {
		// Account in flag specific buckets.
		eachFlagBit(flags, func(flag uint32) {
			m.addFileLocked(dev, flag, f)
		})
	}
}

func (m *metadataTracker) Sequence(dev protocol.DeviceID) int64 {
	m.mut.Lock()
	defer m.mut.Unlock()
	return m.countsPtr(dev, 0).Sequence
}

func (m *metadataTracker) updateSeqLocked(dev protocol.DeviceID, f FileIntf) {
	if dev == protocol.GlobalDeviceID {
		return
	}
	if cp := m.countsPtr(dev, 0); f.SequenceNo() > cp.Sequence {
		cp.Sequence = f.SequenceNo()
	}
}

func (m *metadataTracker) addFileLocked(dev protocol.DeviceID, flags uint32, f FileIntf) {
	cp := m.countsPtr(dev, flags)

	switch {
	case f.IsDeleted():
		cp.Deleted++
	case f.IsDirectory() && !f.IsSymlink():
		cp.Directories++
	case f.IsSymlink():
		cp.Symlinks++
	default:
		cp.Files++
	}
	cp.Bytes += f.FileSize()
}

// removeFile removes a file from the counts
func (m *metadataTracker) removeFile(dev protocol.DeviceID, f FileIntf) {
	if f.IsInvalid() && f.FileLocalFlags() == 0 {
		// This is a remote invalid file; it does not count.
		return
	}

	m.mut.Lock()
	defer m.mut.Unlock()

	m.dirty = true

	if flags := f.FileLocalFlags(); flags == 0 {
		// Remove regular files from the zero-flags bucket
		m.removeFileLocked(dev, 0, f)
	} else {
		// Remove from flag specific buckets.
		eachFlagBit(flags, func(flag uint32) {
			m.removeFileLocked(dev, flag, f)
		})
	}
}

func (m *metadataTracker) removeFileLocked(dev protocol.DeviceID, flags uint32, f FileIntf) {
	cp := m.countsPtr(dev, f.FileLocalFlags())

	switch {
	case f.IsDeleted():
		cp.Deleted--
	case f.IsDirectory() && !f.IsSymlink():
		cp.Directories--
	case f.IsSymlink():
		cp.Symlinks--
	default:
		cp.Files--
	}
	cp.Bytes -= f.FileSize()

	// If we've run into an impossible situation, correct it for now and set
	// the created timestamp to zero. Next time we start up the metadata
	// will be seen as infinitely old and recalculated from scratch.
	if cp.Deleted < 0 {
		cp.Deleted = 0
		m.counts.Created = 0
	}
	if cp.Files < 0 {
		cp.Files = 0
		m.counts.Created = 0
	}
	if cp.Directories < 0 {
		cp.Directories = 0
		m.counts.Created = 0
	}
	if cp.Symlinks < 0 {
		cp.Symlinks = 0
		m.counts.Created = 0
	}
}

// resetAll resets all metadata for the given device
func (m *metadataTracker) resetAll(dev protocol.DeviceID) {
	m.mut.Lock()
	m.dirty = true
	for i, c := range m.counts.Counts {
		if bytes.Equal(c.DeviceID, dev[:]) {
			m.counts.Counts[i] = Counts{
				DeviceID:   c.DeviceID,
				LocalFlags: c.LocalFlags,
			}
		}
	}
	m.mut.Unlock()
}

// resetCounts resets the file, dir, etc. counters, while retaining the
// sequence number
func (m *metadataTracker) resetCounts(dev protocol.DeviceID) {
	m.mut.Lock()
	m.dirty = true

	for i, c := range m.counts.Counts {
		if bytes.Equal(c.DeviceID, dev[:]) {
			m.counts.Counts[i] = Counts{
				DeviceID:   c.DeviceID,
				Sequence:   c.Sequence,
				LocalFlags: c.LocalFlags,
			}
		}
	}

	m.mut.Unlock()
}

// Counts returns the counts for the given device ID and flag. `flag` should
// be zero or have exactly one bit set.
func (m *metadataTracker) Counts(dev protocol.DeviceID, flag uint32) Counts {
	if bits.OnesCount32(flag) > 1 {
		panic("incorrect usage: set at most one bit in flag")
	}

	m.mut.RLock()
	defer m.mut.RUnlock()

	idx, ok := m.indexes[metaKey{dev, flag}]
	if !ok {
		return Counts{}
	}

	return m.counts.Counts[idx]
}

// nextLocalSeq allocates a new local sequence number
func (m *metadataTracker) nextLocalSeq() int64 {
	m.mut.Lock()
	defer m.mut.Unlock()

	c := m.countsPtr(protocol.LocalDeviceID, 0)
	c.Sequence++
	return c.Sequence
}

// devices returns the list of devices tracked, excluding the local device
// (which we don't know the ID of)
func (m *metadataTracker) devices() []protocol.DeviceID {
	devs := make(map[protocol.DeviceID]struct{}, len(m.counts.Counts))

	m.mut.RLock()
	for _, dev := range m.counts.Counts {
		if dev.Sequence > 0 {
			id := protocol.DeviceIDFromBytes(dev.DeviceID)
			if id == protocol.GlobalDeviceID || id == protocol.LocalDeviceID {
				continue
			}
			devs[id] = struct{}{}
		}
	}
	m.mut.RUnlock()

	devList := make([]protocol.DeviceID, 0, len(devs))
	for dev := range devs {
		devList = append(devList, dev)
	}

	return devList
}

func (m *metadataTracker) Created() time.Time {
	m.mut.RLock()
	defer m.mut.RUnlock()
	return time.Unix(0, m.counts.Created)
}

func (m *metadataTracker) SetCreated() {
	m.mut.Lock()
	m.counts.Created = time.Now().UnixNano()
	m.dirty = true
	m.mut.Unlock()
}

// eachFlagBit calls the function once for every bit that is set in flags
func eachFlagBit(flags uint32, fn func(flag uint32)) {
	// Test each bit from the right, as long as there are bits left in the
	// flag set. Clear any bits found and stop testing as soon as there are
	// no more bits set.

	currentBit := uint32(1 << 0)
	for flags != 0 {
		if flags&currentBit != 0 {
			fn(currentBit)
			flags &^= currentBit
		}
		currentBit <<= 1
	}
}
