// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"io"

	"github.com/juju/ratelimit"
)

type limitedReader struct {
	r      io.Reader
	bucket *ratelimit.Bucket
}

func (r *limitedReader) Read(buf []byte) (int, error) {
	n, err := r.r.Read(buf)
	if r.bucket != nil {
		r.bucket.Wait(int64(n))
	}
	return n, err
}
