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

package main

import (
	"sync"
	"testing"
)

var predictableRandomTest sync.Once

func TestPredictableRandom(t *testing.T) {
	predictableRandomTest.Do(func() {
		// predictable random sequence is predictable
		e := 3440579354231278675
		if v := predictableRandom.Int(); v != e {
			t.Errorf("Unexpected random value %d != %d", v, e)
		}
	})
}

func TestSeedFromBytes(t *testing.T) {
	// should always return the same seed for the same bytes
	tcs := []struct {
		bs []byte
		v  int64
	}{
		{[]byte("hello world"), -3639725434188061933},
		{[]byte("hello worlx"), -2539100776074091088},
	}

	for _, tc := range tcs {
		if v := seedFromBytes(tc.bs); v != tc.v {
			t.Errorf("Unexpected seed value %d != %d", v, tc.v)
		}
	}
}

func TestRandomString(t *testing.T) {
	for _, l := range []int{0, 1, 2, 3, 4, 8, 42} {
		s := randomString(l)
		if len(s) != l {
			t.Errorf("Incorrect length %d != %d", len(s), l)
		}
	}

	strings := make([]string, 1000)
	for i := range strings {
		strings[i] = randomString(8)
		for j := range strings {
			if i == j {
				continue
			}
			if strings[i] == strings[j] {
				t.Errorf("Repeated random string %q", strings[i])
			}
		}
	}
}

func TestRandomInt64(t *testing.T) {
	ints := make([]int64, 1000)
	for i := range ints {
		ints[i] = randomInt64()
		for j := range ints {
			if i == j {
				continue
			}
			if ints[i] == ints[j] {
				t.Errorf("Repeated random int64 %d", ints[i])
			}
		}
	}
}
