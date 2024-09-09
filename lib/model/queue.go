// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"sort"
	"time"

	"github.com/syncthing/syncthing/lib/rand"
	"github.com/syncthing/syncthing/lib/sync"
)

type jobQueue struct {
	progress []string
	queued   []jobQueueEntry
	mut      sync.Mutex
}

type jobQueueEntry struct {
	name     string
	size     int64
	modified int64
}

func newJobQueue() *jobQueue {
	return &jobQueue{
		mut: sync.NewMutex(),
	}
}

func (q *jobQueue) Push(file string, size int64, modified time.Time) {
	q.mut.Lock()
	// The range of UnixNano covers a range of reasonable timestamps.
	q.queued = append(q.queued, jobQueueEntry{file, size, modified.UnixNano()})
	q.mut.Unlock()
}

func (q *jobQueue) Pop() (string, bool) {
	q.mut.Lock()
	defer q.mut.Unlock()

	if len(q.queued) == 0 {
		return "", false
	}

	f := q.queued[0].name
	q.queued = q.queued[1:]
	q.progress = append(q.progress, f)

	return f, true
}

func (q *jobQueue) BringToFront(filename string) {
	q.mut.Lock()
	defer q.mut.Unlock()

	for i, cur := range q.queued {
		if cur.name == filename {
			if i > 0 {
				// Shift the elements before the selected element one step to
				// the right, overwriting the selected element
				copy(q.queued[1:i+1], q.queued[0:])
				// Put the selected element at the front
				q.queued[0] = cur
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
	plen := len(q.progress)
	qlen := len(q.queued)

	if tot := plen + qlen; tot <= toSkip {
		return nil, nil, tot
	}

	if plen >= toSkip+perpage {
		progress := make([]string, perpage)
		copy(progress, q.progress[toSkip:toSkip+perpage])
		return progress, nil, toSkip
	}

	var progress []string
	if plen > toSkip {
		progress = make([]string, plen-toSkip)
		copy(progress, q.progress[toSkip:plen])
		toSkip = 0
	} else {
		toSkip -= plen
	}

	var queued []string
	if qlen-toSkip < perpage-len(progress) {
		queued = make([]string, qlen-toSkip)
	} else {
		queued = make([]string, perpage-len(progress))
	}
	for i := range queued {
		queued[i] = q.queued[i+toSkip].name
	}

	return progress, queued, (page - 1) * perpage
}

func (q *jobQueue) Shuffle() {
	q.mut.Lock()
	defer q.mut.Unlock()

	rand.Shuffle(q.queued)
}

func (q *jobQueue) Reset() {
	q.mut.Lock()
	defer q.mut.Unlock()
	q.progress = nil
	q.queued = nil
}

func (q *jobQueue) lenQueued() int {
	q.mut.Lock()
	defer q.mut.Unlock()
	return len(q.queued)
}

func (q *jobQueue) lenProgress() int {
	q.mut.Lock()
	defer q.mut.Unlock()
	return len(q.progress)
}

func (q *jobQueue) SortSmallestFirst() {
	q.mut.Lock()
	defer q.mut.Unlock()

	sort.Sort(smallestFirst(q.queued))
}

func (q *jobQueue) SortLargestFirst() {
	q.mut.Lock()
	defer q.mut.Unlock()

	sort.Sort(sort.Reverse(smallestFirst(q.queued)))
}

func (q *jobQueue) SortOldestFirst() {
	q.mut.Lock()
	defer q.mut.Unlock()

	sort.Sort(oldestFirst(q.queued))
}

func (q *jobQueue) SortNewestFirst() {
	q.mut.Lock()
	defer q.mut.Unlock()

	sort.Sort(sort.Reverse(oldestFirst(q.queued)))
}

// The usual sort.Interface boilerplate

type smallestFirst []jobQueueEntry

func (q smallestFirst) Len() int           { return len(q) }
func (q smallestFirst) Less(a, b int) bool { return q[a].size < q[b].size }
func (q smallestFirst) Swap(a, b int)      { q[a], q[b] = q[b], q[a] }

type oldestFirst []jobQueueEntry

func (q oldestFirst) Len() int           { return len(q) }
func (q oldestFirst) Less(a, b int) bool { return q[a].modified < q[b].modified }
func (q oldestFirst) Swap(a, b int)      { q[a], q[b] = q[b], q[a] }
