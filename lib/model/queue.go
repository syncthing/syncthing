// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"iter"
	"slices"

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
		mut:           sync.NewMutex(),
		iterAllNeeded: iterAllNeeded,
	}
}

func (q *jobQueue) Start(filename string) {
	q.mut.Lock()
	q.progress = append(q.progress, filename)
	l.Debugln("jobQueue.Start", filename)
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
	q.prioritized = slices.Insert(q.prioritized, 0, filename)
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
	l.Debugln("jobQueue.Done", file)
}

// Jobs returns a paginated list of file currently being pulled, files that
// still need to be pulled and any other items that are needed (but can't be
// pushed to front).
// It also returns how many items were skipped.
func (q *jobQueue) Jobs(page, perpage int) ([]string, []string, []string, int) {
	q.mut.Lock()
	defer q.mut.Unlock()

	l.Debugln("jobQueue.Jobs", len(q.progress))

	totalToSkip := (page - 1) * perpage
	toSkip := totalToSkip
	progresstotal := len(q.progress)

	if progresstotal >= toSkip+perpage {
		progress := make([]string, perpage)
		copy(progress, q.progress[toSkip:toSkip+perpage])
		return progress, nil, nil, toSkip
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
	var queued, rest []string
	for file, err := range q.iterAllNeeded() {
		if err != nil {
			break
		}
		if slices.Contains(progress, file.Name) {
			continue
		}
		if file.Type != protocol.FileInfoTypeFile || file.IsDeleted() {
			rest = append(rest, file.Name)
			continue
		}
		if toSkip > 0 {
			toSkip--
			continue
		}
		queued = append(queued, file.Name)
		if len(queued)+progressLen == perpage {
			return progress, queued, nil, totalToSkip
		}
	}
	restLen := len(rest)
	if restLen <= toSkip {
		return progress, queued, nil, totalToSkip - toSkip + restLen
	}
	rest = rest[toSkip:]
	pageLen := progressLen + len(queued)
	if progressLen+pageLen > perpage {
		rest = rest[:perpage-pageLen]
	}
	return progress, queued, rest, totalToSkip
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
