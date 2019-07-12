// Copyright (C) 2014 The Protocol Authors.

package protocol

import (
	"io"
	"sync/atomic"
	"time"
)

type countingReader struct {
	io.Reader
	tot  int64 // bytes (atomic, must remain 64-bit aligned)
	last int64 // unix nanos (atomic, must remain 64-bit aligned)
}

var (
	totalIncoming int64
	totalOutgoing int64
)

func (c *countingReader) Read(bs []byte) (int, error) {
	n, err := c.Reader.Read(bs)
	atomic.AddInt64(&c.tot, int64(n))
	atomic.AddInt64(&totalIncoming, int64(n))
	atomic.StoreInt64(&c.last, time.Now().UnixNano())
	return n, err
}

func (c *countingReader) Tot() int64 {
	return atomic.LoadInt64(&c.tot)
}

func (c *countingReader) Last() time.Time {
	return time.Unix(0, atomic.LoadInt64(&c.last))
}

type countingWriter struct {
	io.Writer
	tot  int64 // bytes (atomic, must remain 64-bit aligned)
	last int64 // unix nanos (atomic, must remain 64-bit aligned)
}

func (c *countingWriter) Write(bs []byte) (int, error) {
	n, err := c.Writer.Write(bs)
	atomic.AddInt64(&c.tot, int64(n))
	atomic.AddInt64(&totalOutgoing, int64(n))
	atomic.StoreInt64(&c.last, time.Now().UnixNano())
	return n, err
}

func (c *countingWriter) Tot() int64 {
	return atomic.LoadInt64(&c.tot)
}

func (c *countingWriter) Last() time.Time {
	return time.Unix(0, atomic.LoadInt64(&c.last))
}

func TotalInOut() (int64, int64) {
	return atomic.LoadInt64(&totalIncoming), atomic.LoadInt64(&totalOutgoing)
}
