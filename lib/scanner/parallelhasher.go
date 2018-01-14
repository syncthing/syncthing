// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package scanner

import (
	"context"

	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/sync"
)

// The parallel hasher reads FileInfo structures from the inbox, hashes the
// file to populate the Blocks element and sends it to the outbox. A number of
// workers are used in parallel. The outbox will become closed when the inbox
// is closed and all items handled.
type ParallelHasher struct {
	outbox     chan<- protocol.FileInfo
	inbox      <-chan protocol.FileInfo
	done       chan<- struct{}
	wg         sync.WaitGroup
	hashConfig *hashConfig
}

func newParallelHasher(hashConfig *hashConfig, outbox chan<- protocol.FileInfo, inbox <-chan protocol.FileInfo, done chan<- struct{}) *ParallelHasher {
	ph := &ParallelHasher{
		outbox:     outbox,
		inbox:      inbox,
		done:       done,
		wg:         sync.NewWaitGroup(),
		hashConfig: hashConfig,
	}

	return ph
}

func (ph *ParallelHasher) run(ctx context.Context, workers int, limiter ScannerLimiter) {
	// TODO does this need to be optimised?
	// when not a noopLimiter is in charge there is no need to spawn multiple threads
	for i := 0; i < workers; i++ {
		ph.wg.Add(1)
		go ph.hashFiles(ctx, limiter)
	}
	go ph.closeWhenDone()
}

// TODO remove parameter 'limiter'
func (ph *ParallelHasher) hashFiles(ctx context.Context, limiter ScannerLimiter) {
	defer ph.wg.Done()

	for {
		select {
		case f, ok := <-ph.inbox:
			if !ok {
				return
			}

			if f.IsDirectory() || f.IsDeleted() {
				panic("Bug. Asked to hash a directory or a deleted file.")
			}

			// TODO propagate hashconfig as whole parameter
			blocks, err := HashFile(ctx, ph.hashConfig.filesystem, f.Name, ph.hashConfig.blockSize, ph.hashConfig.counter, ph.hashConfig.useWeakHashes)
			if err != nil {
				l.Debugln("hash error:", f.Name, err)
				continue
			}

			f.Blocks = blocks

			// The size we saw when initially deciding to hash the file
			// might not have been the size it actually had when we hashed
			// it. Update the size from the block list.

			f.Size = 0
			for _, b := range blocks {
				f.Size += int64(b.Size)
			}

			select {
			case ph.outbox <- f:
			case <-ctx.Done():
				return
			}

		case <-ctx.Done():
			return
		}
	}
}

func (ph *ParallelHasher) closeWhenDone() {
	ph.wg.Wait()
	if ph.done != nil {
		close(ph.done)
	}
	close(ph.outbox)
}
