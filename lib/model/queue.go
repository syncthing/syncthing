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
	prioritized []protocol.FileInfo
	mut         sync.Mutex

	getNeeded     func(name string) (protocol.FileInfo, bool)
	iterAllNeeded func() iter.Seq2[protocol.FileInfo, error]
}

func newJobQueue(getNeeded func(name string) (protocol.FileInfo, bool), iterAllNeeded func() iter.Seq2[protocol.FileInfo, error]) *jobQueue {
	return &jobQueue{
		mut:           sync.NewMutex(),
		getNeeded:     getNeeded,
		iterAllNeeded: iterAllNeeded,
	}
}

func (q *jobQueue) Start(filename string) {
	q.mut.Lock()
	q.progress = append(q.progress, filename)
	l.Debugln("jobQueue.Start", filename)
	q.mut.Unlock()
}

func (q *jobQueue) StartPrioritized() (protocol.FileInfo, bool) {
	q.mut.Lock()
	defer q.mut.Unlock()
	pLen := len(q.prioritized)
	if pLen == 0 {
		return protocol.FileInfo{}, false
	}
	file := q.prioritized[0]
	copy(q.prioritized, q.prioritized[1:])
	q.prioritized = q.prioritized[:pLen-1]
	q.progress = append(q.progress, file.Name)
	return file, true
}

func (q *jobQueue) BringToFront(filename string) {
	q.mut.Lock()
	defer q.mut.Unlock()

	for i, cur := range q.prioritized {
		if cur.Name == filename {
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
	if file, ok := q.getNeeded(filename); ok {
		q.prioritized = slices.Insert(q.prioritized, 0, file)
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
	l.Debugln("jobQueue.Done", file)
}

// Jobs returns a paginated list of file currently being pulled, files that
// still need to be pulled and any other items that are needed (but can't be
// pushed to front).
// It also returns how many items were skipped.
func (q *jobQueue) Jobs(page, perpage int) ([]string, []string, []string, int) {
	q.mut.Lock()
	defer q.mut.Unlock()

	l.Debugln("jobQueue.Jobs", len(q.progress), len(q.prioritized))

	totalToSkip := (page - 1) * perpage
	toSkip := totalToSkip
	progressTotal := len(q.progress)
	seen := make(map[string]struct{}, progressTotal+len(q.prioritized))

	// progress

	progress := make([]string, 0, perpage)
	for _, name := range q.progress {
		seen[name] = struct{}{}
		if toSkip > 0 {
			toSkip--
			continue
		}
		progress = append(progress, name)
		if len(progress) == perpage {
			return progress, nil, nil, totalToSkip
		}
	}

	// prioritized -> queued

	progressLen := len(progress)

	queued := make([]string, 0, perpage-len(progress))
	for _, file := range q.prioritized {
		seen[file.Name] = struct{}{}
		if toSkip > 0 {
			toSkip--
			continue
		}
		queued = append(queued, file.Name)
		if len(queued)+progressLen == perpage {
			return progress, queued, nil, totalToSkip
		}
	}

	l.Debugln("jobs toskip after prio", toSkip)

	// queued and rest

	l.Debugln("Jobs", seen)
	var rest []string
	for file, err := range q.iterAllNeeded() {
		if err != nil {
			break
		}
		if _, ok := seen[file.Name]; ok {
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
