// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// Package lamport implements a simple Lamport Clock for versioning
package lamport

import "sync"

var Default = Clock{}

type Clock struct {
	val int64
	mut sync.Mutex
}

func (c *Clock) Tick(v int64) int64 {
	c.mut.Lock()
	if v > c.val {
		c.val = v + 1
		c.mut.Unlock()
		return v + 1
	} else {
		c.val++
		v = c.val
		c.mut.Unlock()
		return v
	}
}
