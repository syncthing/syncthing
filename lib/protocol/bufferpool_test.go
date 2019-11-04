// Copyright (C) 2019 The Protocol Authors.

package protocol

import "testing"

func TestBucketNumbers(t *testing.T) {
	cases := []struct {
		size int
		put  int
		get  int
	}{
		// Small blocks are put nowwhere, but can be fetched from bucket 0
		{size: 1024, put: -1, get: 0},

		// The blocksize for bucket zero  goes there
		{size: MinBlockSize, put: 0, get: 0},

		// Up to the next blocksize - 1 we still put in the same bucket, but
		// we look for these blocks in the next bucket where we know they
		// can be found.
		{size: 2*MinBlockSize - 1, put: 0, get: 1},

		// Next blocksize at the border...
		{size: 2 * MinBlockSize, put: 1, get: 1},

		// ... and past it.
		{size: 2*MinBlockSize + 1, put: 1, get: 2},

		// ... and past it some more. We can always put large blocks, but
		// can't guarantee getting one.
		{size: 2 * MaxBlockSize, put: len(BlockSizes) - 1, get: -1},
	}

	for _, tc := range cases {
		getRes := getBucketForSize(tc.size)
		if getRes != tc.get {
			t.Errorf("block of size %d should get from bucket %d, not %d", tc.size, tc.get, getRes)
		}
		putRes := putBucketForSize(tc.size)
		if putRes != tc.put {
			t.Errorf("block of size %d should put into bucket %d, not %d", tc.size, tc.put, putRes)
		}
	}
}
