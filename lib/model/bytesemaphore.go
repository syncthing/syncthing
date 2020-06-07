// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"context"
	"sync"
)

type byteSemaphore struct {
	max       int
	available int
	mut       sync.Mutex
	cond      *sync.Cond
}

func newByteSemaphore(max int) *byteSemaphore {
	if max < 0 {
		max = 0
	}
	s := byteSemaphore{
		max:       max,
		available: max,
	}
	s.cond = sync.NewCond(&s.mut)
	return &s
}

func (s *byteSemaphore) takeWithContext(ctx context.Context, bytes int) error {
	done := make(chan struct{})
	var err error
	go func() {
		err = s.takeInner(ctx, bytes)
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

func (s *byteSemaphore) take(bytes int) {
	_ = s.takeInner(context.Background(), bytes)
}

func (s *byteSemaphore) takeInner(ctx context.Context, bytes int) error {
	// Checking context for bytes <= s.available is required for testing and doesn't do any harm.
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	s.mut.Lock()
	defer s.mut.Unlock()
	if bytes > s.max {
		bytes = s.max
	}
	for bytes > s.available {
		s.cond.Wait()
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if bytes > s.max {
			bytes = s.max
		}
	}
	s.available -= bytes
	return nil
}

func (s *byteSemaphore) give(bytes int) {
	s.mut.Lock()
	if bytes > s.max {
		bytes = s.max
	}
	if s.available+bytes > s.max {
		s.available = s.max
	} else {
		s.available += bytes
	}
	s.cond.Broadcast()
	s.mut.Unlock()
}

func (s *byteSemaphore) setCapacity(cap int) {
	if cap < 0 {
		cap = 0
	}
	s.mut.Lock()
	diff := cap - s.max
	s.max = cap
	s.available += diff
	if s.available < 0 {
		s.available = 0
	} else if s.available > s.max {
		s.available = s.max
	}
	s.cond.Broadcast()
	s.mut.Unlock()
}
