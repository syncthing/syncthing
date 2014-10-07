// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

package scanner

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/syncthing/syncthing/internal/protocol"
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
		if protocol.IsDirectory(f.Flags) || protocol.IsDeleted(f.Flags) {
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
