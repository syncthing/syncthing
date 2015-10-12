// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package model

import (
	"math/rand"
	"sort"

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

func (q *jobQueue) Push(file string, size, modified int64) {
	q.mut.Lock()
	q.queued = append(q.queued, jobQueueEntry{file, size, modified})
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

func (q *jobQueue) Jobs() ([]string, []string) {
	q.mut.Lock()
	defer q.mut.Unlock()

	progress := make([]string, len(q.progress))
	copy(progress, q.progress)

	queued := make([]string, len(q.queued))
	for i := range q.queued {
		queued[i] = q.queued[i].name
	}

	return progress, queued
}

func (q *jobQueue) Shuffle() {
	q.mut.Lock()
	defer q.mut.Unlock()

	l := len(q.queued)
	for i := range q.queued {
		r := rand.Intn(l)
		q.queued[i], q.queued[r] = q.queued[r], q.queued[i]
	}
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
