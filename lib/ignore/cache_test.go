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
	if res != DontIgnore || ok != false {
		t.Errorf("res %v %v, ok %v for nonexistent item", DontIgnore, res, ok)
	}

	// Set and check some items

	c.set("nuke", Nuke)
	c.set("preserve", Preserve)
	c.set("dontignore", DontIgnore)

	res, ok = c.get("nuke")
	if res != Nuke || ok != true {
		t.Errorf("res %v, ok %v for nuke item", res, ok)
	}

	res, ok = c.get("preserve")
	if res != Preserve || ok != true {
		t.Errorf("res %v, ok %v for ignore item", res, ok)
	}

	res, ok = c.get("dontignore")
	if res != DontIgnore || ok != true {
		t.Errorf("res %v, ok %v for dontignore item", res, ok)
	}

	// Don't clean anything

	c.clean(time.Second)

	// Same values should exist

	res, ok = c.get("nuke")
	if res != Nuke || ok != true {
		t.Errorf("res %v, ok %v for nuke item", res, ok)
	}

	res, ok = c.get("preserve")
	if res != Preserve || ok != true {
		t.Errorf("res %v, ok %v for ignore item", res, ok)
	}

	res, ok = c.get("dontignore")
	if res != DontIgnore || ok != true {
		t.Errorf("res %v, ok %v for dontignore item", res, ok)
	}

	// Sleep and access, to get some data for clean

	time.Sleep(500 * time.Millisecond)

	c.get("preserve")

	time.Sleep(100 * time.Millisecond)

	// "dontignore" was accessed ~600 ms ago, "ignore" was accessed ~100 ms ago.
	// This should clean out "dontignore" but not "ignore"

	c.clean(300 * time.Millisecond)

	// Same values should exist

	_, ok = c.get("preserve")
	if !ok {
		t.Error("item should still exist")
	}

	_, ok = c.get("dontignore")
	if ok {
		t.Errorf("item should have been cleaned")
	}
}
