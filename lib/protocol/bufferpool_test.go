// Copyright (C) 2019 The Protocol Authors.

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
			shouldPanic(t, func() { getBucketForSize(tc.size) })
		} else {
			res := getBucketForSize(tc.size)
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
			shouldPanic(t, func() { putBucketForSize(tc.size) })
		} else {
			res := putBucketForSize(tc.size)
			if res != tc.bkt {
				t.Errorf("block of size %d should put into bucket %d, not %d", tc.size, tc.bkt, res)
			}
		}
	}
}

func TestStressBufferPool(t *testing.T) {
	const routines = 100
	const runtime = time.Second

	bp := newBufferPool()
	t0 := time.Now()

	var wg sync.WaitGroup
	for i := 0; i < routines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for time.Since(t0) < runtime {
				blocks := make([][]byte, 100)
				for i := range blocks {
					want := rand.Intn(MaxBlockSize)
					blocks[i] = bp.Get(want)
					if len(blocks[i]) != want {
						t.Fatal("wat")
					}
				}
				for i := range blocks {
					bp.Put(blocks[i])
				}
			}
		}()
	}

	wg.Wait()
}

func shouldPanic(t *testing.T, fn func()) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("did not panic")
		}
	}()

	fn()
}
