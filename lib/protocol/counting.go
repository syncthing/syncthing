// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package protocol

import (
	"io"
	"sync/atomic"
	"time"
)

type countingReader struct {
	io.Reader

	idString string
	tot      atomic.Int64 // bytes
	last     atomic.Int64 // unix nanos
}

var (
	totalIncoming atomic.Int64
	totalOutgoing atomic.Int64
)

func (c *countingReader) Read(bs []byte) (int, error) {
	n, err := c.Reader.Read(bs)
	c.tot.Add(int64(n))
	totalIncoming.Add(int64(n))
	c.last.Store(time.Now().UnixNano())
	metricDeviceRecvBytes.WithLabelValues(c.idString).Add(float64(n))
	return n, err
}

func (c *countingReader) Tot() int64 { return c.tot.Load() }

func (c *countingReader) Last() time.Time {
	return time.Unix(0, c.last.Load())
}

type countingWriter struct {
	io.Writer

	idString string
	tot      atomic.Int64 // bytes
	last     atomic.Int64 // unix nanos
}

func (c *countingWriter) Write(bs []byte) (int, error) {
	n, err := c.Writer.Write(bs)
	c.tot.Add(int64(n))
	totalOutgoing.Add(int64(n))
	c.last.Store(time.Now().UnixNano())
	metricDeviceSentBytes.WithLabelValues(c.idString).Add(float64(n))
	return n, err
}

func (c *countingWriter) Tot() int64 { return c.tot.Load() }

func (c *countingWriter) Last() time.Time {
	return time.Unix(0, c.last.Load())
}

func TotalInOut() (int64, int64) {
	return totalIncoming.Load(), totalOutgoing.Load()
}
