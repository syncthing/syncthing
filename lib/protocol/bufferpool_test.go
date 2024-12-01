// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package protocol

import (
	"sync"
	"testing"
	"time"

	"github.com/syncthing/syncthing/lib/rand"
)

func TestGetBucketNumbers(t *testing.T) {
	cases := []struct {
		size   int
		bkt    int
		panics bool
	}{
		{size: 1024, bkt: 0},
		{size: MinBlockSize, bkt: 0},
		{size: MinBlockSize + 1, bkt: 1},
		{size: 2*MinBlockSize - 1, bkt: 1},
		{size: 2 * MinBlockSize, bkt: 1},
		{size: 2*MinBlockSize + 1, bkt: 2},
		{size: MaxBlockSize, bkt: len(BlockSizes) - 1},
		{size: MaxBlockSize + 1, panics: true},
	}

	for _, tc := range cases {
		if tc.panics {
			shouldPanic(t, func() { getBucketForLen(tc.size) })
		} else {
			res := getBucketForLen(tc.size)
			if res != tc.bkt {
				t.Errorf("block of size %d should get from bucket %d, not %d", tc.size, tc.bkt, res)
			}
		}
	}
}

func TestPutBucketNumbers(t *testing.T) {
	cases := []struct {
		size   int
		bkt    int
		panics bool
	}{
		{size: 1024, panics: true},
		{size: MinBlockSize, bkt: 0},
		{size: MinBlockSize + 1, panics: true},
		{size: 2 * MinBlockSize, bkt: 1},
		{size: MaxBlockSize, bkt: len(BlockSizes) - 1},
		{size: MaxBlockSize + 1, panics: true},
	}

	for _, tc := range cases {
		if tc.panics {
			shouldPanic(t, func() { putBucketForCap(tc.size) })
		} else {
			res := putBucketForCap(tc.size)
			if res != tc.bkt {
				t.Errorf("block of size %d should put into bucket %d, not %d", tc.size, tc.bkt, res)
			}
		}
	}
}

func TestStressBufferPool(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	const routines = 10
	const runtime = 2 * time.Second

	bp := newBufferPool()
	t0 := time.Now()

	var wg sync.WaitGroup
	fail := make(chan struct{}, routines)
	for i := 0; i < routines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for time.Since(t0) < runtime {
				blocks := make([][]byte, 10)
				for i := range blocks {
					// Request a block of random size with the range
					// covering smaller-than-min to larger-than-max and
					// everything in between.
					want := rand.Intn(1.5 * MaxBlockSize)
					blocks[i] = bp.Get(want)
					if len(blocks[i]) != want {
						fail <- struct{}{}
						return
					}
				}
				for i := range blocks {
					bp.Put(blocks[i])
				}
			}
		}()
	}

	wg.Wait()
	select {
	case <-fail:
		t.Fatal("a block was bad size")
	default:
	}

	t.Log(bp.puts.Load(), bp.skips.Load(), bp.misses.Load(), bp.hits)
	if bp.puts.Load() == 0 || bp.skips.Load() == 0 || bp.misses.Load() == 0 {
		t.Error("didn't exercise some paths")
	}
	var hits int64
	for i := range bp.hits {
		hits += bp.hits[i].Load()
	}
	if hits == 0 {
		t.Error("didn't exercise some paths")
	}
}

func shouldPanic(t *testing.T, fn func()) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("did not panic")
		}
	}()

	fn()
}
