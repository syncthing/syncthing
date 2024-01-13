// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package ignore

import (
	"testing"
	"time"

	"github.com/syncthing/syncthing/lib/ignore/ignoreresult"
)

func TestCache(t *testing.T) {
	fc := new(fakeClock)
	oldClock := clock
	clock = fc
	defer func() {
		clock = oldClock
	}()

	c := newCache(nil)

	res, ok := c.get("nonexistent")
	if res.IsIgnored() || res.IsDeletable() || ok {
		t.Errorf("res %v, ok %v for nonexistent item", res, ok)
	}

	// Set and check some items

	c.set("true", ignoreresult.IgnoredDeletable)
	c.set("false", 0)

	res, ok = c.get("true")
	if !res.IsIgnored() || !res.IsDeletable() || !ok {
		t.Errorf("res %v, ok %v for true item", res, ok)
	}

	res, ok = c.get("false")
	if res.IsIgnored() || res.IsDeletable() || !ok {
		t.Errorf("res %v, ok %v for false item", res, ok)
	}

	// Don't clean anything

	c.clean(time.Second)

	// Same values should exist

	res, ok = c.get("true")
	if !res.IsIgnored() || !res.IsDeletable() || !ok {
		t.Errorf("res %v, ok %v for true item", res, ok)
	}

	res, ok = c.get("false")
	if res.IsIgnored() || res.IsDeletable() || !ok {
		t.Errorf("res %v, ok %v for false item", res, ok)
	}

	// Sleep and access, to get some data for clean

	*fc += 500 // milliseconds

	c.get("true")

	*fc += 100 // milliseconds

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

type fakeClock int64 // milliseconds

func (f *fakeClock) Now() time.Time {
	t := time.Unix(int64(*f)/1000, (int64(*f)%1000)*int64(time.Millisecond))
	*f++
	return t
}
