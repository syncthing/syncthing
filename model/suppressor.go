// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package model

import (
	"os"
	"sync"
	"time"
)

const (
	MaxChangeHistory = 4
)

type change struct {
	size int64
	when time.Time
}

type changeHistory struct {
	changes []change
	next    int64
	prevSup bool
}

type suppressor struct {
	sync.Mutex
	changes   map[string]changeHistory
	threshold int64 // bytes/s
}

func (h changeHistory) bandwidth(t time.Time) int64 {
	if len(h.changes) == 0 {
		return 0
	}

	var t0 = h.changes[0].when
	if t == t0 {
		return 0
	}

	var bw float64
	for _, c := range h.changes {
		bw += float64(c.size)
	}
	return int64(bw / t.Sub(t0).Seconds())
}

func (h *changeHistory) append(size int64, t time.Time) {
	c := change{size, t}
	if len(h.changes) == MaxChangeHistory {
		h.changes = h.changes[1:MaxChangeHistory]
	}
	h.changes = append(h.changes, c)
}

func (s *suppressor) Suppress(name string, fi os.FileInfo) (cur, prev bool) {
	return s.suppress(name, fi.Size(), time.Now())
}

func (s *suppressor) suppress(name string, size int64, t time.Time) (bool, bool) {
	s.Lock()

	if s.changes == nil {
		s.changes = make(map[string]changeHistory)
	}
	h := s.changes[name]
	sup := h.bandwidth(t) > s.threshold
	prevSup := h.prevSup
	h.prevSup = sup
	if !sup {
		h.append(size, t)
	}
	s.changes[name] = h

	s.Unlock()

	return sup, prevSup
}
