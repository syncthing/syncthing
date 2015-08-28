package model

import (
	"encoding/binary"
	"io/ioutil"
	"os"
	"sort"

	"github.com/syncthing/protocol"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

const (
	batchTargetSize = 250 << 10 // Aim for making index messages no larger than 250 KiB (uncompressed)
	batchMaxFiles   = 1000      // Either way, don't include more files than this
	maxInMemorySize = 4 << 10   // The maximum size of an inMemoryIndexSorter before switching to a leveldb backend
)

// An indexSorter sorts FileInfos.
type indexSorter interface {
	// Adds files to the index
	Enqueue(items ...protocol.FileInfo)
	// Returns a batch, correctly sorted
	Batch() []protocol.FileInfo
	// Releases any resources used
	Close()
}

// newIndexSorter returns a new indexSorter ready for use.
func newIndexSorter() indexSorter {
	return &adaptiveIndexSorter{
		maxInMemorySize: maxInMemorySize,
	}
}

// An adaptiveIndexSorter starts out as an inMemoryIndexSorter and switches to
// being a leveldbIndexSorter when it's size exceeds maxInMemorySize.
type adaptiveIndexSorter struct {
	indexSorter
	maxInMemorySize int
}

func (s *adaptiveIndexSorter) Enqueue(items ...protocol.FileInfo) {
	if s.indexSorter == nil {
		// First time
		s.indexSorter = &inMemoryIndexSorter{}
	} else if is, ok := s.indexSorter.(*inMemoryIndexSorter); ok {
		// We have an in memory index sorter. Check if the size has been
		// exceeded, and if so switch to a database backed index sorter before
		// continuing.

		if is.size > s.maxInMemorySize {
			ds, err := newLeveldbIndexSorter()
			if err != nil {
				panic(err) // What else, at this point?
			}
			for batch := is.Batch(); len(batch) > 0; batch = is.Batch() {
				ds.Enqueue(batch...)
			}
			s.indexSorter = ds
		}
	}

	s.indexSorter.Enqueue(items...)
}

// An inMemoryIndexSort is simply a []protocol.FileInfo that makes sure to
// sort itself on LocalVersion when the user starts asking for batches.
type inMemoryIndexSorter struct {
	items   []protocol.FileInfo
	sorted  bool
	nextIdx int
	size    int
}

func (s *inMemoryIndexSorter) Enqueue(items ...protocol.FileInfo) {
	if s.sorted {
		panic("bug: enqueue on sorted indexSorter")
	}

	// Append the files to our slice, and increase the size by how much space
	// we think this consumes.

	s.items = append(s.items, items...)
	for _, f := range items {
		s.size += sizeBytes(f)
	}
}

func (s *inMemoryIndexSorter) Batch() []protocol.FileInfo {
	if !s.sorted {
		sort.Sort(sortByLocalVersion(s.items))
		s.sorted = true
	}

	if s.nextIdx >= len(s.items) {
		// nextIdx is already at the end, nothing more to return.
		return nil
	}

	// Find the end of the current batch; it needs to be smaller than
	// batchTargetSize bytes, and fewer than batchMaxFiles, and not mroe than
	// we actually have in the slice.

	end := s.nextIdx
	batchSize := 0
	for end < len(s.items) && batchSize < batchTargetSize && end-s.nextIdx < batchMaxFiles {
		batchSize += sizeBytes(s.items[end])
		end++
	}

	batch := s.items[s.nextIdx:end]
	s.nextIdx = end

	return batch
}

func (s *inMemoryIndexSorter) Close() {
	// Nothing to clean up here.
}

// A leveldbIndexSorter inserts items keyed on LocalVersion, then iterates
// over that when asked for batches.
type leveldbIndexSorter struct {
	db      *leveldb.DB
	dbPath  string
	prevKey int64
}

func newLeveldbIndexSorter() (*leveldbIndexSorter, error) {
	path, err := ioutil.TempDir("", "indexCache")
	if err != nil {
		return nil, err
	}
	db, err := leveldb.OpenFile(path, &opt.Options{
		// We're only going to read it once, so we don't need caching
		DisableBlockCache: true,
		// It should certainly not already exist
		ErrorIfExist: true,
		// We don't need sync writes on it, the data is worthless if we crash
		NoSync: true,
		// And we don't want to spend too many open files on it
		OpenFilesCacheCapacity: 16,
	})
	if err != nil {
		return nil, err
	}

	l.Debugln("leveldbIndexSorter at", path)
	return &leveldbIndexSorter{
		db:     db,
		dbPath: path,
	}, nil
}

func (s *leveldbIndexSorter) Enqueue(items ...protocol.FileInfo) {
	// Marshal each item, keyed by it's LocalVersion.
	for _, f := range items {
		var key [8]byte
		binary.BigEndian.PutUint64(key[:], uint64(f.LocalVersion))
		if err := s.db.Put(key[:], f.MustMarshalXDR(), nil); err != nil {
			panic(err)
		}
	}
}

func (s *leveldbIndexSorter) Batch() []protocol.FileInfo {
	iter := s.db.NewIterator(nil, nil)

	// We start iterating at the previous key plus one, or we'd get the last
	// item once more. (They key not existing isn't an issue, the iterator
	// will point at the next higher available one.)

	var key [8]byte
	binary.BigEndian.PutUint64(key[:], uint64(s.prevKey+1))

	// Create a batch fulfilling the usual criteria.

	var batch []protocol.FileInfo
	batchSize := 0
	for ok := iter.Seek(key[:]); ok && batchSize < batchTargetSize && len(batch) < batchMaxFiles; ok = iter.Next() {
		var f protocol.FileInfo
		if err := f.UnmarshalXDR(iter.Value()); err != nil {
			panic(err)
		}
		batch = append(batch, f)
		batchSize += sizeBytes(f)
		s.prevKey = f.LocalVersion
	}

	return batch
}

func (s *leveldbIndexSorter) Close() {
	s.db.Close()
	os.RemoveAll(s.dbPath)
}

// --- Utility functions

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

// The approximate site of a FileInfo in memory.
func sizeBytes(f protocol.FileInfo) int {
	/*
		type FileInfo struct {
			Name         string // max:8192
			Flags        uint32
			Modified     int64
			Version      Vector
			LocalVersion int64
			CachedSize   int64       // noencode (cache only)
			Blocks       []BlockInfo // max:1000000
		}
	*/
	return 8 + len(f.Name) + 4 + 8 + 8 + 16*len(f.Version) + 8 + 8 + 8 + 8 + 40*len(f.Blocks)
}
