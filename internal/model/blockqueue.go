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

package model

import "github.com/syncthing/syncthing/internal/protocol"

type bqAdd struct {
	file protocol.FileInfo
	have []protocol.BlockInfo
	need []protocol.BlockInfo
}

type bqBlock struct {
	file  protocol.FileInfo
	block protocol.BlockInfo   // get this block from the network
	copy  []protocol.BlockInfo // copy these blocks from the old version of the file
	first bool
	last  bool
}

type blockQueue struct {
	queued []bqBlock
}

func (q *blockQueue) put(a bqAdd) {
	// If we already have it queued, return
	for _, b := range q.queued {
		if b.file.Name == a.file.Name {
			return
		}
	}

	l := len(a.need)

	if len(a.have) > 0 {
		// First queue a copy operation
		q.queued = append(q.queued, bqBlock{
			file:  a.file,
			copy:  a.have,
			first: true,
			last:  l == 0,
		})
	}

	// Queue the needed blocks individually
	for i, b := range a.need {
		q.queued = append(q.queued, bqBlock{
			file:  a.file,
			block: b,
			first: len(a.have) == 0 && i == 0,
			last:  i == l-1,
		})
	}

	if len(a.need)+len(a.have) == 0 {
		// If we didn't have anything to fetch, queue an empty block with the "last" flag set to close the file.
		q.queued = append(q.queued, bqBlock{
			file: a.file,
			last: true,
		})
	}
}

func (q *blockQueue) get() (bqBlock, bool) {
	if len(q.queued) == 0 {
		return bqBlock{}, false
	}
	b := q.queued[0]
	q.queued = q.queued[1:]
	return b, true
}
