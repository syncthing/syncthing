// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package connections

import (
	"io"

	"github.com/juju/ratelimit"
)

type LimitedReader struct {
	reader io.Reader
	bucket *ratelimit.Bucket
}

func NewReadLimiter(r io.Reader, b *ratelimit.Bucket) *LimitedReader {
	return &LimitedReader{
		reader: r,
		bucket: b,
	}
}

func (r *LimitedReader) Read(buf []byte) (int, error) {
	n, err := r.reader.Read(buf)
	if r.bucket != nil {
		r.bucket.Wait(int64(n))
	}
	return n, err
}
