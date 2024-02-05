// Copyright (C) 2021 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package db

import "github.com/syncthing/syncthing/lib/protocol"

// How many files to send in each Index/IndexUpdate message.
const (
	MaxBatchSizeBytes = 250 * 1024 // Aim for making index messages no larger than 250 KiB (uncompressed)
	MaxBatchSizeFiles = 1000       // Either way, don't include more files than this
)

// FileInfoBatch is a utility to do file operations on the database in suitably
// sized batches.
type FileInfoBatch struct {
	infos      []protocol.FileInfo
	size       int
	flushFn    func([]protocol.FileInfo) error
	reuseSlice bool
}

// NewReusingFileInfoBatch returns a new FileInfoBatch that calls fn when
// it's time to flush. The given slice of FileInfos is a view into the
// internal buffer which will be reused, so it must not be read after
// returning from the flush function.
func NewReusingFileInfoBatch(fn func([]protocol.FileInfo) error) *FileInfoBatch {
	return &FileInfoBatch{
		infos:      make([]protocol.FileInfo, 0, MaxBatchSizeFiles),
		flushFn:    fn,
		reuseSlice: true,
	}
}

// NewFileInfoBatch returns a new FileInfoBatch that calls fn when it's time
// to flush.
func NewFileInfoBatch(fn func([]protocol.FileInfo) error) *FileInfoBatch {
	return &FileInfoBatch{
		infos:   make([]protocol.FileInfo, 0, MaxBatchSizeFiles),
		flushFn: fn,
	}
}

func (b *FileInfoBatch) SetFlushFunc(fn func([]protocol.FileInfo) error) {
	b.flushFn = fn
}

func (b *FileInfoBatch) Append(f protocol.FileInfo) {
	b.infos = append(b.infos, f)
	b.size += f.ProtoSize()
}

func (b *FileInfoBatch) Full() bool {
	return len(b.infos) >= MaxBatchSizeFiles || b.size >= MaxBatchSizeBytes
}

func (b *FileInfoBatch) FlushIfFull() error {
	if b.Full() {
		return b.Flush()
	}
	return nil
}

func (b *FileInfoBatch) Flush() error {
	if len(b.infos) == 0 {
		return nil
	}
	if err := b.flushFn(b.infos); err != nil {
		return err
	}
	b.Reset()
	return nil
}

func (b *FileInfoBatch) Reset() {
	if b.reuseSlice {
		b.infos = b.infos[:0]
	} else {
		b.infos = make([]protocol.FileInfo, 0, len(b.infos))
	}
	b.size = 0
}

func (b *FileInfoBatch) Size() int {
	return b.size
}
