// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package model

import (
	"sync"

	"github.com/syncthing/syncthing/internal/protocol"
)

// nodeActivity tracks the number of outstanding requests per node and can
// answer which node is least busy. It is safe for use from multiple
// goroutines.
type nodeActivity struct {
	act map[protocol.NodeID]int
	mut sync.Mutex
}

func newNodeActivity() *nodeActivity {
	return &nodeActivity{
		act: make(map[protocol.NodeID]int),
	}
}

func (m nodeActivity) leastBusy(availability []protocol.NodeID) protocol.NodeID {
	m.mut.Lock()
	var low int = 2<<30 - 1
	var selected protocol.NodeID
	for _, node := range availability {
		if usage := m.act[node]; usage < low {
			low = usage
			selected = node
		}
	}
	m.mut.Unlock()
	return selected
}

func (m nodeActivity) using(node protocol.NodeID) {
	m.mut.Lock()
	defer m.mut.Unlock()
	m.act[node]++
}

func (m nodeActivity) done(node protocol.NodeID) {
	m.mut.Lock()
	defer m.mut.Unlock()
	m.act[node]--
}
