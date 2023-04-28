// Copyright (C) 2023 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package netutil

import (
	"context"
	"io"
)

type Limiter interface {
	Unlimited() bool
	Take(int)
	Limit() int // events per second
	Burst() int // maximum number of events
}

type limitedWriter struct {
	writer io.Writer
	Limiter
}

func NewLimitedWriter(w io.Writer, limiter Limiter) io.Writer {
	return &limitedWriter{
		writer:  w,
		Limiter: limiter,
	}
}

// limitedWriter is a rate limited io.Writer
func (w *limitedWriter) Write(buf []byte) (int, error) {
	if w.Unlimited() {
		return w.writer.Write(buf)
	}

	// This does (potentially) multiple smaller writes in order to be less
	// bursty with large writes and slow rates. At the same time we don't
	// want to do hilarious amounts of tiny writes when the rate is high, so
	// try to be a bit adaptable. We range from the minimum write size of 1
	// KiB up to the limiter burst size, aiming for about a write every
	// 10ms.
	singleWriteSize := w.Limiter.Limit() / 100              // 10ms worth of data
	singleWriteSize = ((singleWriteSize / 1024) + 1) * 1024 // round up to the next kibibyte
	if burst := w.Limiter.Burst(); singleWriteSize > burst {
		singleWriteSize = burst
	}

	written := 0
	for written < len(buf) {
		toWrite := singleWriteSize
		if toWrite > len(buf)-written {
			toWrite = len(buf) - written
		}
		w.Take(toWrite)
		n, err := w.writer.Write(buf[written : written+toWrite])
		written += n
		if err != nil {
			return written, err
		}
	}

	return written, nil
}

// limitedReader is a rate limited io.Reader
type limitedReader struct {
	reader io.Reader
	Limiter
}

func NewLimitedReader(r io.Reader, limiter Limiter) io.Reader {
	return &limitedReader{
		reader:  r,
		Limiter: limiter,
	}
}

func (r *limitedReader) Read(buf []byte) (int, error) {
	n, err := r.reader.Read(buf)
	if !r.Unlimited() {
		r.Take(n)
	}
	return n, err
}

type limitedStream struct {
	Stream
	rlim Limiter
	wlim Limiter
	r    *limitedReader
	w    *limitedWriter
}

func NewLimitedStream(s Stream, rlim, wlim Limiter) Stream {
	return &limitedStream{
		Stream: s,
		rlim:   rlim,
		wlim:   wlim,
		r:      &limitedReader{reader: s, Limiter: rlim},
		w:      &limitedWriter{writer: s, Limiter: wlim},
	}
}

func (c *limitedStream) Read(bs []byte) (int, error) {
	return c.r.Read(bs)
}

func (c *limitedStream) Write(bs []byte) (int, error) {
	return c.w.Write(bs)
}

func (c *limitedStream) CreateSubstream(ctx context.Context) (io.ReadWriteCloser, error) {
	s, err := c.Stream.CreateSubstream(ctx)
	if err != nil {
		return nil, err
	}
	return &readWriteCloser{
		Reader: &limitedReader{reader: s, Limiter: c.rlim},
		Writer: &limitedWriter{writer: s, Limiter: c.wlim},
		Closer: s,
	}, nil
}

func (c *limitedStream) AcceptSubstream(ctx context.Context) (io.ReadWriteCloser, error) {
	s, err := c.Stream.AcceptSubstream(ctx)
	if err != nil {
		return nil, err
	}
	return &readWriteCloser{
		Reader: &limitedReader{reader: s, Limiter: c.rlim},
		Writer: &limitedWriter{writer: s, Limiter: c.wlim},
		Closer: s,
	}, nil
}
