// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package protocol

import (
	"time"
)

type TestModel struct {
	data          []byte
	folder        string
	name          string
	offset        int64
	size          int32
	hash          []byte
	weakHash      uint32
	fromTemporary bool
	indexFn       func(string, []FileInfo)
	ccFn          func(*ClusterConfig)
	closedCh      chan struct{}
	closedErr     error
}

func newTestModel() *TestModel {
	return &TestModel{
		closedCh: make(chan struct{}),
	}
}

func (t *TestModel) Index(_ Connection, idx *Index) error {
	if t.indexFn != nil {
		t.indexFn(idx.Folder, idx.Files)
	}
	return nil
}

func (*TestModel) IndexUpdate(Connection, *IndexUpdate) error {
	return nil
}

func (t *TestModel) Request(_ Connection, req *Request) (RequestResponse, error) {
	t.folder = req.Folder
	t.name = req.Name
	t.offset = req.Offset
	t.size = int32(req.Size)
	t.hash = req.Hash
	t.weakHash = req.WeakHash
	t.fromTemporary = req.FromTemporary
	buf := make([]byte, len(t.data))
	copy(buf, t.data)
	return &fakeRequestResponse{buf}, nil
}

func (t *TestModel) Closed(_ Connection, err error) {
	t.closedErr = err
	close(t.closedCh)
}

func (t *TestModel) ClusterConfig(_ Connection, config *ClusterConfig) error {
	if t.ccFn != nil {
		t.ccFn(config)
	}
	return nil
}

func (*TestModel) DownloadProgress(Connection, *DownloadProgress) error {
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
