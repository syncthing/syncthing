// Copyright (C) 2024 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model_test

import (
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/model/mocks"
	"github.com/syncthing/syncthing/lib/protocol"
	protomock "github.com/syncthing/syncthing/lib/protocol/mocks"
	"github.com/syncthing/syncthing/lib/testutil"
)

func TestIndexhandlerConcurrency(t *testing.T) {
	// Verify that sending a lot of index update messages using the
	// FileInfoBatch works and doesn't trigger the race detector.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ar, aw := io.Pipe()
	br, bw := io.Pipe()
	ci := &protomock.ConnectionInfo{}

	m1 := &mocks.Model{}
	c1 := protocol.NewConnection(protocol.EmptyDeviceID, ar, bw, testutil.NoopCloser{}, m1, ci, protocol.CompressionNever, nil, nil)
	c1.Start()
	defer c1.Close(io.EOF)

	m2 := &mocks.Model{}
	c2 := protocol.NewConnection(protocol.EmptyDeviceID, br, aw, testutil.NoopCloser{}, m2, ci, protocol.CompressionNever, nil, nil)
	c2.Start()
	defer c2.Close(io.EOF)

	c1.ClusterConfig(&protocol.ClusterConfig{})
	c2.ClusterConfig(&protocol.ClusterConfig{})
	c1.Index(ctx, &protocol.Index{Folder: "foo"})
	c2.Index(ctx, &protocol.Index{Folder: "foo"})

	const msgs = 5e2
	const files = 1e3

	recvd := 0
	m2.IndexUpdateCalls(func(_ protocol.Connection, idxUp *protocol.IndexUpdate) error {
		for j := 0; j < files; j++ {
			if n := idxUp.Files[j].Name; n != fmt.Sprintf("f%d-%d", recvd, j) {
				t.Error("wrong filename", n)
			}
		}
		recvd++
		return nil
	})

	b1 := db.NewFileInfoBatch(func(fs []protocol.FileInfo) error {
		return c1.IndexUpdate(ctx, &protocol.IndexUpdate{Folder: "foo", Files: fs})
	})
	for i := 0; i < msgs; i++ {
		for j := 0; j < files; j++ {
			b1.Append(protocol.FileInfo{
				Name:   fmt.Sprintf("f%d-%d", i, j),
				Blocks: []protocol.BlockInfo{{Hash: make([]byte, 32)}},
			})
		}
		if err := b1.Flush(); err != nil {
			t.Fatal(err)
		}
	}

	c1.Close(io.EOF)
	c2.Close(io.EOF)
	<-c1.Closed()
	<-c2.Closed()

	if recvd != msgs-1 {
		t.Error("didn't receive all expected messages")
	}
}
