// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package rand

import "testing"

func TestRandomString(t *testing.T) {
	for _, l := range []int{0, 1, 2, 3, 4, 8, 42} {
		s := String(l)
		if len(s) != l {
			t.Errorf("Incorrect length %d != %d", len(s), l)
		}
	}

	strings := make([]string, 1000)
	for i := range strings {
		strings[i] = String(8)
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

func TestRandomUint64(t *testing.T) {
	ints := make([]uint64, 1000)
	for i := range ints {
		ints[i] = Uint64()
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

func BenchmarkString(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		String(32)
	}
}
