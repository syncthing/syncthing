// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

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
