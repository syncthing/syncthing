package model

import (
	"sort"

	"github.com/syncthing/protocol"
)

const (
	batchTargetSize   = 250 * 1024 // Aim for making index messages no larger than 250 KiB (uncompressed)
	batchPerFileSize  = 250        // Each FileInfo is approximately this big, in bytes, excluding BlockInfos
	batchPerBlockSize = 40         // Each BlockInfo is approximately this big
	batchMaxFiles     = 1000       // Either way, don't include more files than this
)

// An indexSorter sorts FileInfos.
type indexSorter interface {
	Enqueue(items ...protocol.FileInfo)
	Batch() []protocol.FileInfo
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

func (s *inmemoryIndexSorter) Batch() []protocol.FileInfo {
	if !s.sorted {
		sort.Sort(sortByLocalVersion(s.items))
		s.sorted = true
	}

	if s.nextIdx >= len(s.items) {
		// Nothing more to return
		return nil
	}

	end := s.nextIdx
	batchSize := 0
	for end < len(s.items) && batchSize < batchTargetSize && end-s.nextIdx < batchMaxFiles {
		batchSize += batchPerFileSize + len(s.items[end].Blocks)*batchPerBlockSize
		end++
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
