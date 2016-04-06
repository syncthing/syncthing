// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package model

import (
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/sync"
)

// The currentTracker keep track of the files that are currently being
// processed, in the order in which they were added.

type currentTracker struct {
	current []string
	mut     sync.Mutex
}

func newCurrentTracker() *currentTracker {
	return &currentTracker{
		mut: sync.NewMutex(),
	}
}

func (p *currentTracker) Started(file protocol.FileInfo) {
	p.mut.Lock()
	p.current = append(p.current, file.Name)
	p.mut.Unlock()
}

func (p *currentTracker) Progress(file protocol.FileInfo, copied, requested, downloaded int) {
	// nothing to do
}

func (p *currentTracker) Completed(file protocol.FileInfo, err error) {
	p.mut.Lock()
	defer p.mut.Unlock()
	for i := range p.current {
		if p.current[i] == file.Name {
			copy(p.current[i:], p.current[i+1:])
			p.current = p.current[:len(p.current)-1]
			return
		}
	}
}

func (p *currentTracker) Current() []string {
	p.mut.Lock()
	defer p.mut.Unlock()
	return append([]string{}, p.current...)
}
