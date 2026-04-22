// Copyright (C) 2021 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"github.com/syncthing/syncthing/lib/protocol"
	"google.golang.org/protobuf/proto"
)

// How many files to send in each Index/IndexUpdate message.
const (
	MaxBatchSizeBytes = 250 * 1024 // Aim for making index messages no larger than 250 KiB (uncompressed)
	MaxBatchSizeFiles = 1000       // Either way, don't include more files than this
)

// FileInfoBatch is a utility to do file operations on the database in suitably
// sized batches.
type FileInfoBatch struct {
	infos   []protocol.FileInfo
	size    int
	flushFn func([]protocol.FileInfo) error
	error   error
}

// NewFileInfoBatch returns a new FileInfoBatch that calls fn when it's time
// to flush. Errors from the flush function are considered non-recoverable;
// once an error is returned the flush function wil not be called again, and
// any further calls to Flush will return the same error (unless Reset is
// called).
func NewFileInfoBatch(fn func([]protocol.FileInfo) error) *FileInfoBatch {
	return &FileInfoBatch{flushFn: fn}
}

func (b *FileInfoBatch) SetFlushFunc(fn func([]protocol.FileInfo) error) {
	b.flushFn = fn
}

func (b *FileInfoBatch) Append(f protocol.FileInfo) {
	if b.error != nil {
		panic("bug: calling append on a failed batch")
	}
	if b.infos == nil {
		b.infos = make([]protocol.FileInfo, 0, MaxBatchSizeFiles)
	}
	b.infos = append(b.infos, f)
	b.size += proto.Size(f.ToWire(true))
}

func (b *FileInfoBatch) Full() bool {
	return len(b.infos) >= MaxBatchSizeFiles || b.size >= MaxBatchSizeBytes
}

func (b *FileInfoBatch) FlushIfFull() error {
	if b.error != nil {
		return b.error
	}
	if b.Full() {
		return b.Flush()
	}
	return nil
}

func (b *FileInfoBatch) Flush() error {
	if b.error != nil {
		return b.error
	}
	if len(b.infos) == 0 {
		return nil
	}
	if err := b.flushFn(b.infos); err != nil {
		b.error = err
		return err
	}
	b.Reset()
	return nil
}

func (b *FileInfoBatch) Reset() {
	b.infos = nil
	b.error = nil
	b.size = 0
}

func (b *FileInfoBatch) Size() int {
	return b.size
}
