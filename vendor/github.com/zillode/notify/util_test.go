// Copyright (c) 2014-2015 The Notify Authors. All rights reserved.
// Use of this source code is governed by the MIT license that can be
// found in the LICENSE file.

package notify

import (
	"path/filepath"
	"testing"
)

type caseCanonical struct {
	path string
	full string
}

func testCanonical(t *testing.T, cases []caseCanonical) {
	for i, cas := range cases {
		full, err := canonical(cas.path)
		if err != nil {
			t.Errorf("want err=nil; got %v (i=%d)", err, i)
			continue
		}
		if full != cas.full {
			t.Errorf("want full=%q; got %q (i=%d)", cas.full, full, i)
			continue
		}
	}
}

func TestCanonicalNoSymlink(t *testing.T) {
	td := filepath.Join(wd, "testdata")
	cases := [...]caseCanonical{
		{".", wd},
		{"testdata", td},
		{filepath.Join("testdata", ".."), wd},
	}
	testCanonical(t, cases[:])
}

func TestJoinevents(t *testing.T) {
	cases := [...]struct {
		evs []Event
		ev  Event
	}{
		0: {nil, All},
		1: {[]Event{}, All},
		2: {[]Event{Create}, Create},
		3: {[]Event{Rename}, Rename},
		4: {[]Event{Create, Write, Remove}, Create | Write | Remove},
	}
	for i, cas := range cases {
		if ev := joinevents(cas.evs); ev != cas.ev {
			t.Errorf("want event=%v; got %v (i=%d)", cas.ev, ev, i)
		}
	}
}

func TestTreeSplit(t *testing.T) {
	cases := [...]struct {
		path string
		dir  string
		base string
	}{
		{"/github.com/rjeczalik/fakerpc", "/github.com/rjeczalik", "fakerpc"},
		{"/home/rjeczalik/src", "/home/rjeczalik", "src"},
		{"/Users/pknap/porn/gopher.avi", "/Users/pknap/porn", "gopher.avi"},
		{"C:/Documents and Users", "C:", "Documents and Users"},
		{"C:/Documents and Users/pblaszczyk/wiertarka.exe", "C:/Documents and Users/pblaszczyk", "wiertarka.exe"},
		{"/home/(╯°□°）╯︵ ┻━┻", "/home", "(╯°□°）╯︵ ┻━┻"},
	}
	for i, cas := range cases {
		dir, base := split(filepath.FromSlash(cas.path))
		if want := filepath.FromSlash(cas.dir); dir != want {
			t.Errorf("want dir=%s; got %s (i=%d)", want, dir, i)
		}
		if want := filepath.FromSlash(cas.base); base != want {
			t.Errorf("want base=%s; got %s (i=%d)", want, base, i)
		}
	}
}

func TestTreeBase(t *testing.T) {
	cases := [...]struct {
		path string
		base string
	}{
		{"/github.com/rjeczalik/fakerpc", "fakerpc"},
		{"/home/rjeczalik/src", "src"},
		{"/Users/pknap/porn/gopher.avi", "gopher.avi"},
		{"C:/Documents and Users", "Documents and Users"},
		{"C:/Documents and Users/pblaszczyk/wiertarka.exe", "wiertarka.exe"},
		{"/home/(╯°□°）╯︵ ┻━┻", "(╯°□°）╯︵ ┻━┻"},
	}
	for i, cas := range cases {
		if base := base(filepath.FromSlash(cas.path)); base != cas.base {
			t.Errorf("want base=%s; got %s (i=%d)", cas.base, base, i)
		}
	}
}

func TestTreeIndexSep(t *testing.T) {
	cases := [...]struct {
		path string
		n    int
	}{
		{"github.com/rjeczalik/fakerpc", 10},
		{"home/rjeczalik/src", 4},
		{"Users/pknap/porn/gopher.avi", 5},
		{"C:/Documents and Users", 2},
		{"Documents and Users/pblaszczyk/wiertarka.exe", 19},
		{"(╯°□°）╯︵ ┻━┻/Downloads", 30},
	}
	for i, cas := range cases {
		if n := indexSep(filepath.FromSlash(cas.path)); n != cas.n {
			t.Errorf("want n=%d; got %d (i=%d)", cas.n, n, i)
		}
	}
}

func TestTreeLastIndexSep(t *testing.T) {
	cases := [...]struct {
		path string
		n    int
	}{
		{"github.com/rjeczalik/fakerpc", 20},
		{"home/rjeczalik/src", 14},
		{"Users/pknap/porn/gopher.avi", 16},
		{"C:/Documents and Users", 2},
		{"Documents and Users/pblaszczyk/wiertarka.exe", 30},
		{"/home/(╯°□°）╯︵ ┻━┻", 5},
	}
	for i, cas := range cases {
		if n := lastIndexSep(filepath.FromSlash(cas.path)); n != cas.n {
			t.Errorf("want n=%d; got %d (i=%d)", cas.n, n, i)
		}
	}
}

func TestCleanpath(t *testing.T) {
	t.Skip("TODO(rjeczalik)")
}
