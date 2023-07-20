// Copyright (C) 2017 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package db

import (
	"bytes"
	"errors"
	"fmt"
	"math/bits"
	"time"

	"github.com/syncthing/syncthing/lib/db/backend"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/sync"
)

var errMetaInconsistent = errors.New("inconsistent counts detected")

type countsMap struct {
	counts  CountsSet
	indexes map[metaKey]int // device ID + local flags -> index in counts
}

// metadataTracker keeps metadata on a per device, per local flag basis.
type metadataTracker struct {
	keyer keyer
	countsMap
	mut      sync.RWMutex
	dirty    bool
	evLogger events.Logger
}

type metaKey struct {
	dev  protocol.DeviceID
	flag uint32
}

const needFlag uint32 = 1 << 31 // Last bit, as early ones are local flags

func newMetadataTracker(keyer keyer, evLogger events.Logger) *metadataTracker {
	return &metadataTracker{
		keyer: keyer,
		mut:   sync.NewRWMutex(),
		countsMap: countsMap{
			indexes: make(map[metaKey]int),
		},
		evLogger: evLogger,
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
		dev, err := protocol.DeviceIDFromBytes(c.DeviceID)
		if err != nil {
			return err
		}
		m.indexes[metaKey{dev, c.LocalFlags}] = i
	}
	return nil
}

// Marshal returns the protobuf representation of the metadataTracker
func (m *metadataTracker) Marshal() ([]byte, error) {
	return m.counts.Marshal()
}

func (m *metadataTracker) CommitHook(folder []byte) backend.CommitHook {
	return func(t backend.WriteTransaction) error {
		return m.toDB(t, folder)
	}
}

// toDB saves the marshalled metadataTracker to the given db, under the key
// corresponding to the given folder
func (m *metadataTracker) toDB(t backend.WriteTransaction, folder []byte) error {
	key, err := m.keyer.GenerateFolderMetaKey(nil, folder)
	if err != nil {
		return err
	}

	m.mut.RLock()
	defer m.mut.RUnlock()

	if !m.dirty {
		return nil
	}

	bs, err := m.Marshal()
	if err != nil {
		return err
	}
	err = t.Put(key, bs)
	if err == nil {
		m.dirty = false
	}

	return err
}

// fromDB initializes the metadataTracker from the marshalled data found in
// the database under the key corresponding to the given folder
func (m *metadataTracker) fromDB(db *Lowlevel, folder []byte) error {
	key, err := db.keyer.GenerateFolderMetaKey(nil, folder)
	if err != nil {
		return err
	}
	bs, err := db.Get(key)
	if err != nil {
		return err
	}
	if err = m.Unmarshal(bs); err != nil {
		return err
	}
	if m.counts.Created == 0 {
		return errMetaInconsistent
	}
	return nil
}

// countsPtr returns a pointer to the corresponding Counts struct, if
// necessary allocating one in the process
func (m *metadataTracker) countsPtr(dev protocol.DeviceID, flag uint32) *Counts {
	// must be called with the mutex held

	if bits.OnesCount32(flag) > 1 {
		panic("incorrect usage: set at most one bit in flag")
	}

	key := metaKey{dev, flag}
	idx, ok := m.indexes[key]
	if !ok {
		idx = len(m.counts.Counts)
		m.counts.Counts = append(m.counts.Counts, Counts{DeviceID: dev[:], LocalFlags: flag})
		m.indexes[key] = idx
		// Need bucket must be initialized when a device first occurs in
		// the metadatatracker, even if there's no change to the need
		// bucket itself.
		nkey := metaKey{dev, needFlag}
		if _, ok := m.indexes[nkey]; !ok {
			// Initially a new device needs everything, except deletes
			nidx := len(m.counts.Counts)
			m.counts.Counts = append(m.counts.Counts, m.allNeededCounts(dev))
			m.indexes[nkey] = nidx
		}
	}
	return &m.counts.Counts[idx]
}

// allNeeded makes sure there is a counts in case the device needs everything.
func (m *countsMap) allNeededCounts(dev protocol.DeviceID) Counts {
	counts := Counts{}
	if idx, ok := m.indexes[metaKey{protocol.GlobalDeviceID, 0}]; ok {
		counts = m.counts.Counts[idx]
		counts.Deleted = 0 // Don't need deletes if having nothing
	}
	counts.DeviceID = dev[:]
	counts.LocalFlags = needFlag
	return counts
}

// addFile adds a file to the counts, adjusting the sequence number as
// appropriate
func (m *metadataTracker) addFile(dev protocol.DeviceID, f protocol.FileIntf) {
	m.mut.Lock()
	defer m.mut.Unlock()

	m.updateSeqLocked(dev, f)

	m.updateFileLocked(dev, f, m.addFileLocked)
}

func (m *metadataTracker) updateFileLocked(dev protocol.DeviceID, f protocol.FileIntf, fn func(protocol.DeviceID, uint32, protocol.FileIntf)) {
	m.dirty = true

	if f.IsInvalid() && (f.FileLocalFlags() == 0 || dev == protocol.GlobalDeviceID) {
		// This is a remote invalid file or concern the global state.
		// In either case invalid files are not accounted.
		return
	}

	if flags := f.FileLocalFlags(); flags == 0 {
		// Account regular files in the zero-flags bucket.
		fn(dev, 0, f)
	} else {
		// Account in flag specific buckets.
		eachFlagBit(flags, func(flag uint32) {
			fn(dev, flag, f)
		})
	}
}

// emptyNeeded ensures that there is a need count for the given device and that it is empty.
func (m *metadataTracker) emptyNeeded(dev protocol.DeviceID) {
	m.mut.Lock()
	defer m.mut.Unlock()

	m.dirty = true

	empty := Counts{
		DeviceID:   dev[:],
		LocalFlags: needFlag,
	}
	key := metaKey{dev, needFlag}
	if idx, ok := m.indexes[key]; ok {
		m.counts.Counts[idx] = empty
		return
	}
	m.indexes[key] = len(m.counts.Counts)
	m.counts.Counts = append(m.counts.Counts, empty)
}

// addNeeded adds a file to the needed counts
func (m *metadataTracker) addNeeded(dev protocol.DeviceID, f protocol.FileIntf) {
	m.mut.Lock()
	defer m.mut.Unlock()

	m.dirty = true

	m.addFileLocked(dev, needFlag, f)
}

func (m *metadataTracker) Sequence(dev protocol.DeviceID) int64 {
	m.mut.Lock()
	defer m.mut.Unlock()
	return m.countsPtr(dev, 0).Sequence
}

func (m *metadataTracker) updateSeqLocked(dev protocol.DeviceID, f protocol.FileIntf) {
	if dev == protocol.GlobalDeviceID {
		return
	}
	if cp := m.countsPtr(dev, 0); f.SequenceNo() > cp.Sequence {
		cp.Sequence = f.SequenceNo()
	}
}

func (m *metadataTracker) addFileLocked(dev protocol.DeviceID, flag uint32, f protocol.FileIntf) {
	cp := m.countsPtr(dev, flag)

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
func (m *metadataTracker) removeFile(dev protocol.DeviceID, f protocol.FileIntf) {
	m.mut.Lock()
	defer m.mut.Unlock()

	m.updateFileLocked(dev, f, m.removeFileLocked)
}

// removeNeeded removes a file from the needed counts
func (m *metadataTracker) removeNeeded(dev protocol.DeviceID, f protocol.FileIntf) {
	m.mut.Lock()
	defer m.mut.Unlock()

	m.dirty = true

	m.removeFileLocked(dev, needFlag, f)
}

func (m *metadataTracker) removeFileLocked(dev protocol.DeviceID, flag uint32, f protocol.FileIntf) {
	cp := m.countsPtr(dev, flag)

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
		m.evLogger.Log(events.Failure, fmt.Sprintf("meta deleted count for flag 0x%x dropped below zero", flag))
		cp.Deleted = 0
		m.counts.Created = 0
	}
	if cp.Files < 0 {
		m.evLogger.Log(events.Failure, fmt.Sprintf("meta files count for flag 0x%x dropped below zero", flag))
		cp.Files = 0
		m.counts.Created = 0
	}
	if cp.Directories < 0 {
		m.evLogger.Log(events.Failure, fmt.Sprintf("meta directories count for flag 0x%x dropped below zero", flag))
		cp.Directories = 0
		m.counts.Created = 0
	}
	if cp.Symlinks < 0 {
		m.evLogger.Log(events.Failure, fmt.Sprintf("meta deleted count for flag 0x%x dropped below zero", flag))
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
			if c.LocalFlags != needFlag {
				m.counts.Counts[i] = Counts{
					DeviceID:   c.DeviceID,
					LocalFlags: c.LocalFlags,
				}
			} else {
				m.counts.Counts[i] = m.allNeededCounts(dev)
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

func (m *countsMap) Counts(dev protocol.DeviceID, flag uint32) Counts {
	if bits.OnesCount32(flag) > 1 {
		panic("incorrect usage: set at most one bit in flag")
	}

	idx, ok := m.indexes[metaKey{dev, flag}]
	if !ok {
		if flag == needFlag {
			// If there's nothing about a device in the index yet,
			// it needs everything.
			return m.allNeededCounts(dev)
		}
		return Counts{}
	}

	return m.counts.Counts[idx]
}

// Snapshot returns a copy of the metadata for reading.
func (m *metadataTracker) Snapshot() *countsMap {
	m.mut.RLock()
	defer m.mut.RUnlock()

	c := &countsMap{
		counts: CountsSet{
			Counts:  make([]Counts, len(m.counts.Counts)),
			Created: m.counts.Created,
		},
		indexes: make(map[metaKey]int, len(m.indexes)),
	}
	for k, v := range m.indexes {
		c.indexes[k] = v
	}
	copy(c.counts.Counts, m.counts.Counts)

	return c
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
	m.mut.RLock()
	defer m.mut.RUnlock()
	return m.countsMap.devices()
}

func (m *countsMap) devices() []protocol.DeviceID {
	devs := make([]protocol.DeviceID, 0, len(m.counts.Counts))

	for _, dev := range m.counts.Counts {
		if dev.Sequence > 0 {
			id, err := protocol.DeviceIDFromBytes(dev.DeviceID)
			if err != nil {
				panic(err)
			}
			if id == protocol.GlobalDeviceID || id == protocol.LocalDeviceID {
				continue
			}
			devs = append(devs, id)
		}
	}

	return devs
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
