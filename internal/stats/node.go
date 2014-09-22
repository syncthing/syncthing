// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package stats

import (
	"time"

	"github.com/syncthing/syncthing/internal/protocol"
	"github.com/syndtr/goleveldb/leveldb"
)

const (
	nodeStatisticTypeLastSeen = iota
)

var nodeStatisticsTypes = []byte{
	nodeStatisticTypeLastSeen,
}

type NodeStatistics struct {
	LastSeen time.Time
}

type NodeStatisticsReference struct {
	db   *leveldb.DB
	node protocol.NodeID
}

func NewNodeStatisticsReference(db *leveldb.DB, node protocol.NodeID) *NodeStatisticsReference {
	return &NodeStatisticsReference{
		db:   db,
		node: node,
	}
}

func (s *NodeStatisticsReference) key(stat byte) []byte {
	k := make([]byte, 1+1+32)
	k[0] = keyTypeNodeStatistic
	k[1] = stat
	copy(k[1+1:], s.node[:])
	return k
}

func (s *NodeStatisticsReference) GetLastSeen() time.Time {
	value, err := s.db.Get(s.key(nodeStatisticTypeLastSeen), nil)
	if err != nil {
		if err != leveldb.ErrNotFound {
			l.Warnln("NodeStatisticsReference: Failed loading last seen value for", s.node, ":", err)
		}
		return time.Unix(0, 0)
	}

	rtime := time.Time{}
	err = rtime.UnmarshalBinary(value)
	if err != nil {
		l.Warnln("NodeStatisticsReference: Failed parsing last seen value for", s.node, ":", err)
		return time.Unix(0, 0)
	}
	if debug {
		l.Debugln("stats.NodeStatisticsReference.GetLastSeen:", s.node, rtime)
	}
	return rtime
}

func (s *NodeStatisticsReference) WasSeen() {
	if debug {
		l.Debugln("stats.NodeStatisticsReference.WasSeen:", s.node)
	}
	value, err := time.Now().MarshalBinary()
	if err != nil {
		l.Warnln("NodeStatisticsReference: Failed serializing last seen value for", s.node, ":", err)
		return
	}

	err = s.db.Put(s.key(nodeStatisticTypeLastSeen), value, nil)
	if err != nil {
		l.Warnln("Failed serializing last seen value for", s.node, ":", err)
	}
}

// Never called, maybe because it's worth while to keep the data
// or maybe because we have no easy way of knowing that a node has been removed.
func (s *NodeStatisticsReference) Delete() error {
	for _, stype := range nodeStatisticsTypes {
		err := s.db.Delete(s.key(stype), nil)
		if debug && err == nil {
			l.Debugln("stats.NodeStatisticsReference.Delete:", s.node, stype)
		}
		if err != nil && err != leveldb.ErrNotFound {
			return err
		}
	}
	return nil
}

func (s *NodeStatisticsReference) GetStatistics() NodeStatistics {
	return NodeStatistics{
		LastSeen: s.GetLastSeen(),
	}
}
