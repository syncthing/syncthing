// Copyright (C) 2014 Jakob Borg and other contributors. All rights reserved.
// Use of this source code is governed by an MIT-style license that can be
// found in the LICENSE file.

// Package cid provides a manager for mappings between node ID:s and connection ID:s.
package cid

import (
	"sync"

	"github.com/calmh/syncthing/protocol"
)

type Map struct {
	sync.Mutex
	toCid  map[protocol.NodeID]uint
	toName []protocol.NodeID
}

var (
	LocalNodeID      = protocol.NodeID{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
	LocalID     uint = 0
	emptyNodeID protocol.NodeID
)

func NewMap() *Map {
	return &Map{
		toCid:  map[protocol.NodeID]uint{LocalNodeID: LocalID},
		toName: []protocol.NodeID{LocalNodeID},
	}
}

func (m *Map) Get(name protocol.NodeID) uint {
	m.Lock()
	defer m.Unlock()

	cid, ok := m.toCid[name]
	if ok {
		return cid
	}

	// Find a free slot to get a new ID
	for i, n := range m.toName {
		if n == emptyNodeID {
			m.toName[i] = name
			m.toCid[name] = uint(i)
			return uint(i)
		}
	}

	// Add it to the end since we didn't find a free slot
	m.toName = append(m.toName, name)
	cid = uint(len(m.toName) - 1)
	m.toCid[name] = cid
	return cid
}

func (m *Map) Name(cid uint) protocol.NodeID {
	m.Lock()
	defer m.Unlock()

	return m.toName[cid]
}

func (m *Map) Names() []protocol.NodeID {
	m.Lock()

	var names []protocol.NodeID
	for _, name := range m.toName {
		if name != emptyNodeID {
			names = append(names, name)
		}
	}

	m.Unlock()
	return names
}

func (m *Map) Clear(name protocol.NodeID) {
	m.Lock()
	cid, ok := m.toCid[name]
	if ok {
		m.toName[cid] = emptyNodeID
		delete(m.toCid, name)
	}
	m.Unlock()
}
