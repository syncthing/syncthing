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
	queued         diskoverflow.Sorted
	broughtToFront []string
	handledAtFront map[string]struct{}
	location       string
	order          config.PullOrder
	shuffleKeys    map[uint64]struct{}
	mut            sync.Mutex
}

func newJobQueue(order config.PullOrder, loc string) *jobQueue {
	q := &jobQueue{
		handledAtFront: make(map[string]struct{}),
		location:       loc,
		order:          order,
		mut:            sync.NewMutex(),
	}
	q.queued = diskoverflow.NewSorted(loc)
	if order == config.OrderRandom {
		q.shuffleKeys = make(map[uint64]struct{})
	}
	return q
}

func (q *jobQueue) Push(file string, size int64, modified time.Time) {
	var key []byte
	switch q.order {
	case config.OrderRandom:
		var n uint64
		for {
			n = rand.Uint64()
			if _, ok := q.shuffleKeys[n]; !ok {
				q.shuffleKeys[n] = struct{}{}
				break
			}
		}
		key = make([]byte, 8)
		binary.BigEndian.PutUint64(key[:], n)
	case config.OrderAlphabetic:
		key = []byte(file)
	case config.OrderSmallestFirst, config.OrderLargestFirst:
		key = make([]byte, 8)
		binary.BigEndian.PutUint64(key[:], uint64(size))
	case config.OrderOldestFirst, config.OrderNewestFirst:
		key, _ = modified.MarshalText()
	}
	q.mut.Lock()
	q.queued.Add(key, &queueValue{file})
	q.mut.Unlock()
}

func (q *jobQueue) Pop() (string, bool) {
	q.mut.Lock()
	defer q.mut.Unlock()
	if l := len(q.broughtToFront); l > 0 {
		f := q.broughtToFront[l-1]
		q.broughtToFront = q.broughtToFront[:l-1]
		if _, ok := q.handledAtFront[f]; !ok {
			q.handledAtFront[f] = struct{}{}
			q.progress = append(q.progress, f)
			return f, true
		}
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
	if _, ok := q.handledAtFront[f]; ok {
		return "", false
	}
	q.progress = append(q.progress, f)
	return f, true
}

func (q *jobQueue) BringToFront(filename string) {
	q.mut.Lock()
	defer q.mut.Unlock()

	it := q.queued.NewIterator(false)
	defer it.Release()
	v := &queueValue{}
	for it.Next() {
		it.Value(v)
		if f := v.string; f == filename {
			q.broughtToFront = append([]string{f}, q.broughtToFront...)
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

	queued := make([]string, 0, q.queued.Items())

	atFront := make(map[string]struct{}, len(q.broughtToFront))
	for _, f := range q.broughtToFront {
		if _, ok := atFront[f]; !ok {
			queued = append(queued, f)
			atFront[f] = struct{}{}
		}
	}

	rev := false
	switch q.order {
	case config.OrderLargestFirst, config.OrderNewestFirst:
		l.Infoln("queue iter", q.order)
		rev = true
	}
	it := q.queued.NewIterator(rev)
	v := &queueValue{}
	for it.Next() {
		it.Value(v)
		f := v.string
		if _, ok := atFront[f]; !ok {
			if _, ok := q.handledAtFront[f]; !ok {
				queued = append(queued, f)
			}
		}
	}
	it.Release()

	return progress, queued
}

// To be called after a puller iteration finishes
func (q *jobQueue) Reset() {
	q.mut.Lock()
	q.queued = diskoverflow.NewSorted(q.location)
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

func (q *queueValue) Bytes() int {
	return len(q.string)
}

func (q *queueValue) Marshal() []byte {
	return []byte(q.string)
}

func (q *queueValue) Unmarshal(v []byte) {
	q.string = string(v)
}

func (q *queueValue) Copy(v diskoverflow.Value) {
	q.string = v.(*queueValue).string
}

func (q *queueValue) Reset() {
	q.string = ""
}
