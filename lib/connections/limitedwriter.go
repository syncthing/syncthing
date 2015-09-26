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

type LimitedWriter struct {
	writer io.Writer
	bucket *ratelimit.Bucket
}

func NewWriteLimiter(w io.Writer, b *ratelimit.Bucket) *LimitedWriter {
	return &LimitedWriter{
		writer: w,
		bucket: b,
	}
}

func (w *LimitedWriter) Write(buf []byte) (int, error) {
	if w.bucket != nil {
		w.bucket.Wait(int64(len(buf)))
	}
	return w.writer.Write(buf)
}
