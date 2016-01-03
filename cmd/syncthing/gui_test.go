// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package main

import "testing"

func TestCSRFToken(t *testing.T) {
	t1 := newCsrfToken()
	t2 := newCsrfToken()

	t3 := newCsrfToken()
	if !validCsrfToken(t3) {
		t.Fatal("t3 should be valid")
	}

	for i := 0; i < 250; i++ {
		if i%5 == 0 {
			// t1 and t2 should remain valid by virtue of us checking them now
			// and then.
			if !validCsrfToken(t1) {
				t.Fatal("t1 should be valid at iteration", i)
			}
			if !validCsrfToken(t2) {
				t.Fatal("t2 should be valid at iteration", i)
			}
		}

		// The newly generated token is always valid
		t4 := newCsrfToken()
		if !validCsrfToken(t4) {
			t.Fatal("t4 should be valid at iteration", i)
		}
	}

	if validCsrfToken(t3) {
		t.Fatal("t3 should have expired by now")
	}
}
