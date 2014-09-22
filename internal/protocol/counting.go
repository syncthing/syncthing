// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package protocol

import (
	"io"
	"sync/atomic"
	"time"
)

type countingReader struct {
	io.Reader
	tot  uint64 // bytes
	last int64  // unix nanos
}

var (
	totalIncoming uint64
	totalOutgoing uint64
)

func (c *countingReader) Read(bs []byte) (int, error) {
	n, err := c.Reader.Read(bs)
	atomic.AddUint64(&c.tot, uint64(n))
	atomic.AddUint64(&totalIncoming, uint64(n))
	atomic.StoreInt64(&c.last, time.Now().UnixNano())
	return n, err
}

func (c *countingReader) Tot() uint64 {
	return atomic.LoadUint64(&c.tot)
}

func (c *countingReader) Last() time.Time {
	return time.Unix(0, atomic.LoadInt64(&c.last))
}

type countingWriter struct {
	io.Writer
	tot  uint64 // bytes
	last int64  // unix nanos
}

func (c *countingWriter) Write(bs []byte) (int, error) {
	n, err := c.Writer.Write(bs)
	atomic.AddUint64(&c.tot, uint64(n))
	atomic.AddUint64(&totalOutgoing, uint64(n))
	atomic.StoreInt64(&c.last, time.Now().UnixNano())
	return n, err
}

func (c *countingWriter) Tot() uint64 {
	return atomic.LoadUint64(&c.tot)
}

func (c *countingWriter) Last() time.Time {
	return time.Unix(0, atomic.LoadInt64(&c.last))
}

func TotalInOut() (uint64, uint64) {
	return atomic.LoadUint64(&totalIncoming), atomic.LoadUint64(&totalOutgoing)
}
