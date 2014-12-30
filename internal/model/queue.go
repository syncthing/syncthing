// Copyright (C) 2014 The Syncthing Authors.
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

import (
	"sync"

	"github.com/syncthing/syncthing/internal/protocol"
)

type JobQueue struct {
	progress []*protocol.FileInfo
	queued   []*protocol.FileInfo
	mut      sync.Mutex
}

func NewJobQueue() *JobQueue {
	return &JobQueue{}
}

func (q *JobQueue) Push(file *protocol.FileInfo) {
	q.mut.Lock()
	q.queued = append(q.queued, file)
	q.mut.Unlock()
}

func (q *JobQueue) Pop() *protocol.FileInfo {
	q.mut.Lock()
	defer q.mut.Unlock()

	if len(q.queued) == 0 {
		return nil
	}

	var f *protocol.FileInfo
	f, q.queued[0] = q.queued[0], nil
	q.queued = q.queued[1:]
	q.progress = append(q.progress, f)

	return f
}

func (q *JobQueue) Bump(filename string) {
	q.mut.Lock()
	defer q.mut.Unlock()

	for i := range q.queued {
		if q.queued[i].Name == filename {
			q.queued[0], q.queued[i] = q.queued[i], q.queued[0]
			return
		}
	}
}

func (q *JobQueue) Done(file *protocol.FileInfo) {
	q.mut.Lock()
	defer q.mut.Unlock()

	for i := range q.progress {
		if q.progress[i].Name == file.Name {
			copy(q.progress[i:], q.progress[i+1:])
			q.progress[len(q.progress)-1] = nil
			q.progress = q.progress[:len(q.progress)-1]
			return
		}
	}
}

func (q *JobQueue) Jobs() ([]protocol.FileInfoTruncated, []protocol.FileInfoTruncated) {
	q.mut.Lock()
	defer q.mut.Unlock()

	progress := make([]protocol.FileInfoTruncated, len(q.progress))
	for i := range q.progress {
		progress[i] = q.progress[i].ToTruncated()
	}

	queued := make([]protocol.FileInfoTruncated, len(q.queued))
	for i := range q.queued {
		queued[i] = q.queued[i].ToTruncated()
	}

	return progress, queued
}
