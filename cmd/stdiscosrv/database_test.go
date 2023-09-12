// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestDatabaseGetSet(t *testing.T) {
	db, err := newMemoryLevelDBStore()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go db.Serve(ctx)
	defer cancel()

	// Check missing record

	rec, err := db.get("abcd")
	if err != nil {
		t.Error("not found should not be an error")
	}
	if len(rec.Addresses) != 0 {
		t.Error("addresses should be empty")
	}
	if rec.Misses != 0 {
		t.Error("missing should be zero")
	}

	// Set up a clock

	now := time.Now()
	tc := &testClock{now}
	db.clock = tc

	// Put a record

	rec.Addresses = []DatabaseAddress{
		{Address: "tcp://1.2.3.4:5", Expires: tc.Now().Add(time.Minute).UnixNano()},
	}
	if err := db.put("abcd", rec); err != nil {
		t.Fatal(err)
	}

	// Verify it

	rec, err = db.get("abcd")
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.Addresses) != 1 {
		t.Log(rec.Addresses)
		t.Fatal("should have one address")
	}
	if rec.Addresses[0].Address != "tcp://1.2.3.4:5" {
		t.Log(rec.Addresses)
		t.Error("incorrect address")
	}

	// Wind the clock one half expiry, and merge in a new address

	tc.wind(30 * time.Second)

	addrs := []DatabaseAddress{
		{Address: "tcp://6.7.8.9:0", Expires: tc.Now().Add(time.Minute).UnixNano()},
	}
	if err := db.merge("abcd", addrs, tc.Now().UnixNano()); err != nil {
		t.Fatal(err)
	}

	// Verify it

	rec, err = db.get("abcd")
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.Addresses) != 2 {
		t.Log(rec.Addresses)
		t.Fatal("should have two addresses")
	}
	if rec.Addresses[0].Address != "tcp://1.2.3.4:5" {
		t.Log(rec.Addresses)
		t.Error("incorrect address[0]")
	}
	if rec.Addresses[1].Address != "tcp://6.7.8.9:0" {
		t.Log(rec.Addresses)
		t.Error("incorrect address[1]")
	}

	// Pass the first expiry time

	tc.wind(45 * time.Second)

	// Verify it

	rec, err = db.get("abcd")
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.Addresses) != 1 {
		t.Log(rec.Addresses)
		t.Fatal("should have one address")
	}
	if rec.Addresses[0].Address != "tcp://6.7.8.9:0" {
		t.Log(rec.Addresses)
		t.Error("incorrect address")
	}

	// Put a record with misses

	rec = DatabaseRecord{Misses: 42, Missed: tc.Now().UnixNano()}
	if err := db.put("efgh", rec); err != nil {
		t.Fatal(err)
	}

	// Verify it

	rec, err = db.get("efgh")
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.Addresses) != 0 {
		t.Log(rec.Addresses)
		t.Fatal("should have no addresses")
	}
	if rec.Misses != 42 {
		t.Log(rec.Misses)
		t.Error("incorrect misses")
	}

	// Set an address

	addrs = []DatabaseAddress{
		{Address: "tcp://6.7.8.9:0", Expires: tc.Now().Add(time.Minute).UnixNano()},
	}
	if err := db.merge("efgh", addrs, tc.Now().UnixNano()); err != nil {
		t.Fatal(err)
	}

	// Verify it

	rec, err = db.get("efgh")
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.Addresses) != 1 {
		t.Log(rec.Addresses)
		t.Fatal("should have one address")
	}
	if rec.Misses != 0 {
		t.Log(rec.Misses)
		t.Error("should have no misses")
	}
}

func TestFilter(t *testing.T) {
	// all cases are expired with t=10
	cases := []struct {
		a []DatabaseAddress
		b []DatabaseAddress
	}{
		{
			a: nil,
			b: nil,
		},
		{
			a: []DatabaseAddress{{Address: "a", Expires: 9}, {Address: "b", Expires: 9}, {Address: "c", Expires: 9}},
			b: []DatabaseAddress{},
		},
		{
			a: []DatabaseAddress{{Address: "a", Expires: 10}},
			b: []DatabaseAddress{{Address: "a", Expires: 10}},
		},
		{
			a: []DatabaseAddress{{Address: "a", Expires: 10}, {Address: "b", Expires: 10}, {Address: "c", Expires: 10}},
			b: []DatabaseAddress{{Address: "a", Expires: 10}, {Address: "b", Expires: 10}, {Address: "c", Expires: 10}},
		},
		{
			a: []DatabaseAddress{{Address: "a", Expires: 5}, {Address: "b", Expires: 15}, {Address: "c", Expires: 5}, {Address: "d", Expires: 15}, {Address: "e", Expires: 5}},
			b: []DatabaseAddress{{Address: "b", Expires: 15}, {Address: "d", Expires: 15}},
		},
	}

	for _, tc := range cases {
		res := expire(tc.a, 10)
		if fmt.Sprint(res) != fmt.Sprint(tc.b) {
			t.Errorf("Incorrect result %v, expected %v", res, tc.b)
		}
	}
}

type testClock struct {
	now time.Time
}

func (t *testClock) wind(d time.Duration) {
	t.now = t.now.Add(d)
}

func (t *testClock) Now() time.Time {
	t.now = t.now.Add(time.Nanosecond)
	return t.now
}
