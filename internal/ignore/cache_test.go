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

	time.Sleep(100 * time.Millisecond)
	c.get("true")
	time.Sleep(100 * time.Millisecond)

	// "false" was accessed 200 ms ago, "true" was accessed 100 ms ago.
	// This should clean out "false" but not "true"

	c.clean(150 * time.Millisecond)

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
