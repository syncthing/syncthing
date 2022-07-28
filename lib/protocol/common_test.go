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
	ccFn          func(DeviceID, ClusterConfig)
	closedCh      chan struct{}
	closedErr     error
}

func newTestModel() *TestModel {
	return &TestModel{
		closedCh: make(chan struct{}),
	}
}

func (t *TestModel) Index(deviceID DeviceID, folder string, files []FileInfo) error {
	if t.indexFn != nil {
		t.indexFn(deviceID, folder, files)
	}
	return nil
}

func (*TestModel) IndexUpdate(_ DeviceID, _ string, _ []FileInfo) error {
	return nil
}

func (t *TestModel) Request(_ DeviceID, folder, name string, _, size int32, offset int64, hash []byte, weakHash uint32, fromTemporary bool) (RequestResponse, error) {
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

func (t *TestModel) Closed(_ DeviceID, err error) {
	t.closedErr = err
	close(t.closedCh)
}

func (t *TestModel) ClusterConfig(deviceID DeviceID, config ClusterConfig) error {
	if t.ccFn != nil {
		t.ccFn(deviceID, config)
	}
	return nil
}

func (*TestModel) DownloadProgress(DeviceID, string, []FileDownloadProgressUpdate) error {
	return nil
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

func (*fakeRequestResponse) Close() {}

func (*fakeRequestResponse) Wait() {}
