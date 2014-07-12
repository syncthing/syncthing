// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"io"

	"github.com/juju/ratelimit"
)

type limitedWriter struct {
	w      io.Writer
	bucket *ratelimit.Bucket
}

func (w *limitedWriter) Write(buf []byte) (int, error) {
	if w.bucket != nil {
		w.bucket.Wait(int64(len(buf)))
	}
	return w.w.Write(buf)
}
