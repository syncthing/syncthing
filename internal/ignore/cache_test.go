// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package ignore

import (
	"testing"
	"time"
)

func TestCache(t *testing.T) {
	c := newCache(nil)

	res, ok := c.get("nonexistent")
	if res != false || ok != false {
		t.Errorf("res %v, ok %v for nonexistent item", res, ok)
	}

	// Set and check some items

	c.set("true", true)
	c.set("false", false)

	res, ok = c.get("true")
	if res != true || ok != true {
		t.Errorf("res %v, ok %v for true item", res, ok)
	}

	res, ok = c.get("false")
	if res != false || ok != true {
		t.Errorf("res %v, ok %v for false item", res, ok)
	}

	// Don't clean anything

	c.clean(time.Second)

	// Same values should exist

	res, ok = c.get("true")
	if res != true || ok != true {
		t.Errorf("res %v, ok %v for true item", res, ok)
	}

	res, ok = c.get("false")
	if res != false || ok != true {
		t.Errorf("res %v, ok %v for false item", res, ok)
	}

	// Sleep and access, to get some data for clean

	time.Sleep(500 * time.Millisecond)

	c.get("true")

	time.Sleep(100 * time.Millisecond)

	// "false" was accessed ~600 ms ago, "true" was accessed ~100 ms ago.
	// This should clean out "false" but not "true"

	c.clean(300 * time.Millisecond)

	// Same values should exist

	_, ok = c.get("true")
	if !ok {
		t.Error("item should still exist")
	}

	_, ok = c.get("false")
	if ok {
		t.Errorf("item should have been cleaned")
	}
}
