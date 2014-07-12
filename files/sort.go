package files

import (
	"sort"
	"github.com/calmh/syncthing/protocol"
)

type SortBy func(p protocol.FileInfo) int

func (by SortBy) Sort(files []protocol.FileInfo) {
	ps := &fileSorter{
		files: files,
		by:    by,
	}
	sort.Sort(ps)
}

type fileSorter struct {
	files []protocol.FileInfo
	by    func(p1 protocol.FileInfo) int
}

func (s *fileSorter) Len() int {
	return len(s.files)
}

func (s *fileSorter) Swap(i, j int) {
	s.files[i], s.files[j] = s.files[j], s.files[i]
}

func (s *fileSorter) Less(i, j int) bool {
	return s.by(s.files[i]) > s.by(s.files[j])
}
