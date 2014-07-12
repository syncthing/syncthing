// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// Package files provides a set type to track local/remote files with newness checks.
package files

import (
	"sync"

	"github.com/calmh/syncthing/protocol"
	"github.com/syndtr/goleveldb/leveldb"
)

type fileRecord struct {
	File   protocol.FileInfo
	Usage  int
	Global bool
}

type bitset uint64

type Set struct {
	changes map[protocol.NodeID]uint64
	mutex   sync.Mutex
	repo    string
	db      *leveldb.DB
}

func NewSet(repo string, db *leveldb.DB) *Set {
	var s = Set{
		changes: make(map[protocol.NodeID]uint64),
		repo:    repo,
		db:      db,
	}
	return &s
}

func (s *Set) Replace(node protocol.NodeID, fs []protocol.FileInfo) {
	if debug {
		l.Debugf("%s Replace(%v, [%d])", s.repo, node, len(fs))
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if ldbReplace(s.db, []byte(s.repo), node[:], fs) {
		s.changes[node]++
	}
}

func (s *Set) ReplaceWithDelete(node protocol.NodeID, fs []protocol.FileInfo) {
	if debug {
		l.Debugf("%s ReplaceWithDelete(%v, [%d])", s.repo, node, len(fs))
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if ldbReplaceWithDelete(s.db, []byte(s.repo), node[:], fs) {
		s.changes[node]++
	}
}

func (s *Set) Update(node protocol.NodeID, fs []protocol.FileInfo) {
	if debug {
		l.Debugf("%s Update(%v, [%d])", s.repo, node, len(fs))
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if ldbUpdate(s.db, []byte(s.repo), node[:], fs) {
		s.changes[node]++
	}
}

func (s *Set) WithNeed(node protocol.NodeID, fn fileIterator) {
	if debug {
		l.Debugf("%s Need(%v)", s.repo, node)
	}
	ldbWithNeed(s.db, []byte(s.repo), node[:], fn)
}

func (s *Set) WithHave(node protocol.NodeID, fn fileIterator) {
	if debug {
		l.Debugf("%s WithHave(%v)", s.repo, node)
	}
	ldbWithHave(s.db, []byte(s.repo), node[:], fn)
}

func (s *Set) WithGlobal(fn fileIterator) {
	if debug {
		l.Debugf("%s WithGlobal()", s.repo)
	}
	ldbWithGlobal(s.db, []byte(s.repo), fn)
}

func (s *Set) Get(node protocol.NodeID, file string) protocol.FileInfo {
	return ldbGet(s.db, []byte(s.repo), node[:], []byte(file))
}

func (s *Set) GetGlobal(file string) protocol.FileInfo {
	return ldbGetGlobal(s.db, []byte(s.repo), []byte(file))
}

func (s *Set) Availability(file string) []protocol.NodeID {
	return ldbAvailability(s.db, []byte(s.repo), []byte(file))
}

func (s *Set) Changes(node protocol.NodeID) uint64 {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.changes[node]
}
