// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import "sync"

type byteSemaphore struct {
	max       int
	available int
	mut       sync.Mutex
	cond      *sync.Cond
}

func newByteSemaphore(max int) *byteSemaphore {
	s := byteSemaphore{
		max:       max,
		available: max,
	}
	s.cond = sync.NewCond(&s.mut)
	return &s
}

func (s *byteSemaphore) take(bytes int) {
	if bytes > s.max {
		bytes = s.max
	}
	s.mut.Lock()
	for bytes > s.available {
		s.cond.Wait()
	}
	s.available -= bytes
	s.mut.Unlock()
}

func (s *byteSemaphore) give(bytes int) {
	if bytes > s.max {
		bytes = s.max
	}
	s.mut.Lock()
	if s.available+bytes > s.max {
		panic("bug: can never give more than max")
	}
	s.available += bytes
	s.cond.Broadcast()
	s.mut.Unlock()
}
