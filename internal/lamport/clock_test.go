// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package lamport

import "testing"

var inputs = []int64{0, 42, 2, 3, 4, 8, 9, 33, 44, 112, 100}

func TestClock(t *testing.T) {
	c := Clock{}

	var prev int64
	for _, input := range inputs {
		cur := c.Tick(input)
		if cur <= prev || cur <= input {
			t.Error("Clock moving backwards")
		}
		prev = cur
	}
}
