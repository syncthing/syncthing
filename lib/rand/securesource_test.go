// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package rand

import "testing"

func TestSecureSource(t *testing.T) {
	// This is not a test to verify that the random numbers are secure,
	// merely that the numbers look random at all and that we've haven't
	// broken the implementation by masking off half the bits or always
	// returning "4" (chosen by fair dice roll),

	const nsamples = 10000

	// Create a new source and sample values from it.
	s := newSecureSource()
	res0 := make([]int64, nsamples)
	for i := range res0 {
		res0[i] = s.Int63()
	}

	// Do it again
	s = newSecureSource()
	res1 := make([]int64, nsamples)
	for i := range res1 {
		res1[i] = s.Int63()
	}

	// There should (statistically speaking) be no repetition of the values,
	// neither within the samples from a source nor between sources.
	for _, v0 := range res0 {
		for _, v1 := range res1 {
			if v0 == v1 {
				t.Errorf("Suspicious coincidence, %d repeated between res0/res1", v0)
			}
		}
	}
	for i, v0 := range res0 {
		for _, v1 := range res0[i+1:] {
			if v0 == v1 {
				t.Errorf("Suspicious coincidence, %d repeated within res0", v0)
			}
		}
	}
	for i, v0 := range res1 {
		for _, v1 := range res1[i+1:] {
			if v0 == v1 {
				t.Errorf("Suspicious coincidence, %d repeated within res1", v0)
			}
		}
	}

	// Count how many times each bit was set. On average each bit ought to
	// be set in half of the samples, except the topmost bit which must
	// never be set (int63). We raise an alarm if a single bit is set in
	// fewer than 1/3 of the samples or more often than 2/3 of the samples.
	var bits [64]int
	for _, v := range res0 {
		for i := range bits {
			if v&1 == 1 {
				bits[i]++
			}
			v >>= 1
		}
	}
	for bit, count := range bits {
		switch bit {
		case 63:
			// The topmost bit is never set
			if count != 0 {
				t.Errorf("The topmost bit was set %d times in %d samples (should be 0)", count, nsamples)
			}
		default:
			if count < nsamples/3 {
				t.Errorf("Bit %d was set only %d times out of %d", bit, count, nsamples)
			}
			if count > nsamples/3*2 {
				t.Errorf("Bit %d was set fully %d times out of %d", bit, count, nsamples)
			}
		}
	}
}

var sink int64

func BenchmarkSecureSource(b *testing.B) {
	s := newSecureSource()
	for i := 0; i < b.N; i++ {
		sink = s.Int63()
	}
	b.ReportAllocs()
}
