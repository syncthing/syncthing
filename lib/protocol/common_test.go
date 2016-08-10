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
	fromTemporary bool
	closedCh      chan struct{}
	closedErr     error
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

func (t *TestModel) Request(deviceID DeviceID, folder, name string, offset int64, hash []byte, fromTemporary bool, buf []byte) error {
	t.folder = folder
	t.name = name
	t.offset = offset
	t.size = len(buf)
	t.hash = hash
	t.fromTemporary = fromTemporary
	copy(buf, t.data)
	return nil
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
