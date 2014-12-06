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
	"container/list"
	"sync"

	"github.com/syncthing/syncthing/internal/protocol"
)

type JobQueue struct {
	progress []*protocol.FileInfo

	queued *list.List
	lookup map[string]*list.Element // O(1) lookups

	mut sync.Mutex
}

func NewJobQueue() *JobQueue {
	return &JobQueue{
		progress: []*protocol.FileInfo{},
		queued:   list.New(),
		lookup:   make(map[string]*list.Element),
	}
}

func (q *JobQueue) Push(file *protocol.FileInfo) {
	q.mut.Lock()
	defer q.mut.Unlock()

	q.lookup[file.Name] = q.queued.PushBack(file)
}

func (q *JobQueue) Pop() *protocol.FileInfo {
	q.mut.Lock()
	defer q.mut.Unlock()

	if q.queued.Len() == 0 {
		return nil
	}

	f := q.queued.Remove(q.queued.Front()).(*protocol.FileInfo)
	delete(q.lookup, f.Name)
	q.progress = append(q.progress, f)

	return f
}

func (q *JobQueue) Bump(filename string) {
	q.mut.Lock()
	defer q.mut.Unlock()

	ele, ok := q.lookup[filename]
	if ok {
		q.queued.MoveToFront(ele)
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

	queued := make([]protocol.FileInfoTruncated, q.queued.Len())
	i := 0
	for e := q.queued.Front(); e != nil; e = e.Next() {
		fi := e.Value.(*protocol.FileInfo)
		queued[i] = fi.ToTruncated()
		i++
	}

	return progress, queued
}
