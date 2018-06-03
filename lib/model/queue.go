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
		queued: diskoverflow.NewSorted(loc),
		// broughtToFront: make([]string),
		handledAtFront: make(map[string]struct{}),
		location:       loc,
		order:          order,
		mut:            sync.NewMutex(),
	}
	if order == config.OrderRandom {
		q.shuffleKeys = make(map[uint64]struct{})
	}
	return q
}

func (q *jobQueue) Push(file string, size int64, modified time.Time) {
	var v diskoverflow.SortValue
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
		v = queueValueShuffled{
			queueValue: queueValue(file),
			key:        n,
		}
	case config.OrderAlphabetic:
		v = queueValueAlphabetic{queueValue(file)}
	case config.OrderSmallestFirst, config.OrderLargestFirst:
		v = queueValueSmallest{
			queueValue: queueValue(file),
			size:       size,
		}
	case config.OrderOldestFirst, config.OrderNewestFirst:
		v = queueValueOldest{
			queueValue: queueValue(file),
			time:       modified,
		}
	}
	q.mut.Lock()
	q.queued.Add(v)
	// q.queued.Append(q.newSortValue(file, size, modified))
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
	v, ok := pop()
	if !ok {
		return "", false
	}
	f := q.toString(v)
	if _, ok := q.handledAtFront[f]; ok {
		return "", false
	}
	q.progress = append(q.progress, f)
	return f, true
}

func (q *jobQueue) toString(v diskoverflow.Value) string {
	switch q.order {
	case config.OrderRandom:
		return string(v.(queueValueShuffled).queueValue)
	case config.OrderAlphabetic:
		return string(v.(queueValueAlphabetic).queueValue)
	case config.OrderSmallestFirst, config.OrderLargestFirst:
		return string(v.(queueValueSmallest).queueValue)
	case config.OrderOldestFirst, config.OrderNewestFirst:
		return string(v.(queueValueOldest).queueValue)
	default:
		panic("unknown type")
	}
}

// func (q *jobQueue) Iter(fn func(string) bool) {
// 	q.iter(fn, q.queued.Iter)
// }

// func (q *jobQueue) IterAndClose(fn func(string) bool) {
// 	q.iter(fn, q.queued.IterAndClose)
// 	q.mut.Lock()
// 	q.queued.Close()
// 	q.mut.Unlock()
// }

// func (q *jobQueue) iter(fn func(string) bool, iterFn func(func(diskoverflow.Value) bool, bool)) {
// 	frontDone := make(map[string]struct{})
// 	for i := len(q.broughtToFront) - 1; i >= 0; i-- {
// 		f := q.broughtToFront[i]
// 		if _, ok := frontDone[f]; !ok {
// 			if !fn(f) {
// 				return
// 			}
// 		}
// 	}

// 	rev := false
// 	switch q.order {
// 	case config.OrderLargestFirst, config.OrderNewestFirst:
// 		rev = true
// 	}
// 	iterFn(func(v diskoverflow.Value) bool {
// 		f := string(v.(queueValue))
// 		if _, ok := frontDone[f]; !ok {
// 			return fn(f)
// 		}
// 		return true
// 	}, rev)
// }

func (q *jobQueue) BringToFront(filename string) {
	q.mut.Lock()
	defer q.mut.Unlock()

	q.queued.Iter(func(v diskoverflow.Value) bool {
		f := q.toString(v)
		if f == filename {
			q.broughtToFront = append([]string{f}, q.broughtToFront...)
			return false
		}
		return true
	}, false)
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

	queued := make([]string, 0, q.queued.Length())

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
		rev = true
	}
	q.queued.Iter(func(v diskoverflow.Value) bool {
		f := q.toString(v)
		if _, ok := atFront[f]; !ok {
			if _, ok := q.handledAtFront[f]; !ok {
				queued = append(queued, f)
			}
		}
		return true
	}, rev)

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
	return len(q.broughtToFront) + q.queued.Length()
}

func (q *jobQueue) lenProgress() int {
	q.mut.Lock()
	defer q.mut.Unlock()
	return len(q.progress)
}

// queueValue implements diskoverflow.Value for strings
type queueValue string

func (q queueValue) Size() int64 {
	return int64(len(q))
}

func (q queueValue) Marshal() []byte {
	return []byte(q)
}

func (q queueValue) Unmarshal(v []byte) {
	q = queueValue(v)
}

type queueValueAlphabetic struct {
	queueValue
}

func (q queueValueAlphabetic) Key() []byte {
	return []byte(q.queueValue)
}

func (q queueValueAlphabetic) Less(other diskoverflow.SortValue) bool {
	return q.queueValue < other.(queueValueAlphabetic).queueValue
}

type queueValueSmallest struct {
	queueValue
	size int64
}

func (q queueValueSmallest) Key() []byte {
	key := make([]byte, 8)
	binary.BigEndian.PutUint64(key[:], uint64(q.size))
	return key
}

func (q queueValueSmallest) Less(other diskoverflow.SortValue) bool {
	return q.size < other.(queueValueSmallest).size
}

type queueValueOldest struct {
	queueValue
	time time.Time
}

func (q queueValueOldest) Key() []byte {
	key, err := q.time.MarshalText()
	if err != nil {
		panic("bug: marshalling time.time should never fail: " + err.Error())
	}
	return key
}

func (q queueValueOldest) Less(other diskoverflow.SortValue) bool {
	return q.time.Before(other.(queueValueOldest).time)
}

type queueValueShuffled struct {
	queueValue
	key uint64
}

func (q queueValueShuffled) Key() []byte {
	key := make([]byte, 8)
	binary.BigEndian.PutUint64(key[:], q.key)
	return key
}

func (q queueValueShuffled) Less(other diskoverflow.SortValue) bool {
	return q.key < other.(queueValueShuffled).key
}
