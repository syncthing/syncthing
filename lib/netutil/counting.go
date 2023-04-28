// Copyright (C) 2023 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package netutil

import (
	"context"
	"io"
	"sync/atomic"
	"time"
)

type Counted interface {
	BytesRead() int64
	LastRead() time.Time
	BytesWritten() int64
	LastWrite() time.Time
}

var rootCounter Counter

func RootCounter() Counted {
	return &rootCounter
}

type CountedStream interface {
	Counted
	Stream
}

type Counter struct {
	readBytes  atomic.Int64
	lastRead   atomic.Int64
	writeBytes atomic.Int64
	lastWrite  atomic.Int64
	parent     *Counter
}

func NewCounter() *Counter {
	return newCounterWithParent(&rootCounter)
}

func newCounterWithParent(parent *Counter) *Counter {
	return &Counter{
		parent: parent,
	}
}

func (c *Counter) BytesRead() int64 {
	return c.readBytes.Load()
}

func (c *Counter) BytesWritten() int64 {
	return c.writeBytes.Load()
}

func (c *Counter) LastRead() time.Time {
	return time.Unix(0, c.lastRead.Load())
}

func (c *Counter) LastWrite() time.Time {
	return time.Unix(0, c.lastWrite.Load())
}

func (c *Counter) addRead(n int) {
	c.readBytes.Add(int64(n))
	c.lastRead.Store(time.Now().UnixNano())
	if c.parent != nil {
		c.parent.addRead(n)
	}
}

func (c *Counter) addWrite(n int) {
	c.writeBytes.Add(int64(n))
	c.lastWrite.Store(time.Now().UnixNano())
	if c.parent != nil {
		c.parent.addWrite(n)
	}
}

type CountingStream struct {
	Stream
	*Counter
}

func NewCountingStream(s Stream, c *Counter) *CountingStream {
	return &CountingStream{
		Stream:  s,
		Counter: c,
	}
}

func (c *CountingStream) Read(bs []byte) (int, error) {
	n, err := c.Stream.Read(bs)
	c.Counter.addRead(n)
	return n, err
}

func (c *CountingStream) Write(bs []byte) (int, error) {
	n, err := c.Stream.Write(bs)
	c.Counter.addWrite(n)
	return n, err
}

func (c *CountingStream) CreateSubstream(ctx context.Context) (io.ReadWriteCloser, error) {
	s, err := c.Stream.CreateSubstream(ctx)
	if err != nil {
		return nil, err
	}
	return &readWriteCloser{
		Reader: NewCountingReader(s, c.Counter),
		Writer: NewCountingWriter(s, c.Counter),
		Closer: s,
	}, nil
}

func (c *CountingStream) AcceptSubstream(ctx context.Context) (io.ReadWriteCloser, error) {
	s, err := c.Stream.AcceptSubstream(ctx)
	if err != nil {
		return nil, err
	}
	return &readWriteCloser{
		Reader: NewCountingReader(s, c.Counter),
		Writer: NewCountingWriter(s, c.Counter),
		Closer: s,
	}, nil
}

type countingReader struct {
	io.Reader
	*Counter
}

func NewCountingReader(r io.Reader, c *Counter) io.Reader {
	return &countingReader{
		Reader:  r,
		Counter: c,
	}
}

func (c *countingReader) Read(bs []byte) (int, error) {
	n, err := c.Reader.Read(bs)
	c.Counter.addRead(n)
	return n, err
}

type countingWriter struct {
	io.Writer
	*Counter
}

func NewCountingWriter(w io.Writer, c *Counter) io.Writer {
	return &countingWriter{
		Writer:  w,
		Counter: c,
	}
}

func (c *countingWriter) Write(bs []byte) (int, error) {
	n, err := c.Writer.Write(bs)
	c.Counter.addWrite(n)
	return n, err
}
