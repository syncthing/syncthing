// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package scanner

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	origAdler32 "hash/adler32"
	mrand "math/rand"
	"testing"
	"testing/quick"

	rollingAdler32 "github.com/chmduquesne/rollinghash/adler32"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/sha256"
)

var blocksTestData = []struct {
	data      []byte
	blocksize int
	hash      []string
	weakhash  []uint32
}{
	{[]byte(""), 1024, []string{
		"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"},
		[]uint32{0},
	},
	{[]byte("contents"), 1024, []string{
		"d1b2a59fbea7e20077af9f91b27e95e865061b270be03ff539ab3b73587882e8"},
		[]uint32{0x0f3a036f},
	},
	{[]byte("contents"), 9, []string{
		"d1b2a59fbea7e20077af9f91b27e95e865061b270be03ff539ab3b73587882e8"},
		[]uint32{0x0f3a036f},
	},
	{[]byte("contents"), 8, []string{
		"d1b2a59fbea7e20077af9f91b27e95e865061b270be03ff539ab3b73587882e8"},
		[]uint32{0x0f3a036f},
	},
	{[]byte("contents"), 7, []string{
		"ed7002b439e9ac845f22357d822bac1444730fbdb6016d3ec9432297b9ec9f73",
		"043a718774c572bd8a25adbeb1bfcd5c0256ae11cecf9f9c3f925d0e52beaf89"},
		[]uint32{0x0bcb02fc, 0x00740074},
	},
	{[]byte("contents"), 3, []string{
		"1143da2bc54c495c4be31d3868785d39ffdfd56df5668f0645d8f14d47647952",
		"e4432baa90819aaef51d2a7f8e148bf7e679610f3173752fabb4dcb2d0f418d3",
		"44ad63f60af0f6db6fdde6d5186ef78176367df261fa06be3079b6c80c8adba4"},
		[]uint32{0x02780141, 0x02970148, 0x015d00e8},
	},
	{[]byte("conconts"), 3, []string{
		"1143da2bc54c495c4be31d3868785d39ffdfd56df5668f0645d8f14d47647952",
		"1143da2bc54c495c4be31d3868785d39ffdfd56df5668f0645d8f14d47647952",
		"44ad63f60af0f6db6fdde6d5186ef78176367df261fa06be3079b6c80c8adba4"},
		[]uint32{0x02780141, 0x02780141, 0x015d00e8},
	},
	{[]byte("contenten"), 3, []string{
		"1143da2bc54c495c4be31d3868785d39ffdfd56df5668f0645d8f14d47647952",
		"e4432baa90819aaef51d2a7f8e148bf7e679610f3173752fabb4dcb2d0f418d3",
		"e4432baa90819aaef51d2a7f8e148bf7e679610f3173752fabb4dcb2d0f418d3"},
		[]uint32{0x02780141, 0x02970148, 0x02970148},
	},
}

func TestBlocks(t *testing.T) {
	for testNo, test := range blocksTestData {
		buf := bytes.NewBuffer(test.data)
		blocks, err := Blocks(context.TODO(), buf, test.blocksize, -1, nil, true)

		if err != nil {
			t.Fatal(err)
		}

		if l := len(blocks); l != len(test.hash) {
			t.Fatalf("%d: Incorrect number of blocks %d != %d", testNo, l, len(test.hash))
		} else {
			i := 0
			for off := int64(0); off < int64(len(test.data)); off += int64(test.blocksize) {
				if blocks[i].Offset != off {
					t.Errorf("%d/%d: Incorrect offset %d != %d", testNo, i, blocks[i].Offset, off)
				}

				bs := test.blocksize
				if rem := len(test.data) - int(off); bs > rem {
					bs = rem
				}
				if int(blocks[i].Size) != bs {
					t.Errorf("%d/%d: Incorrect length %d != %d", testNo, i, blocks[i].Size, bs)
				}
				if h := fmt.Sprintf("%x", blocks[i].Hash); h != test.hash[i] {
					t.Errorf("%d/%d: Incorrect block hash %q != %q", testNo, i, h, test.hash[i])
				}
				if h := blocks[i].WeakHash; h != test.weakhash[i] {
					t.Errorf("%d/%d: Incorrect block weakhash 0x%08x != 0x%08x", testNo, i, h, test.weakhash[i])
				}

				i++
			}
		}
	}
}

func TestAdler32Variants(t *testing.T) {
	// Verify that the two adler32 functions give matching results for a few
	// different blocks of data.

	hf1 := origAdler32.New()
	hf2 := rollingAdler32.New()

	checkFn := func(data []byte) bool {
		hf1.Write(data)
		sum1 := hf1.Sum32()

		hf2.Write(data)
		sum2 := hf2.Sum32()

		hf1.Reset()
		hf2.Reset()

		// Make sure whatever we use in Validate matches too resp. this
		// tests gets adjusted if we ever switch the weak hash algo.
		return sum1 == sum2 && Validate(data, nil, sum1)
	}

	// protocol block sized data
	data := make([]byte, protocol.MinBlockSize)
	for i := 0; i < 5; i++ {
		rand.Read(data)
		if !checkFn(data) {
			t.Errorf("Hash mismatch on block sized data")
		}
	}

	// random small blocks
	if err := quick.Check(checkFn, nil); err != nil {
		t.Error(err)
	}

	// rolling should have the same result as the individual blocks
	// themselves.

	windowSize := 128

	hf3 := rollingAdler32.New()
	hf3.Write(data[:windowSize])

	for i := windowSize; i < len(data); i++ {
		if i%windowSize == 0 {
			// let the reference function catch up
			window := data[i-windowSize : i]
			hf1.Reset()
			hf1.Write(window)
			hf2.Reset()
			hf2.Write(window)

			// verify that they are in sync with the rolling function
			sum1 := hf1.Sum32()
			sum2 := hf2.Sum32()
			sum3 := hf3.Sum32()
			t.Logf("At i=%d, sum2=%08x, sum3=%08x", i, sum2, sum3)
			if sum2 != sum3 {
				t.Errorf("Mismatch after roll; i=%d, sum2=%08x, sum3=%08x", i, sum2, sum3)
				break
			}
			if sum1 != sum3 {
				t.Errorf("Mismatch after roll; i=%d, sum1=%08x, sum3=%08x", i, sum1, sum3)
				break
			}
			if !Validate(window, nil, sum1) {
				t.Errorf("Validation failure after roll; i=%d", i)
			}
		}
		hf3.Roll(data[i])
	}
}

func BenchmarkValidate(b *testing.B) {
	type block struct {
		data     []byte
		hash     [sha256.Size]byte
		weakhash uint32
	}
	var blocks []block
	const blocksPerType = 100

	r := mrand.New(mrand.NewSource(0x136bea689e851))

	// Valid blocks.
	for i := 0; i < blocksPerType; i++ {
		var b block
		b.data = make([]byte, 128<<10)
		r.Read(b.data)
		b.hash = sha256.Sum256(b.data)
		b.weakhash = origAdler32.Checksum(b.data)
		blocks = append(blocks, b)
	}
	// Blocks where the hash matches, but the weakhash doesn't.
	for i := 0; i < blocksPerType; i++ {
		var b block
		b.data = make([]byte, 128<<10)
		r.Read(b.data)
		b.hash = sha256.Sum256(b.data)
		b.weakhash = 1 // Zeros causes Validate to skip the weakhash.
		blocks = append(blocks, b)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for _, b := range blocks {
			Validate(b.data, b.hash[:], b.weakhash)
		}
	}
}
