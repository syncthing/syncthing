// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package protocol

import (
	"io"
	"time"
)

type TestModel struct {
	data     []byte
	repo     string
	name     string
	offset   int64
	size     int
	closedCh chan bool
}

func newTestModel() *TestModel {
	return &TestModel{
		closedCh: make(chan bool),
	}
}

func (t *TestModel) Index(nodeID NodeID, repo string, files []FileInfo) {
}

func (t *TestModel) IndexUpdate(nodeID NodeID, repo string, files []FileInfo) {
}

func (t *TestModel) Request(nodeID NodeID, repo, name string, offset int64, size int) ([]byte, error) {
	t.repo = repo
	t.name = name
	t.offset = offset
	t.size = size
	return t.data, nil
}

func (t *TestModel) Close(nodeID NodeID, err error) {
	close(t.closedCh)
}

func (t *TestModel) ClusterConfig(nodeID NodeID, config ClusterConfigMessage) {
}

func (t *TestModel) isClosed() bool {
	select {
	case <-t.closedCh:
		return true
	case <-time.After(1 * time.Second):
		return false // Timeout
	}
}

type ErrPipe struct {
	io.PipeWriter
	written int
	max     int
	err     error
	closed  bool
}

func (e *ErrPipe) Write(data []byte) (int, error) {
	if e.closed {
		return 0, e.err
	}
	if e.written+len(data) > e.max {
		n, _ := e.PipeWriter.Write(data[:e.max-e.written])
		e.PipeWriter.CloseWithError(e.err)
		e.closed = true
		return n, e.err
	}
	return e.PipeWriter.Write(data)
}
