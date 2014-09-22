// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// Package files provides a set type to track local/remote files with newness
// checks. We must do a certain amount of normalization in here. We will get
// fed paths with either native or wire-format separators and encodings
// depending on who calls us. We transform paths to wire-format (NFC and
// slashes) on the way to the database, and transform to native format
// (varying separator and encoding) on the way back out.
package files

import (
	"sync"

	"github.com/syncthing/syncthing/internal/lamport"
	"github.com/syncthing/syncthing/internal/protocol"
	"github.com/syndtr/goleveldb/leveldb"
)

type fileRecord struct {
	File   protocol.FileInfo
	Usage  int
	Global bool
}

type bitset uint64

type Set struct {
	localVersion map[protocol.NodeID]uint64
	mutex        sync.Mutex
	repo         string
	db           *leveldb.DB
}

func NewSet(repo string, db *leveldb.DB) *Set {
	var s = Set{
		localVersion: make(map[protocol.NodeID]uint64),
		repo:         repo,
		db:           db,
	}

	var nodeID protocol.NodeID
	ldbWithAllRepoTruncated(db, []byte(repo), func(node []byte, f protocol.FileInfoTruncated) bool {
		copy(nodeID[:], node)
		if f.LocalVersion > s.localVersion[nodeID] {
			s.localVersion[nodeID] = f.LocalVersion
		}
		lamport.Default.Tick(f.Version)
		return true
	})
	if debug {
		l.Debugf("loaded localVersion for %q: %#v", repo, s.localVersion)
	}
	clock(s.localVersion[protocol.LocalNodeID])

	return &s
}

func (s *Set) Replace(node protocol.NodeID, fs []protocol.FileInfo) {
	if debug {
		l.Debugf("%s Replace(%v, [%d])", s.repo, node, len(fs))
	}
	normalizeFilenames(fs)
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.localVersion[node] = ldbReplace(s.db, []byte(s.repo), node[:], fs)
}

func (s *Set) ReplaceWithDelete(node protocol.NodeID, fs []protocol.FileInfo) {
	if debug {
		l.Debugf("%s ReplaceWithDelete(%v, [%d])", s.repo, node, len(fs))
	}
	normalizeFilenames(fs)
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if lv := ldbReplaceWithDelete(s.db, []byte(s.repo), node[:], fs); lv > s.localVersion[node] {
		s.localVersion[node] = lv
	}
}

func (s *Set) Update(node protocol.NodeID, fs []protocol.FileInfo) {
	if debug {
		l.Debugf("%s Update(%v, [%d])", s.repo, node, len(fs))
	}
	normalizeFilenames(fs)
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if lv := ldbUpdate(s.db, []byte(s.repo), node[:], fs); lv > s.localVersion[node] {
		s.localVersion[node] = lv
	}
}

func (s *Set) WithNeed(node protocol.NodeID, fn fileIterator) {
	if debug {
		l.Debugf("%s WithNeed(%v)", s.repo, node)
	}
	ldbWithNeed(s.db, []byte(s.repo), node[:], false, nativeFileIterator(fn))
}

func (s *Set) WithNeedTruncated(node protocol.NodeID, fn fileIterator) {
	if debug {
		l.Debugf("%s WithNeedTruncated(%v)", s.repo, node)
	}
	ldbWithNeed(s.db, []byte(s.repo), node[:], true, nativeFileIterator(fn))
}

func (s *Set) WithHave(node protocol.NodeID, fn fileIterator) {
	if debug {
		l.Debugf("%s WithHave(%v)", s.repo, node)
	}
	ldbWithHave(s.db, []byte(s.repo), node[:], false, nativeFileIterator(fn))
}

func (s *Set) WithHaveTruncated(node protocol.NodeID, fn fileIterator) {
	if debug {
		l.Debugf("%s WithHaveTruncated(%v)", s.repo, node)
	}
	ldbWithHave(s.db, []byte(s.repo), node[:], true, nativeFileIterator(fn))
}

func (s *Set) WithGlobal(fn fileIterator) {
	if debug {
		l.Debugf("%s WithGlobal()", s.repo)
	}
	ldbWithGlobal(s.db, []byte(s.repo), false, nativeFileIterator(fn))
}

func (s *Set) WithGlobalTruncated(fn fileIterator) {
	if debug {
		l.Debugf("%s WithGlobalTruncated()", s.repo)
	}
	ldbWithGlobal(s.db, []byte(s.repo), true, nativeFileIterator(fn))
}

func (s *Set) Get(node protocol.NodeID, file string) protocol.FileInfo {
	f := ldbGet(s.db, []byte(s.repo), node[:], []byte(normalizedFilename(file)))
	f.Name = nativeFilename(f.Name)
	return f
}

func (s *Set) GetGlobal(file string) protocol.FileInfo {
	f := ldbGetGlobal(s.db, []byte(s.repo), []byte(normalizedFilename(file)))
	f.Name = nativeFilename(f.Name)
	return f
}

func (s *Set) Availability(file string) []protocol.NodeID {
	return ldbAvailability(s.db, []byte(s.repo), []byte(normalizedFilename(file)))
}

func (s *Set) LocalVersion(node protocol.NodeID) uint64 {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.localVersion[node]
}

// ListRepos returns the repository IDs seen in the database.
func ListRepos(db *leveldb.DB) []string {
	return ldbListRepos(db)
}

// DropRepo clears out all information related to the given repo from the
// database.
func DropRepo(db *leveldb.DB, repo string) {
	ldbDropRepo(db, []byte(repo))
}

func normalizeFilenames(fs []protocol.FileInfo) {
	for i := range fs {
		fs[i].Name = normalizedFilename(fs[i].Name)
	}
}

func nativeFileIterator(fn fileIterator) fileIterator {
	return func(fi protocol.FileIntf) bool {
		switch f := fi.(type) {
		case protocol.FileInfo:
			f.Name = nativeFilename(f.Name)
			return fn(f)
		case protocol.FileInfoTruncated:
			f.Name = nativeFilename(f.Name)
			return fn(f)
		default:
			panic("unknown interface type")
		}
	}
}
