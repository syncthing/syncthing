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
