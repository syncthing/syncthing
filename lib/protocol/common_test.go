// Copyright (C) 2014 The Protocol Authors.

package protocol

import "time"

type TestModel struct {
	data          []byte
	folder        string
	name          string
	offset        int64
	size          int
	hash          []byte
	weakHash      uint32
	fromTemporary bool
	closedCh      chan struct{}
	closedErr     error
	conn          Connection
}

func newTestModel() *TestModel {
	return &TestModel{
		closedCh: make(chan struct{}),
	}
}

func (t *TestModel) Index(deviceID DeviceID, folder string, files []FileInfo) {
}

func (t *TestModel) IndexUpdate(deviceID DeviceID, folder string, files []FileInfo) {
}

func (t *TestModel) Request(requestID int32, deviceID DeviceID, folder, name string, size int32, offset int64, hash []byte, weakHash uint32, fromTemporary bool) {
	t.folder = folder
	t.name = name
	t.offset = offset
	t.size = int(size)
	t.hash = hash
	t.weakHash = weakHash
	t.fromTemporary = fromTemporary
	go t.conn.Response(RequestResult{
		ID:   requestID,
		Data: t.data,
		Done: make(chan struct{}),
	})
}

func (t *TestModel) Closed(conn Connection, err error) {
	t.closedErr = err
	close(t.closedCh)
}

func (t *TestModel) ClusterConfig(deviceID DeviceID, config ClusterConfig) {
}

func (t *TestModel) DownloadProgress(DeviceID, string, []FileDownloadProgressUpdate) {
}

func (t *TestModel) closedError() error {
	select {
	case <-t.closedCh:
		return t.closedErr
	case <-time.After(1 * time.Second):
		return nil // Timeout
	}
}
