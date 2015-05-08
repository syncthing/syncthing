// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package db

import (
	"math/rand"
	"testing"

	"github.com/syncthing/protocol"
)

func genBlocks(n int) []protocol.BlockInfo {
	b := make([]protocol.BlockInfo, n)
	for i := range b {
		h := make([]byte, 32)
		for i := 0; i < len(h)/4; i += 4 {
			r := rand.Uint32()
			h[i*4] = byte(r)
			h[i*4+1] = byte(r >> 8)
			h[i*4+2] = byte(r >> 16)
			h[i*4+3] = byte(r >> 24)
		}
		b[i].Hash = h

		if i == n-1 {
			b[i].Size = 1234
		} else {
			b[i].Size = protocol.BlockSize
		}
	}
	return b
}

var f1, f2, f3 protocol.FileInfo

func init() {
	blocks := genBlocks(30)

	f1 = protocol.FileInfo{
		Name:   "f1",
		Blocks: blocks[:10],
	}

	f2 = protocol.FileInfo{
		Name:   "f2",
		Blocks: blocks[10:20],
	}

	f3 = protocol.FileInfo{
		Name:   "f3",
		Blocks: blocks[20:],
	}
}

func TestBlockMapAddUpdateWipe(t *testing.T) {
	m := NewBlockMap()

	f3.Flags |= protocol.FlagDirectory

	m.Add([]protocol.FileInfo{f1, f2, f3})

	m.Iterate(f1.Blocks[0].Hash, func(file string, index int) bool {
		if file != "f1" || index != 0 {
			t.Fatal("Mismatch")
		}
		return true
	})

	m.Iterate(f2.Blocks[0].Hash, func(file string, index int) bool {
		if file != "f2" || index != 0 {
			t.Fatal("Mismatch")
		}
		return true
	})

	m.Iterate(f3.Blocks[0].Hash, func(file string, index int) bool {
		t.Fatal("Unexpected block")
		return true
	})

	f3.Flags = f1.Flags
	f1.Flags |= protocol.FlagDeleted
	f2.Flags |= protocol.FlagInvalid

	// Should remove
	m.Update([]protocol.FileInfo{f1, f2, f3})

	m.Iterate(f1.Blocks[0].Hash, func(file string, index int) bool {
		t.Fatal("Unexpected block")
		return false
	})

	m.Iterate(f2.Blocks[0].Hash, func(file string, index int) bool {
		t.Fatal("Unexpected block")
		return false
	})

	m.Iterate(f3.Blocks[0].Hash, func(file string, index int) bool {
		if file != "f3" || index != 0 {
			t.Fatal("Mismatch")
		}
		return true
	})

	f1.Flags = 0
	f2.Flags = 0
	f3.Flags = 0
}

/*
func TestBlockFinderFix(t *testing.T) {
	db, f := setup()

	iterFn := func(folder, file string, index int) bool {
		return true
	}

	m := NewBlockMap(db, "folder1")
	err := m.Add([]protocol.FileInfo{f1})
	if err != nil {
		t.Fatal(err)
	}

	if !f.Iterate(f1.Blocks[0].Hash, iterFn) {
		t.Fatal("Block not found")
	}

	err = f.Fix("folder1", f1.Name, 0, f1.Blocks[0].Hash, f2.Blocks[0].Hash)
	if err != nil {
		t.Fatal(err)
	}

	if f.Iterate(f1.Blocks[0].Hash, iterFn) {
		t.Fatal("Unexpected block")
	}

	if !f.Iterate(f2.Blocks[0].Hash, iterFn) {
		t.Fatal("Block not found")
	}
}
*/

func BenchmarkBlockMapAdd(b *testing.B) {
	m := NewBlockMap()

	f := protocol.FileInfo{
		Name:   "A moderately long filename such as would be seen when things are a few directories deep or are movie files or something",
		Blocks: genBlocks(100000), // This is a 12 GB file
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		m.Add([]protocol.FileInfo{f})
	}

	b.ReportAllocs()
}
