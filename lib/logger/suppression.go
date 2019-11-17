// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package logger

import (
	"fmt"
	"hash/fnv"
	"sync"
	"time"
)

func NewSuppressionMiddleware(numBuckets, threshold int, bucketInterval time.Duration) OutputMiddleware {
	return func(ll Lowlevel) Lowlevel {
		supp := newSuppressingLogger(ll, numBuckets, threshold, bucketInterval)
		go supp.Serve()
		return supp
	}
}

// The suppressingLogger passes logging through to the next logger, unless
// it's a repeated message or the rate thresholds are exceeded in which case
// previously seen messages are filtered.
type suppressingLogger struct {
	next Lowlevel

	mut            sync.Mutex
	buckets        []suppressingLoggerBucket
	bucketInterval time.Duration // how often to rotate
	threshold      int           // threshold for suppression in average messages/bucket
	active         bool          // whether suppression is currently active
	stop           chan struct{}
}

func newSuppressingLogger(next Lowlevel, numBuckets, threshold int, bucketInterval time.Duration) *suppressingLogger {
	l := &suppressingLogger{
		next:           next,
		buckets:        make([]suppressingLoggerBucket, numBuckets),
		bucketInterval: bucketInterval,
		threshold:      threshold,
		stop:           make(chan struct{}),
	}
	l.buckets[0].reset() // prep for first use
	return l
}

func (l *suppressingLogger) Serve() {
	// Rotate buckets every bucketInterval, also setting/clearing the active
	// flag.

	t := time.NewTicker(l.bucketInterval)
	defer t.Stop()

	for {
		select {
		case <-t.C:
			l.rotateBuckets()
		case <-l.stop:
			return
		}
	}
}

func (l *suppressingLogger) Stop() {
	close(l.stop)
}

func (l *suppressingLogger) SetFlags(flags int) {
	l.next.SetFlags(flags)
}

func (l *suppressingLogger) SetPrefix(prefix string) {
	l.next.SetPrefix(prefix)
}

func (l *suppressingLogger) Output(level int, message string) error {
	l.mut.Lock()
	defer l.mut.Unlock()

	hash := messageHash(message)
	l.buckets[0].seen[hash] = struct{}{}

	if l.active && l.haveSeen(hash) {
		l.buckets[0].suppressed++
		return nil
	}

	l.buckets[0].count++
	return l.next.Output(level+1, message)
}

func (l *suppressingLogger) rotateBuckets() {
	l.mut.Lock()
	defer l.mut.Unlock()

	if supp := l.buckets[0].suppressed; supp > 0 {
		_ = l.next.Output(0, fmt.Sprintf("LOG: Suppressed %d previously seen log messages", supp))
	}

	shouldActive := l.averageRate() > l.threshold
	if shouldActive && !l.active {
		_ = l.next.Output(0, "LOG: Enabling log suppression")
	} else if !shouldActive && l.active {
		_ = l.next.Output(0, "LOG: Disabling log suppression")
	}

	l.active = shouldActive

	// Shift all buckets to the right and reset the one at [0]
	copy(l.buckets[1:], l.buckets)
	l.buckets[0].reset()
}

func (l *suppressingLogger) haveSeen(hash uint64) bool {
	for _, b := range l.buckets {
		_, seen := b.seen[hash]
		if seen {
			return true
		}
	}
	return false
}

// averageRate returns the average logging rate for the current set of
// buckets, in messages / secondsPerBucket
func (l *suppressingLogger) averageRate() int {
	sum := 0
	for _, b := range l.buckets {
		sum += b.count + b.suppressed
	}
	return sum / len(l.buckets)
}

func messageHash(message string) uint64 {
	h := fnv.New64()
	_, _ = h.Write([]byte(message))
	return h.Sum64()
}

type suppressingLoggerBucket struct {
	seen       map[uint64]struct{}
	count      int
	suppressed int
}

func (b *suppressingLoggerBucket) reset() {
	b.seen = make(map[uint64]struct{})
	b.count = 0
	b.suppressed = 0
}
