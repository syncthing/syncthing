// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"slices"
	"sync"

	"github.com/syncthing/syncthing/lib/protocol"
)

// deviceFolderFileDownloadState holds current download state of a file that
// a remote device has advertised. blockIndexes represents indexes within
// FileInfo.Blocks that the remote device already has, and version represents
// the version of the file that the remote device is downloading.
type deviceFolderFileDownloadState struct {
	blockIndexes []int
	version      protocol.Vector
	blockSize    int
}

// deviceFolderDownloadState holds current download state of all files that
// a remote device is currently downloading in a specific folder.
type deviceFolderDownloadState struct {
	mut   sync.RWMutex
	files map[string]deviceFolderFileDownloadState
}

// Has returns whether a block at that specific index, and that specific version of the file
// is currently available on the remote device for pulling from a temporary file.
func (p *deviceFolderDownloadState) Has(file string, version protocol.Vector, index int) bool {
	p.mut.RLock()
	defer p.mut.RUnlock()

	local, ok := p.files[file]

	if !ok || !local.version.Equal(version) {
		return false
	}

	return slices.Contains(local.blockIndexes, index)
}

// Update updates internal state of what has been downloaded into the temporary
// files by the remote device for this specific folder.
func (p *deviceFolderDownloadState) Update(updates []protocol.FileDownloadProgressUpdate) {
	p.mut.Lock()
	defer p.mut.Unlock()

	for _, update := range updates {
		local, ok := p.files[update.Name]
		if update.UpdateType == protocol.FileDownloadProgressUpdateTypeForget && ok && local.version.Equal(update.Version) {
			delete(p.files, update.Name)
		} else if update.UpdateType == protocol.FileDownloadProgressUpdateTypeAppend {
			switch {
			case !ok:
				local = deviceFolderFileDownloadState{
					blockIndexes: update.BlockIndexes,
					version:      update.Version,
					blockSize:    update.BlockSize,
				}
			case !local.version.Equal(update.Version):
				local.blockIndexes = append(local.blockIndexes[:0], update.BlockIndexes...)
				local.version = update.Version
				local.blockSize = update.BlockSize
			default:
				local.blockIndexes = append(local.blockIndexes, update.BlockIndexes...)
			}
			p.files[update.Name] = local
		}
	}
}

func (p *deviceFolderDownloadState) BytesDownloaded() int64 {
	p.mut.RLock()
	defer p.mut.RUnlock()

	var res int64
	for _, state := range p.files {
		// BlockSize is a new field introduced in 1.4.1, thus a fallback
		// is required (will potentially underrepresent downloaded bytes).
		if state.blockSize != 0 {
			res += int64(len(state.blockIndexes) * state.blockSize)
		} else {
			res += int64(len(state.blockIndexes) * protocol.MinBlockSize)
		}
	}
	return res
}

// GetBlockCounts returns a map filename -> number of blocks downloaded.
func (p *deviceFolderDownloadState) GetBlockCounts() map[string]int {
	p.mut.RLock()
	res := make(map[string]int, len(p.files))
	for name, state := range p.files {
		res[name] = len(state.blockIndexes)
	}
	p.mut.RUnlock()
	return res
}

// deviceDownloadState represents the state of all in progress downloads
// for all folders of a specific device.
type deviceDownloadState struct {
	mut     sync.RWMutex
	folders map[string]*deviceFolderDownloadState
}

// Update updates internal state of what has been downloaded into the temporary
// files by the remote device for this specific folder.
func (t *deviceDownloadState) Update(folder string, updates []protocol.FileDownloadProgressUpdate) {
	if t == nil {
		return
	}
	t.mut.RLock()
	f, ok := t.folders[folder]
	t.mut.RUnlock()

	if !ok {
		f = &deviceFolderDownloadState{
			files: make(map[string]deviceFolderFileDownloadState),
		}
		t.mut.Lock()
		t.folders[folder] = f
		t.mut.Unlock()
	}

	f.Update(updates)
}

// Has returns whether block at that specific index, and that specific version of the file
// is currently available on the remote device for pulling from a temporary file.
func (t *deviceDownloadState) Has(folder, file string, version protocol.Vector, index int) bool {
	if t == nil {
		return false
	}
	t.mut.RLock()
	f, ok := t.folders[folder]
	t.mut.RUnlock()

	if !ok {
		return false
	}

	return f.Has(file, version, index)
}

// GetBlockCounts returns a map filename -> number of blocks downloaded for the
// given folder.
func (t *deviceDownloadState) GetBlockCounts(folder string) map[string]int {
	if t == nil {
		return nil
	}

	t.mut.RLock()
	defer t.mut.RUnlock()

	for name, state := range t.folders {
		if name == folder {
			return state.GetBlockCounts()
		}
	}
	return nil
}

func (t *deviceDownloadState) BytesDownloaded(folder string) int64 {
	if t == nil {
		return 0
	}

	t.mut.RLock()
	defer t.mut.RUnlock()

	for name, state := range t.folders {
		if name == folder {
			return state.BytesDownloaded()
		}
	}
	return 0
}

func newDeviceDownloadState() *deviceDownloadState {
	return &deviceDownloadState{
		folders: make(map[string]*deviceFolderDownloadState),
	}
}
