// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package scanner

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/syncthing/syncthing/protocol"
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
			hashFile(dir, blockSize, outbox, inbox)
			wg.Done()
		}()
	}

	go func() {
		wg.Wait()
		close(outbox)
	}()
}

func hashFile(dir string, blockSize int, outbox, inbox chan protocol.FileInfo) {
	for f := range inbox {
		if protocol.IsDirectory(f.Flags) || protocol.IsDeleted(f.Flags) {
			outbox <- f
			continue
		}

		fd, err := os.Open(filepath.Join(dir, f.Name))
		if err != nil {
			if debug {
				l.Debugln("open:", err)
			}
			continue
		}

		blocks, err := Blocks(fd, blockSize)
		fd.Close()

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
