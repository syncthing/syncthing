// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"fmt"
	"os"
	"testing"

	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/rand"
)

func TestInMemoryIndexSorter(t *testing.T) {
	// An inMemorySorter should be able to absorb a few files in unsorted
	// order, and return them sorted.

	s := newInMemoryIndexSorter()
	addFiles(50, s)
	verifySorted(t, s, 50)
	verifyBreak(t, s, 50)
	s.Close()
}

func TestOnDiskIndexSorter(t *testing.T) {
	// An onDiskSorter should be able to absorb a few files in unsorted
	// order, and return them sorted.

	s := newOnDiskIndexSorter("testdata")
	addFiles(50, s)
	verifySorted(t, s, 50)
	verifyBreak(t, s, 50)

	// The temporary database should exist on disk. When Close()d, it should
	// be removed.

	info, err := os.Stat(s.dir)
	if err != nil {
		t.Fatal("temp database should exist on disk:", err)
	}
	if !info.IsDir() {
		t.Fatal("temp database should be a directory")
	}

	s.Close()

	_, err = os.Stat(s.dir)
	if !os.IsNotExist(err) {
		t.Fatal("temp database should have been removed")
	}
}

func TestIndexSorter(t *testing.T) {
	// An default IndexSorter should be able to absorb files, have them in
	// memory, and at some point switch to an on disk database.

	s := NewIndexSorter("testdata")
	defer s.Close()

	// We should start out as an in memory store.

	nFiles := 1
	addFiles(1, s)
	verifySorted(t, s, nFiles)

	as := s.(*autoSwitchingIndexSorter)
	if _, ok := as.internalIndexSorter.(*inMemoryIndexSorter); !ok {
		t.Fatalf("the sorter should be in memory after only one file")
	}

	// At some point, for sure with less than maxBytesInMemory files, we
	// should switch over to an on disk sorter.
	for i := 0; i < maxBytesInMemory; i++ {
		addFiles(1, s)
		nFiles++
		if _, ok := as.internalIndexSorter.(*onDiskIndexSorter); ok {
			break
		}
	}

	if _, ok := as.internalIndexSorter.(*onDiskIndexSorter); !ok {
		t.Fatalf("the sorter should be on disk after %d files", nFiles)
	}

	verifySorted(t, s, nFiles)

	// For test coverage, as some methods are called on the onDiskSorter
	// only after switching to it.

	addFiles(1, s)
	verifySorted(t, s, nFiles+1)
}

// addFiles adds files with random Sequence to the Sorter.
func addFiles(n int, s IndexSorter) {
	for i := 0; i < n; i++ {
		rnd := rand.Int63()
		f := protocol.FileInfo{
			Name:        fmt.Sprintf("file-%d", rnd),
			Size:        rand.Int63(),
			Permissions: uint32(rand.Intn(0777)),
			ModifiedS:   rand.Int63(),
			ModifiedNs:  int32(rand.Int63()),
			Sequence:    rnd,
			Version:     protocol.Vector{Counters: []protocol.Counter{{ID: 42, Value: uint64(rand.Int63())}}},
			Blocks: []protocol.BlockInfo{{
				Size: int32(rand.Intn(128 << 10)),
				Hash: []byte(rand.String(32)),
			}},
		}
		s.Append(f)
	}
}

// verifySorted checks that the files are returned sorted by Sequence.
func verifySorted(t *testing.T, s IndexSorter, expected int) {
	prevSequence := int64(-1)
	seen := 0
	s.Sorted(func(f protocol.FileInfo) bool {
		if f.Sequence <= prevSequence {
			t.Fatalf("Unsorted Sequence, %d <= %d", f.Sequence, prevSequence)
		}
		prevSequence = f.Sequence
		seen++
		return true
	})
	if seen != expected {
		t.Fatalf("expected %d files returned, got %d", expected, seen)
	}
}

// verifyBreak checks that the Sorter stops iteration once we return false.
func verifyBreak(t *testing.T, s IndexSorter, expected int) {
	prevSequence := int64(-1)
	seen := 0
	s.Sorted(func(f protocol.FileInfo) bool {
		if f.Sequence <= prevSequence {
			t.Fatalf("Unsorted Sequence, %d <= %d", f.Sequence, prevSequence)
		}
		if len(f.Blocks) != 1 {
			t.Fatalf("incorrect number of blocks %d != 1", len(f.Blocks))
		}
		if len(f.Version.Counters) != 1 {
			t.Fatalf("incorrect number of version counters %d != 1", len(f.Version.Counters))
		}
		prevSequence = f.Sequence
		seen++
		return seen < expected/2
	})
	if seen != expected/2 {
		t.Fatalf("expected %d files iterated over, got %d", expected, seen)
	}
}
