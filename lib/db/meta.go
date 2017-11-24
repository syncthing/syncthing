// Copyright (C) 2017 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package db

import (
	"time"

	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/sync"
)

// like protocol.LocalDeviceID but with 0xf8 in all positions
var globalDeviceID = protocol.DeviceID{0xf8, 0xf8, 0xf8, 0xf8, 0xf8, 0xf8, 0xf8, 0xf8, 0xf8, 0xf8, 0xf8, 0xf8, 0xf8, 0xf8, 0xf8, 0xf8, 0xf8, 0xf8, 0xf8, 0xf8, 0xf8, 0xf8, 0xf8, 0xf8, 0xf8, 0xf8, 0xf8, 0xf8, 0xf8, 0xf8, 0xf8, 0xf8}

type metadataTracker struct {
	mut     sync.RWMutex
	counts  CountsSet
	indexes map[protocol.DeviceID]int // device ID -> index in counts
}

func newMetadataTracker() *metadataTracker {
	return &metadataTracker{
		mut:     sync.NewRWMutex(),
		indexes: make(map[protocol.DeviceID]int),
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
		m.indexes[protocol.DeviceIDFromBytes(c.DeviceID)] = i
	}
	return nil
}

// Unmarshal returns the protobuf representation of the metadataTracker
func (m *metadataTracker) Marshal() ([]byte, error) {
	return m.counts.Marshal()
}

// toDB saves the marshalled metadataTracker to the given db, under the key
// corresponding to the given folder
func (m *metadataTracker) toDB(db *Instance, folder []byte) error {
	key := db.folderMetaKey(folder)
	bs, err := m.Marshal()
	if err != nil {
		return err
	}
	return db.Put(key, bs, nil)
}

// fromDB initializes the metadataTracker from the marshalled data found in
// the database under the key corresponding to the given folder
func (m *metadataTracker) fromDB(db *Instance, folder []byte) error {
	key := db.folderMetaKey(folder)
	bs, err := db.Get(key, nil)
	if err != nil {
		return err
	}
	return m.Unmarshal(bs)
}

// countsPtr returns a pointer to the corresponding Counts struct, if
// necessary allocating one in the process
func (m *metadataTracker) countsPtr(dev protocol.DeviceID) *Counts {
	// must be called with the mutex held

	idx, ok := m.indexes[dev]
	if !ok {
		idx = len(m.counts.Counts)
		m.counts.Counts = append(m.counts.Counts, Counts{DeviceID: dev[:]})
		m.indexes[dev] = idx
	}
	return &m.counts.Counts[idx]
}

// addFile adds a file to the counts, adjusting the sequence number as
// appropriate
func (m *metadataTracker) addFile(dev protocol.DeviceID, f FileIntf) {
	if f.IsInvalid() {
		return
	}

	m.mut.Lock()
	cp := m.countsPtr(dev)

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

	if seq := f.SequenceNo(); seq > cp.Sequence {
		cp.Sequence = seq
	}

	m.mut.Unlock()
}

// removeFile removes a file from the counts
func (m *metadataTracker) removeFile(dev protocol.DeviceID, f FileIntf) {
	if f.IsInvalid() {
		return
	}

	m.mut.Lock()
	cp := m.countsPtr(dev)

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

	if cp.Deleted < 0 || cp.Files < 0 || cp.Directories < 0 || cp.Symlinks < 0 {
		panic("bug: removed more than added")
	}

	m.mut.Unlock()
}

// resetAll resets all metadata for the given device
func (m *metadataTracker) resetAll(dev protocol.DeviceID) {
	m.mut.Lock()
	*m.countsPtr(dev) = Counts{DeviceID: dev[:]}
	m.mut.Unlock()
}

// resetCounts resets the dile, dir, etc. counters, while retaining the
// sequence number
func (m *metadataTracker) resetCounts(dev protocol.DeviceID) {
	m.mut.Lock()

	c := m.countsPtr(dev)
	c.Bytes = 0
	c.Deleted = 0
	c.Directories = 0
	c.Files = 0
	c.Symlinks = 0
	// c.Sequence deliberately untouched

	m.mut.Unlock()
}

// Size returns the counts for the given device ID
func (m *metadataTracker) Size(dev protocol.DeviceID) Counts {
	m.mut.RLock()
	defer m.mut.RUnlock()

	idx, ok := m.indexes[dev]
	if !ok {
		return Counts{}
	}

	return m.counts.Counts[idx]
}

// nextSeq allocates a new sequence number for the given device
func (m *metadataTracker) nextSeq(dev protocol.DeviceID) int64 {
	m.mut.Lock()
	defer m.mut.Unlock()

	c := m.countsPtr(dev)
	c.Sequence++
	return c.Sequence
}

// devices returns the list of devices tracked, excluding the local device
// (which we don't know the ID of)
func (m *metadataTracker) devices() []protocol.DeviceID {
	devs := make([]protocol.DeviceID, 0, len(m.counts.Counts))

	m.mut.RLock()
	for _, dev := range m.counts.Counts {
		if dev.Sequence > 0 {
			id := protocol.DeviceIDFromBytes(dev.DeviceID)
			if id == globalDeviceID || id == protocol.LocalDeviceID {
				continue
			}
			devs = append(devs, id)
		}
	}
	m.mut.RUnlock()

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
	m.mut.Unlock()
}
