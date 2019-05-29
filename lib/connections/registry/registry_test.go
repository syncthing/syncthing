// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package registry

import (
	"testing"
)

func TestRegistry(t *testing.T) {
	r := New()

	if res := r.Get("int", intLess); res != nil {
		t.Error("unexpected")
	}

	r.Register("int", 1)
	r.Register("int", 11)
	r.Register("int4", 4)
	r.Register("int4", 44)
	r.Register("int6", 6)
	r.Register("int6", 66)

	if res := r.Get("int", intLess).(int); res != 1 {
		t.Error("unexpected", res)
	}

	// int is prefix of int4, so returns 1
	if res := r.Get("int4", intLess).(int); res != 1 {
		t.Error("unexpected", res)
	}

	r.Unregister("int", 1)

	// Check that falls through to 11
	if res := r.Get("int", intLess).(int); res != 11 {
		t.Error("unexpected", res)
	}

	// 6 is smaller than 11 available in int.
	if res := r.Get("int6", intLess).(int); res != 6 {
		t.Error("unexpected", res)
	}

	// Unregister 11, int should be impossible to find
	r.Unregister("int", 11)
	if res := r.Get("int", intLess); res != nil {
		t.Error("unexpected")
	}

	// Unregister a second time does nothing.
	r.Unregister("int", 1)

	// Can have multiple of the same
	r.Register("int", 1)
	r.Register("int", 1)
	r.Unregister("int", 1)

	if res := r.Get("int4", intLess).(int); res != 1 {
		t.Error("unexpected", res)
	}
}

func intLess(i, j interface{}) bool {
	iInt := i.(int)
	jInt := j.(int)
	return iInt < jInt
}
