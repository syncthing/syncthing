// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"bytes"
	"context"
	"sync"
	"time"

	"github.com/syncthing/syncthing/lib/protocol"
	protocolmocks "github.com/syncthing/syncthing/lib/protocol/mocks"
	"github.com/syncthing/syncthing/lib/rand"
	"github.com/syncthing/syncthing/lib/scanner"
)

type downloadProgressMessage struct {
	folder  string
	updates []protocol.FileDownloadProgressUpdate
}

func newFakeConnection(id protocol.DeviceID, model Model) *fakeConnection {
	f := &fakeConnection{
		Connection: new(protocolmocks.Connection),
		id:         id,
		model:      model,
		closed:     make(chan struct{}),
	}
	f.RequestCalls(func(ctx context.Context, req *protocol.Request) ([]byte, error) {
		return f.fileData[req.Name], nil
	})
	f.DeviceIDReturns(id)
	f.ConnectionIDReturns(rand.String(16))
	f.CloseCalls(func(err error) {
		f.closeOnce.Do(func() {
			close(f.closed)
			model.Closed(f, err)
		})
		f.ClosedReturns(f.closed)
	})
	f.StringReturns(rand.String(8))
	return f
}

type fakeConnection struct {
	*protocolmocks.Connection
	id                       protocol.DeviceID
	downloadProgressMessages []downloadProgressMessage
	files                    []protocol.FileInfo
	fileData                 map[string][]byte
	folder                   string
	model                    Model
	closed                   chan struct{}
	closeOnce                sync.Once
	mut                      sync.Mutex
}

func (f *fakeConnection) setIndexFn(fn func(_ context.Context, folder string, fs []protocol.FileInfo) error) {
	f.IndexCalls(func(ctx context.Context, idx *protocol.Index) error { return fn(ctx, idx.Folder, idx.Files) })
	f.IndexUpdateCalls(func(ctx context.Context, idxUp *protocol.IndexUpdate) error {
		return fn(ctx, idxUp.Folder, idxUp.Files)
	})
}

func (f *fakeConnection) DownloadProgress(_ context.Context, dp *protocol.DownloadProgress) {
	f.downloadProgressMessages = append(f.downloadProgressMessages, downloadProgressMessage{
		folder:  dp.Folder,
		updates: dp.Updates,
	})
}

func (f *fakeConnection) addFileLocked(name string, flags uint32, ftype protocol.FileInfoType, data []byte, version protocol.Vector, localFlags uint32) {
	blockSize := protocol.BlockSize(int64(len(data)))
	blocks, _ := scanner.Blocks(context.TODO(), bytes.NewReader(data), blockSize, int64(len(data)), nil, true)

	file := protocol.FileInfo{
		Name:       name,
		Type:       ftype,
		Version:    version,
		Sequence:   time.Now().UnixNano(),
		LocalFlags: localFlags,
	}
	switch ftype {
	case protocol.FileInfoTypeFile, protocol.FileInfoTypeDirectory:
		file.ModifiedS = time.Now().Unix()
		file.Permissions = flags
		if ftype == protocol.FileInfoTypeFile {
			file.Size = int64(len(data))
			file.RawBlockSize = int32(blockSize)
			file.Blocks = blocks
		}
	default: // Symlink
		file.Name = name
		file.Type = ftype
		file.Version = version
		file.SymlinkTarget = data
		file.NoPermissions = true
	}
	f.files = append(f.files, file)

	if f.fileData == nil {
		f.fileData = make(map[string][]byte)
	}
	f.fileData[name] = data
}

func (f *fakeConnection) addFileWithLocalFlags(name string, ftype protocol.FileInfoType, localFlags uint32) {
	f.mut.Lock()
	defer f.mut.Unlock()

	var version protocol.Vector
	version = version.Update(f.id.Short())
	f.addFileLocked(name, 0, ftype, nil, version, localFlags)
}

func (f *fakeConnection) addFile(name string, flags uint32, ftype protocol.FileInfoType, data []byte) {
	f.mut.Lock()
	defer f.mut.Unlock()

	var version protocol.Vector
	version = version.Update(f.id.Short())
	f.addFileLocked(name, flags, ftype, data, version, 0)
}

func (f *fakeConnection) updateFile(name string, flags uint32, ftype protocol.FileInfoType, data []byte) {
	f.mut.Lock()
	defer f.mut.Unlock()

	for i, fi := range f.files {
		if fi.Name == name {
			f.files = append(f.files[:i], f.files[i+1:]...)
			f.addFileLocked(name, flags, ftype, data, fi.Version.Update(f.id.Short()), 0)
			return
		}
	}
}

func (f *fakeConnection) deleteFile(name string) {
	f.mut.Lock()
	defer f.mut.Unlock()

	for i, fi := range f.files {
		if fi.Name == name {
			fi.Deleted = true
			fi.ModifiedS = time.Now().Unix()
			fi.Version = fi.Version.Update(f.id.Short())
			fi.Sequence = time.Now().UnixNano()
			fi.Blocks = nil

			f.files = append(append(f.files[:i], f.files[i+1:]...), fi)
			return
		}
	}
}

func (f *fakeConnection) sendIndexUpdate() {
	toSend := make([]protocol.FileInfo, len(f.files))
	for i := range f.files {
		toSend[i] = prepareFileInfoForIndex(f.files[i])
	}
	f.model.IndexUpdate(f, &protocol.IndexUpdate{Folder: f.folder, Files: toSend})
}

func addFakeConn(m *testModel, dev protocol.DeviceID, folderID string) *fakeConnection {
	fc := newFakeConnection(dev, m)
	fc.folder = folderID
	m.AddConnection(fc, protocol.Hello{})

	m.ClusterConfig(fc, &protocol.ClusterConfig{
		Folders: []protocol.Folder{
			{
				ID: folderID,
				Devices: []protocol.Device{
					{ID: myID},
					{ID: dev},
				},
			},
		},
	})

	return fc
}
