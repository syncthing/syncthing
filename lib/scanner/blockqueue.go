// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package scanner

import (
	"errors"
	"path/filepath"

	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/sync"
)

// HashFile hashes the files and returns a list of blocks representing the file.
func HashFile(fs fs.Filesystem, path string, blockSize int, counter Counter, useWeakHashes bool) ([]protocol.BlockInfo, error) {
	fd, err := fs.Open(path)
	if err != nil {
		l.Debugln("open:", err)
		return nil, err
	}
	defer fd.Close()

	// Get the size and modtime of the file before we start hashing it.

	fi, err := fd.Stat()
	if err != nil {
		l.Debugln("stat before:", err)
		return nil, err
	}
	size := fi.Size()
	modTime := fi.ModTime()

	// Hash the file. This may take a while for large files.

	blocks, err := Blocks(fd, blockSize, size, counter, useWeakHashes)
	if err != nil {
		l.Debugln("blocks:", err)
		return nil, err
	}

	// Recheck the size and modtime again. If they differ, the file changed
	// while we were reading it and our hash results are invalid.

	fi, err = fd.Stat()
	if err != nil {
		l.Debugln("stat after:", err)
		return nil, err
	}
	if size != fi.Size() || !modTime.Equal(fi.ModTime()) {
		return nil, errors.New("file changed during hashing")
	}

	return blocks, nil
}

// The parallel hasher reads FileInfo structures from the inbox, hashes the
// file to populate the Blocks element and sends it to the outbox. A number of
// workers are used in parallel. The outbox will become closed when the inbox
// is closed and all items handled.
type parallelHasher struct {
	fs            fs.Filesystem
	dir           string
	blockSize     int
	workers       int
	outbox        chan<- protocol.FileInfo
	inbox         <-chan protocol.FileInfo
	counter       Counter
	done          chan<- struct{}
	cancel        <-chan struct{}
	useWeakHashes bool
	wg            sync.WaitGroup
}

func newParallelHasher(fs fs.Filesystem, dir string, blockSize, workers int, outbox chan<- protocol.FileInfo, inbox <-chan protocol.FileInfo, counter Counter, done chan<- struct{}, cancel <-chan struct{}, useWeakHashes bool) {
	ph := &parallelHasher{
		fs:            fs,
		dir:           dir,
		blockSize:     blockSize,
		workers:       workers,
		outbox:        outbox,
		inbox:         inbox,
		counter:       counter,
		done:          done,
		cancel:        cancel,
		useWeakHashes: useWeakHashes,
		wg:            sync.NewWaitGroup(),
	}

	for i := 0; i < workers; i++ {
		ph.wg.Add(1)
		go ph.hashFiles()
	}

	go ph.closeWhenDone()
}

func (ph *parallelHasher) hashFiles() {
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

			blocks, err := HashFile(ph.fs, filepath.Join(ph.dir, f.Name), ph.blockSize, ph.counter, ph.useWeakHashes)
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
			case <-ph.cancel:
				return
			}

		case <-ph.cancel:
			return
		}
	}
}

func (ph *parallelHasher) closeWhenDone() {
	ph.wg.Wait()
	if ph.done != nil {
		close(ph.done)
	}
	close(ph.outbox)
}
