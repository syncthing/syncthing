// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package lamport

import "testing"

var inputs = []uint64{0, 42, 2, 3, 4, 8, 9, 33, 44, 112, 100}

func TestClock(t *testing.T) {
	c := Clock{}

	var prev uint64
	for _, input := range inputs {
		cur := c.Tick(input)
		if cur <= prev || cur <= input {
			t.Error("Clock moving backwards")
		}
		prev = cur
	}
}
