// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package semaphore

import (
	"context"
	"sync"
)

type Semaphore struct {
	max       int
	available int
	mut       sync.Mutex
	cond      *sync.Cond
}

func New(max int) *Semaphore {
	if max < 0 {
		max = 0
	}
	s := Semaphore{
		max:       max,
		available: max,
	}
	s.cond = sync.NewCond(&s.mut)
	return &s
}

func (s *Semaphore) TakeWithContext(ctx context.Context, size int) error {
	done := make(chan struct{})
	var err error
	go func() {
		err = s.takeInner(ctx, size)
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		s.cond.Broadcast()
		<-done
	}
	return err
}

func (s *Semaphore) Take(size int) {
	_ = s.takeInner(context.Background(), size)
}

func (s *Semaphore) takeInner(ctx context.Context, size int) error {
	// Checking context for size <= s.available is required for testing and doesn't do any harm.
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	s.mut.Lock()
	defer s.mut.Unlock()
	if size > s.max {
		size = s.max
	}
	for size > s.available {
		s.cond.Wait()
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if size > s.max {
			size = s.max
		}
	}
	s.available -= size
	return nil
}

func (s *Semaphore) Give(size int) {
	s.mut.Lock()
	if size > s.max {
		size = s.max
	}
	if s.available+size > s.max {
		s.available = s.max
	} else {
		s.available += size
	}
	s.cond.Broadcast()
	s.mut.Unlock()
}

func (s *Semaphore) SetCapacity(capacity int) {
	if capacity < 0 {
		capacity = 0
	}
	s.mut.Lock()
	diff := capacity - s.max
	s.max = capacity
	s.available += diff
	if s.available < 0 {
		s.available = 0
	} else if s.available > s.max {
		s.available = s.max
	}
	s.cond.Broadcast()
	s.mut.Unlock()
}

func (s *Semaphore) Available() int {
	s.mut.Lock()
	defer s.mut.Unlock()
	return s.available
}

// MultiSemaphore combines semaphores, making sure to always take and give in
// the same order (reversed for give). A semaphore may be nil, in which case it
// is skipped.
type MultiSemaphore []*Semaphore

func (s MultiSemaphore) TakeWithContext(ctx context.Context, size int) error {
	for _, limiter := range s {
		if limiter != nil {
			if err := limiter.TakeWithContext(ctx, size); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s MultiSemaphore) Take(size int) {
	for _, limiter := range s {
		if limiter != nil {
			limiter.Take(size)
		}
	}
}

func (s MultiSemaphore) Give(size int) {
	for i := range s {
		limiter := s[len(s)-1-i]
		if limiter != nil {
			limiter.Give(size)
		}
	}
}
