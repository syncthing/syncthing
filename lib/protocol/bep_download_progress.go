// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package protocol

import "github.com/syncthing/syncthing/internal/gen/bep"

type FileDownloadProgressUpdateType = bep.FileDownloadProgressUpdateType

const (
	FileDownloadProgressUpdateTypeAppend = bep.FileDownloadProgressUpdateType_FILE_DOWNLOAD_PROGRESS_UPDATE_TYPE_APPEND
	FileDownloadProgressUpdateTypeForget = bep.FileDownloadProgressUpdateType_FILE_DOWNLOAD_PROGRESS_UPDATE_TYPE_FORGET
)

type DownloadProgress struct {
	Folder  string
	Updates []FileDownloadProgressUpdate
}

func (d *DownloadProgress) toWire() *bep.DownloadProgress {
	updates := make([]*bep.FileDownloadProgressUpdate, len(d.Updates))
	for i, u := range d.Updates {
		updates[i] = u.toWire()
	}
	return &bep.DownloadProgress{
		Folder:  d.Folder,
		Updates: updates,
	}
}

func downloadProgressFromWire(w *bep.DownloadProgress) *DownloadProgress {
	dp := &DownloadProgress{
		Folder:  w.Folder,
		Updates: make([]FileDownloadProgressUpdate, len(w.Updates)),
	}
	for i, u := range w.Updates {
		dp.Updates[i] = fileDownloadProgressUpdateFromWire(u)
	}
	return dp
}

type FileDownloadProgressUpdate struct {
	UpdateType   FileDownloadProgressUpdateType
	Name         string
	Version      Vector
	BlockIndexes []int
	BlockSize    int
}

func (f *FileDownloadProgressUpdate) toWire() *bep.FileDownloadProgressUpdate {
	bidxs := make([]int32, len(f.BlockIndexes))
	for i, b := range f.BlockIndexes {
		bidxs[i] = int32(b)
	}
	return &bep.FileDownloadProgressUpdate{
		UpdateType:   f.UpdateType,
		Name:         f.Name,
		Version:      f.Version.ToWire(),
		BlockIndexes: bidxs,
		BlockSize:    int32(f.BlockSize),
	}
}

func fileDownloadProgressUpdateFromWire(w *bep.FileDownloadProgressUpdate) FileDownloadProgressUpdate {
	bidxs := make([]int, len(w.BlockIndexes))
	for i, b := range w.BlockIndexes {
		bidxs[i] = int(b)
	}
	return FileDownloadProgressUpdate{
		UpdateType:   w.UpdateType,
		Name:         w.Name,
		Version:      VectorFromWire(w.Version),
		BlockIndexes: bidxs,
		BlockSize:    int(w.BlockSize),
	}
}
