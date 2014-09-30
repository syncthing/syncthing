// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

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
