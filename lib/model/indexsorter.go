package model

import (
	"sort"

	"github.com/syncthing/protocol"
)

// An indexSorter accepts FileInfos via enqueue, then returns them sorted via
// getChunk(). It's an error to call enqueue() after getChunk() as we've
// already began returning sorted entries.
type indexSorter interface {
	Enqueue(items ...protocol.FileInfo)
	GetChunk(size int) []protocol.FileInfo
	Size() int
	Close()
}

func newIndexSorter() indexSorter {
	return &inmemoryIndexSorter{}
}

type inmemoryIndexSorter struct {
	items   []protocol.FileInfo
	sorted  bool
	nextIdx int
}

func (s *inmemoryIndexSorter) Enqueue(items ...protocol.FileInfo) {
	if s.sorted {
		panic("bug: enqueue on sorted indexSorted")
	}
	s.items = append(s.items, items...)
}

func (s *inmemoryIndexSorter) GetChunk(size int) []protocol.FileInfo {
	if !s.sorted {
		sort.Sort(sortByLocalVersion(s.items))
		s.sorted = true
	}

	if s.nextIdx >= len(s.items) {
		// Nothing more to return
		return nil
	}

	end := s.nextIdx + size
	if end > len(s.items) {
		end = len(s.items)
	}

	chunk := s.items[s.nextIdx:end]
	s.nextIdx = end

	return chunk
}

func (s *inmemoryIndexSorter) Size() int {
	return len(s.items)
}

func (s *inmemoryIndexSorter) Close() {}

type sortByLocalVersion []protocol.FileInfo

func (s sortByLocalVersion) Len() int {
	return len(s)
}
func (s sortByLocalVersion) Swap(a, b int) {
	s[a], s[b] = s[b], s[a]
}
func (s sortByLocalVersion) Less(a, b int) bool {
	return s[a].LocalVersion < s[b].LocalVersion
}
