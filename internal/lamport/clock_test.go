// Copyright (C) 2014 The Syncthing Authors.
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
