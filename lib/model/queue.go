// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"iter"

	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/sync"
)

type jobQueue struct {
	progress    []string
	prioritized []string
	mut         sync.Mutex

	iterAllNeeded func() iter.Seq2[protocol.FileInfo, error]
}

func newJobQueue(iterAllNeeded func() iter.Seq2[protocol.FileInfo, error]) *jobQueue {
	return &jobQueue{
		mut: sync.NewMutex(),
		iterAllNeeded: iterAllNeeded,
	}
}

func (q *jobQueue) Start(filename string) {
	q.mut.Lock()
	q.progress = append(q.progress, filename)
	q.mut.Unlock()
}

func (q *jobQueue) StartPrioritized() (string, bool) {
	q.mut.Lock()
	defer q.mut.Unlock()
	pLen := len(q.prioritized)
	if pLen == 0 {
		return "", false
	}
	filename := q.prioritized[0]
	copy(q.prioritized, q.prioritized[1:])
	q.prioritized = q.prioritized[:pLen-1]
	q.progress = append(q.progress, filename)
	return filename, true
}

func (q *jobQueue) BringToFront(filename string) {
	q.mut.Lock()
	defer q.mut.Unlock()

	for i, cur := range q.prioritized {
		if cur == filename {
			if i > 0 {
				// Shift the elements before the selected element one step to
				// the right, overwriting the selected element
				copy(q.prioritized[1:i+1], q.prioritized[0:])
				// Put the selected element at the front
				q.prioritized[0] = cur
			}
			return
		}
	}
}

func (q *jobQueue) Done(file string) {
	q.mut.Lock()
	defer q.mut.Unlock()

	for i := range q.progress {
		if q.progress[i] == file {
			copy(q.progress[i:], q.progress[i+1:])
			q.progress = q.progress[:len(q.progress)-1]
			return
		}
	}
}

// Jobs returns a paginated list of file currently being pulled and files queued
// to be pulled. It also returns how many items were skipped.
func (q *jobQueue) Jobs(page, perpage int) ([]string, []string, int) {
	q.mut.Lock()
	defer q.mut.Unlock()

	toSkip := (page - 1) * perpage
	progresstotal := len(q.progress)

	if progresstotal >= toSkip+perpage {
		progress := make([]string, perpage)
		copy(progress, q.progress[toSkip:toSkip+perpage])
		return progress, nil, toSkip
	}

	var progress []string
	if progresstotal > toSkip {
		progress = make([]string, progresstotal-toSkip)
		copy(progress, q.progress[toSkip:progresstotal])
		toSkip = 0
	} else {
		toSkip -= progresstotal
	}

	progressLen := len(progress)
	var queued []string
	for file, err := range q.iterAllNeeded() {
		if err != nil {
			break
		}
		if file.Type != protocol.FileInfoTypeFile || file.IsDeleted() {
			continue
		}
		if toSkip > 0 {
			toSkip--
			continue
		}
		queued = append(queued, file.Name)
		if len(queued) + progressLen == perpage {
			break
		}
	}

	return progress, queued, (page - 1) * perpage
}

func (q *jobQueue) Reset() {
	q.mut.Lock()
	defer q.mut.Unlock()
	q.progress = nil
	q.prioritized = nil
}

func (q *jobQueue) lenProgress() int {
	q.mut.Lock()
	defer q.mut.Unlock()
	return len(q.progress)
}
