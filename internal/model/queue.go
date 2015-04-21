// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package model

import (
	"math/rand"
	"sync"
)

type jobQueue struct {
	progress []string
	queued   []string
	mut      sync.Mutex
}

func newJobQueue() *jobQueue {
	return &jobQueue{}
}

func (q *jobQueue) Push(file string) {
	q.mut.Lock()
	q.queued = append(q.queued, file)
	q.mut.Unlock()
}

func (q *jobQueue) Pop() (string, bool) {
	q.mut.Lock()
	defer q.mut.Unlock()

	if len(q.queued) == 0 {
		return "", false
	}

	var f string
	f = q.queued[0]
	q.queued = q.queued[1:]
	q.progress = append(q.progress, f)

	return f, true
}

func (q *jobQueue) BringToFront(filename string) {
	q.mut.Lock()
	defer q.mut.Unlock()

	for i, cur := range q.queued {
		if cur == filename {
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
	copy(queued, q.queued)

	return progress, queued
}

func (q *jobQueue) Shuffle() {
	q.mut.Lock()
	defer q.mut.Unlock()

	for i := range q.queued {
		j := rand.Intn(i + 1)
		q.queued[i], q.queued[j] = q.queued[j], q.queued[i]
	}
}
