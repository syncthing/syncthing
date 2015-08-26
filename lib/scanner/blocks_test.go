// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package scanner

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/syncthing/protocol"
)

var blocksTestData = []struct {
	data      []byte
	blocksize int
	hash      []string
}{
	{[]byte(""), 1024, []string{
		"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}},
	{[]byte("contents"), 1024, []string{
		"d1b2a59fbea7e20077af9f91b27e95e865061b270be03ff539ab3b73587882e8"}},
	{[]byte("contents"), 9, []string{
		"d1b2a59fbea7e20077af9f91b27e95e865061b270be03ff539ab3b73587882e8"}},
	{[]byte("contents"), 8, []string{
		"d1b2a59fbea7e20077af9f91b27e95e865061b270be03ff539ab3b73587882e8"}},
	{[]byte("contents"), 7, []string{
		"ed7002b439e9ac845f22357d822bac1444730fbdb6016d3ec9432297b9ec9f73",
		"043a718774c572bd8a25adbeb1bfcd5c0256ae11cecf9f9c3f925d0e52beaf89"},
	},
	{[]byte("contents"), 3, []string{
		"1143da2bc54c495c4be31d3868785d39ffdfd56df5668f0645d8f14d47647952",
		"e4432baa90819aaef51d2a7f8e148bf7e679610f3173752fabb4dcb2d0f418d3",
		"44ad63f60af0f6db6fdde6d5186ef78176367df261fa06be3079b6c80c8adba4"},
	},
	{[]byte("conconts"), 3, []string{
		"1143da2bc54c495c4be31d3868785d39ffdfd56df5668f0645d8f14d47647952",
		"1143da2bc54c495c4be31d3868785d39ffdfd56df5668f0645d8f14d47647952",
		"44ad63f60af0f6db6fdde6d5186ef78176367df261fa06be3079b6c80c8adba4"},
	},
	{[]byte("contenten"), 3, []string{
		"1143da2bc54c495c4be31d3868785d39ffdfd56df5668f0645d8f14d47647952",
		"e4432baa90819aaef51d2a7f8e148bf7e679610f3173752fabb4dcb2d0f418d3",
		"e4432baa90819aaef51d2a7f8e148bf7e679610f3173752fabb4dcb2d0f418d3"},
	},
}

func TestBlocks(t *testing.T) {
	for _, test := range blocksTestData {
		buf := bytes.NewBuffer(test.data)
		blocks, err := Blocks(buf, test.blocksize, 0, nil)

		if err != nil {
			t.Fatal(err)
		}

		if l := len(blocks); l != len(test.hash) {
			t.Fatalf("Incorrect number of blocks %d != %d", l, len(test.hash))
		} else {
			i := 0
			for off := int64(0); off < int64(len(test.data)); off += int64(test.blocksize) {
				if blocks[i].Offset != off {
					t.Errorf("Incorrect offset for block %d: %d != %d", i, blocks[i].Offset, off)
				}

				bs := test.blocksize
				if rem := len(test.data) - int(off); bs > rem {
					bs = rem
				}
				if int(blocks[i].Size) != bs {
					t.Errorf("Incorrect length for block %d: %d != %d", i, blocks[i].Size, bs)
				}
				if h := fmt.Sprintf("%x", blocks[i].Hash); h != test.hash[i] {
					t.Errorf("Incorrect block hash %q != %q", h, test.hash[i])
				}

				i++
			}
		}
	}
}

var diffTestData = []struct {
	a string
	b string
	s int
	d []protocol.BlockInfo
}{
	{"contents", "contents", 1024, []protocol.BlockInfo{}},
	{"", "", 1024, []protocol.BlockInfo{}},
	{"contents", "contents", 3, []protocol.BlockInfo{}},
	{"contents", "cantents", 3, []protocol.BlockInfo{{0, 3, nil}}},
	{"contents", "contants", 3, []protocol.BlockInfo{{3, 3, nil}}},
	{"contents", "cantants", 3, []protocol.BlockInfo{{0, 3, nil}, {3, 3, nil}}},
	{"contents", "", 3, []protocol.BlockInfo{{0, 0, nil}}},
	{"", "contents", 3, []protocol.BlockInfo{{0, 3, nil}, {3, 3, nil}, {6, 2, nil}}},
	{"con", "contents", 3, []protocol.BlockInfo{{3, 3, nil}, {6, 2, nil}}},
	{"contents", "con", 3, nil},
	{"contents", "cont", 3, []protocol.BlockInfo{{3, 1, nil}}},
	{"cont", "contents", 3, []protocol.BlockInfo{{3, 3, nil}, {6, 2, nil}}},
}

func TestDiff(t *testing.T) {
	for i, test := range diffTestData {
		a, _ := Blocks(bytes.NewBufferString(test.a), test.s, 0, nil)
		b, _ := Blocks(bytes.NewBufferString(test.b), test.s, 0, nil)
		_, d := BlockDiff(a, b)
		if len(d) != len(test.d) {
			t.Fatalf("Incorrect length for diff %d; %d != %d", i, len(d), len(test.d))
		} else {
			for j := range test.d {
				if d[j].Offset != test.d[j].Offset {
					t.Errorf("Incorrect offset for diff %d block %d; %d != %d", i, j, d[j].Offset, test.d[j].Offset)
				}
				if d[j].Size != test.d[j].Size {
					t.Errorf("Incorrect length for diff %d block %d; %d != %d", i, j, d[j].Size, test.d[j].Size)
				}
			}
		}
	}
}
