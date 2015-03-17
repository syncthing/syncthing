// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package scanner

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/syncthing/protocol"
)

// The parallell hasher reads FileInfo structures from the inbox, hashes the
// file to populate the Blocks element and sends it to the outbox. A number of
// workers are used in parallel. The outbox will become closed when the inbox
// is closed and all items handled.

func newParallelHasher(dir string, blockSize, workers int, outbox, inbox chan protocol.FileInfo) {
	var wg sync.WaitGroup
	wg.Add(workers)

	for i := 0; i < workers; i++ {
		go func() {
			hashFiles(dir, blockSize, outbox, inbox)
			wg.Done()
		}()
	}

	go func() {
		wg.Wait()
		close(outbox)
	}()
}

func HashFile(path string, blockSize int) ([]protocol.BlockInfo, error) {
	fd, err := os.Open(path)
	if err != nil {
		if debug {
			l.Debugln("open:", err)
		}
		return []protocol.BlockInfo{}, err
	}

	fi, err := fd.Stat()
	if err != nil {
		fd.Close()
		if debug {
			l.Debugln("stat:", err)
		}
		return []protocol.BlockInfo{}, err
	}
	defer fd.Close()
	return Blocks(fd, blockSize, fi.Size())
}

func hashFiles(dir string, blockSize int, outbox, inbox chan protocol.FileInfo) {
	for f := range inbox {
		if f.IsDirectory() || f.IsDeleted() || f.IsSymlink() {
			outbox <- f
			continue
		}

		blocks, err := HashFile(filepath.Join(dir, f.Name), blockSize)
		if err != nil {
			if debug {
				l.Debugln("hash error:", f.Name, err)
			}
			continue
		}

		f.Blocks = blocks
		outbox <- f
	}
}
