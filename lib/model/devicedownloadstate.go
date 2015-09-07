// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package model

import (
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/sync"
)

type deviceFolderDownloadState struct {
	mut     sync.RWMutex
	indexes map[string][]int32
	version map[string]protocol.Vector
}

func (p *deviceFolderDownloadState) Has(file string, version protocol.Vector, index int32) bool {
	p.mut.RLock()
	blockIndexes, ok := p.indexes[file]
	curVersion := p.version[file]
	p.mut.RUnlock()

	if !ok || !curVersion.Equal(version) {
		return false
	}

	for _, existingIndex := range blockIndexes {
		if existingIndex == index {
			return true
		}
	}
	return false
}

func (p *deviceFolderDownloadState) Update(updates []protocol.FileDownloadProgressUpdate) {
	// Could acquire lock in the loop to reduce contention, but we shouldn't be
	// getting many updates at a time, hence probably not worth it.
	p.mut.Lock()
	for _, update := range updates {
		localVersion, ok := p.version[update.Name]
		if update.UpdateType == protocol.UpdateTypeForget && ok && localVersion.Equal(update.Version) {
			delete(p.indexes, update.Name)
			delete(p.version, update.Name)
		} else if update.UpdateType == protocol.UpdateTypeAppend {
			curIndexes, ok := p.indexes[update.Name]
			curVersion := p.version[update.Name]
			if !ok || !curVersion.Equal(update.Version) {
				curIndexes = make([]int32, 0, len(update.BlockIndexes))
			}
			p.indexes[update.Name] = append(curIndexes, update.BlockIndexes...)
			p.version[update.Name] = update.Version
		}
	}
	p.mut.Unlock()
}

type deviceDownloadState struct {
	mut     sync.RWMutex
	folders map[string]deviceFolderDownloadState
}

func (t *deviceDownloadState) Update(folder string, updates []protocol.FileDownloadProgressUpdate) {
	t.mut.RLock()
	f, ok := t.folders[folder]
	t.mut.RUnlock()

	if !ok {
		f = deviceFolderDownloadState{
			mut:     sync.NewRWMutex(),
			indexes: make(map[string][]int32),
			version: make(map[string]protocol.Vector),
		}
		t.mut.Lock()
		t.folders[folder] = f
		t.mut.Unlock()
	}

	f.Update(updates)
}

func (t *deviceDownloadState) Has(folder, file string, version protocol.Vector, index int32) bool {
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

func newdeviceDownloadState() *deviceDownloadState {
	return &deviceDownloadState{
		mut:     sync.NewRWMutex(),
		folders: make(map[string]deviceFolderDownloadState),
	}
}
