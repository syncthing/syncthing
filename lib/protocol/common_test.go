// Copyright (C) 2014 The Protocol Authors.

package protocol

import "time"

type TestModel struct {
	data          []byte
	folder        string
	name          string
	offset        int64
	size          int32
	hash          []byte
	weakHash      uint32
	fromTemporary bool
	indexFn       func(DeviceID, string, []FileInfo)
	closedCh      chan struct{}
	closedErr     error
}

func newTestModel() *TestModel {
	return &TestModel{
		closedCh: make(chan struct{}),
	}
}

func (t *TestModel) Index(deviceID DeviceID, folder string, files []FileInfo) {
	if t.indexFn != nil {
		t.indexFn(deviceID, folder, files)
	}
}

func (t *TestModel) IndexUpdate(deviceID DeviceID, folder string, files []FileInfo) {
}

func (t *TestModel) Request(deviceID DeviceID, folder, name string, size int32, offset int64, hash []byte, weakHash uint32, fromTemporary bool) (RequestResponse, error) {
	t.folder = folder
	t.name = name
	t.offset = offset
	t.size = size
	t.hash = hash
	t.weakHash = weakHash
	t.fromTemporary = fromTemporary
	buf := make([]byte, len(t.data))
	copy(buf, t.data)
	return &fakeRequestResponse{buf}, nil
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

type fakeRequestResponse struct {
	data []byte
}

func (r *fakeRequestResponse) Data() []byte {
	return r.data
}

func (r *fakeRequestResponse) Close() {}

func (r *fakeRequestResponse) Wait() {}
