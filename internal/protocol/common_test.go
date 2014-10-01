// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

package protocol

import (
	"io"
	"time"
)

type TestModel struct {
	data     []byte
	folder   string
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

func (t *TestModel) Index(deviceID DeviceID, folder string, files []FileInfo) {
}

func (t *TestModel) IndexUpdate(deviceID DeviceID, folder string, files []FileInfo) {
}

func (t *TestModel) Request(deviceID DeviceID, folder, name string, offset int64, size int) ([]byte, error) {
	t.folder = folder
	t.name = name
	t.offset = offset
	t.size = size
	return t.data, nil
}

func (t *TestModel) Close(deviceID DeviceID, err error) {
	close(t.closedCh)
}

func (t *TestModel) ClusterConfig(deviceID DeviceID, config ClusterConfigMessage) {
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
