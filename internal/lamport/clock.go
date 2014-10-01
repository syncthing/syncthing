// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
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

// Package lamport implements a simple Lamport Clock for versioning
package lamport

import "sync"

var Default = Clock{}

type Clock struct {
	val uint64
	mut sync.Mutex
}

func (c *Clock) Tick(v uint64) uint64 {
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
