// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"encoding/binary"
	"math/rand"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/diskoverflow"
	"github.com/syncthing/syncthing/lib/sync"
)

type jobQueue struct {
	progress       []string
	queued         diskoverflow.SortedMap
	broughtToFront []string
	location       string
	order          config.PullOrder
	mut            sync.Mutex
}

func newJobQueue(order config.PullOrder, loc string) *jobQueue {
	q := &jobQueue{
		location: loc,
		order:    order,
		mut:      sync.NewMutex(),
	}
	q.queued = diskoverflow.NewSortedMap(loc)
	return q
}

func (q *jobQueue) Push(file string, size int64, modified time.Time) {
	var key []byte
	switch q.order {
	case config.OrderRandom:
		n := rand.Uint64()
		key = make([]byte, 8)
		binary.BigEndian.PutUint64(key, n)
	case config.OrderAlphabetic:
		key = []byte(file)
	case config.OrderSmallestFirst, config.OrderLargestFirst:
		key = make([]byte, 8)
		binary.BigEndian.PutUint64(key[:], uint64(size))
	case config.OrderOldestFirst, config.OrderNewestFirst:
		key, _ = modified.MarshalText()
	}
	q.mut.Lock()
	q.queued.Set(key, &queueValue{file})
	q.mut.Unlock()
}

func (q *jobQueue) Pop() (string, bool) {
	q.mut.Lock()
	defer q.mut.Unlock()
	if l := len(q.broughtToFront); l > 0 {
		f := q.broughtToFront[l-1]
		q.broughtToFront = q.broughtToFront[:l-1]
		q.progress = append(q.progress, f)
		return f, true
	}
	pop := q.queued.PopFirst
	switch q.order {
	case config.OrderLargestFirst, config.OrderNewestFirst:
		pop = q.queued.PopLast
	}
	v := &queueValue{}
	ok := pop(v)
	if !ok {
		return "", false
	}
	f := v.string
	q.progress = append(q.progress, f)
	return f, true
}

func (q *jobQueue) BringToFront(filename string) {
	q.mut.Lock()
	defer q.mut.Unlock()

	var v queueValue
	if ok := q.queued.Pop([]byte(filename), &v); ok {
		q.broughtToFront = append([]string{v.string}, q.broughtToFront...)
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

	queued := make([]string, 0, q.queued.Items())

	atFront := make(map[string]struct{}, len(q.broughtToFront))
	for _, f := range q.broughtToFront {
		if _, ok := atFront[f]; !ok {
			queued = append(queued, f)
			atFront[f] = struct{}{}
		}
	}

	var it diskoverflow.Iterator
	switch q.order {
	case config.OrderLargestFirst, config.OrderNewestFirst:
		it = q.queued.NewReverseIterator()
	default:
		it = q.queued.NewIterator()
	}
	for it.Next() {
		v := queueValue{}
		it.Value(&v)
		queued = append(queued, v.string)
	}
	it.Release()

	return progress, queued
}

func (q *jobQueue) Close() {
	q.mut.Lock()
	q.queued.Close()
	q.mut.Unlock()
}

func (q *jobQueue) lenQueued() int {
	q.mut.Lock()
	defer q.mut.Unlock()
	return len(q.broughtToFront) + q.queued.Items()
}

func (q *jobQueue) lenProgress() int {
	q.mut.Lock()
	defer q.mut.Unlock()
	return len(q.progress)
}

// queueValue implements diskoverflow.Value for strings
type queueValue struct {
	string
}

func (q *queueValue) ProtoSize() int {
	return len(q.string)
}

func (q *queueValue) Marshal() ([]byte, error) {
	return []byte(q.string), nil
}

func (q *queueValue) Unmarshal(v []byte) error {
	q.string = string(v)
	return nil
}

func (q *queueValue) Reset() {
	q.string = ""
}
