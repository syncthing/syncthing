// Copyright (C) 2023 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package sliceutil_test

import (
	"slices"
	"testing"

	"github.com/syncthing/syncthing/lib/sliceutil"
)

func TestRemoveAndZero(t *testing.T) {
	a := []int{1, 2, 3, 4, 5}
	b := sliceutil.RemoveAndZero(a, 2)
	exp := []int{1, 2, 4, 5}
	if !slices.Equal(b, exp) {
		t.Errorf("got %v, expected %v", b, exp)
	}
	for _, e := range a {
		if e == 3 {
			t.Errorf("element should have been zeroed")
		}
	}
}
