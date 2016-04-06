// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package model

import (
	"time"

	"github.com/syncthing/syncthing/lib/protocol"
)

// The databaseUpdater performs database updates in batches based on
// Progresser updates.
type databaseUpdater struct {
	folder           string
	model            *Model
	updates          chan protocol.FileInfo
	batch            []protocol.FileInfo
	lastReceivedFile protocol.FileInfo
	done             chan struct{} // closed when Close() has been called and all pending operations have been committed
}

func newDatabaseUpdater(folder string, model *Model) *databaseUpdater {
	p := &databaseUpdater{
		folder:  folder,
		model:   model,
		updates: make(chan protocol.FileInfo),
		done:    make(chan struct{}),
	}
	go p.runner()
	return p
}

func (p *databaseUpdater) Started(file protocol.FileInfo) {
	// Don't care
}

func (p *databaseUpdater) Progress(file protocol.FileInfo, copied, requested, downloaded int) {
	// Don't care
}

func (p *databaseUpdater) Completed(file protocol.FileInfo, err error) {
	if err == nil {
		file.LocalVersion = 0
		p.updates <- file
	}
}

// Close stops the databaseUpdater from accepting further changes and
// awaits commit of all pending operations before returning.
func (p *databaseUpdater) Close() {
	close(p.updates)
	<-p.done
}

func (p *databaseUpdater) runner() {
	const (
		maxBatchSize = 1000
		maxBatchTime = 2 * time.Second
	)

	defer close(p.done)

	p.batch = make([]protocol.FileInfo, 0, maxBatchSize)
	nextCommit := time.NewTimer(maxBatchTime)
	defer nextCommit.Stop()

loop:
	for {
		select {
		case update, ok := <-p.updates:
			if !ok {
				break loop
			}

			if !update.IsDirectory() && !update.IsDeleted() && !update.IsInvalid() && !update.IsSymlink() {
				p.lastReceivedFile = update
			}

			p.batch = append(p.batch, update)
			if len(p.batch) == maxBatchSize {
				p.commit()
				nextCommit.Reset(maxBatchTime)
			}

		case <-nextCommit.C:
			if len(p.batch) > 0 {
				p.commit()
			}
			nextCommit.Reset(maxBatchTime)
		}
	}

	if len(p.batch) > 0 {
		p.commit()
	}
}

func (p *databaseUpdater) commit() {
	if shouldDebug() {
		l.Debugln("databaseUpdater.commit() committing batch of size", len(p.batch))
	}
	p.model.updateLocals(p.folder, p.batch)
	if p.lastReceivedFile.Name != "" {
		p.model.receivedFile(p.folder, p.lastReceivedFile)
		p.lastReceivedFile = protocol.FileInfo{}
	}

	for i := range p.batch {
		// Clear out the existing structures to the garbage collector can free
		// their block lists and so on.
		p.batch[i] = protocol.FileInfo{}
	}
	p.batch = p.batch[:0]
}
