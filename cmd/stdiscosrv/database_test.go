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

	"github.com/syncthing/syncthing/internal/gen/discosrv"
	"github.com/syncthing/syncthing/lib/protocol"
)

func TestDatabaseGetSet(t *testing.T) {
	db := newInMemoryStore(t.TempDir(), 0, nil)
	ctx, cancel := context.WithCancel(context.Background())
	go db.Serve(ctx)
	defer cancel()

	// Check missing record

	rec, err := db.get(&protocol.EmptyDeviceID)
	if err != nil {
		t.Error("not found should not be an error")
	}
	if len(rec.Addresses) != 0 {
		t.Error("addresses should be empty")
	}

	// Set up a clock

	now := time.Now()
	tc := &testClock{now}
	db.clock = tc

	// Put a record

	rec.Addresses = []*discosrv.DatabaseAddress{
		{Address: "tcp://1.2.3.4:5", Expires: tc.Now().Add(time.Minute).UnixNano()},
	}
	if err := db.put(&protocol.EmptyDeviceID, rec); err != nil {
		t.Fatal(err)
	}

	// Verify it

	rec, err = db.get(&protocol.EmptyDeviceID)
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

	addrs := []*discosrv.DatabaseAddress{
		{Address: "tcp://6.7.8.9:0", Expires: tc.Now().Add(time.Minute).UnixNano()},
	}
	if err := db.merge(&protocol.EmptyDeviceID, addrs, tc.Now().UnixNano()); err != nil {
		t.Fatal(err)
	}

	// Verify it

	rec, err = db.get(&protocol.EmptyDeviceID)
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

	rec, err = db.get(&protocol.EmptyDeviceID)
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

	// Set an address

	addrs = []*discosrv.DatabaseAddress{
		{Address: "tcp://6.7.8.9:0", Expires: tc.Now().Add(time.Minute).UnixNano()},
	}
	if err := db.merge(&protocol.GlobalDeviceID, addrs, tc.Now().UnixNano()); err != nil {
		t.Fatal(err)
	}

	// Verify it

	rec, err = db.get(&protocol.GlobalDeviceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.Addresses) != 1 {
		t.Log(rec.Addresses)
		t.Fatal("should have one address")
	}
}

func TestFilter(t *testing.T) {
	// all cases are expired with t=10
	cases := []struct {
		a []*discosrv.DatabaseAddress
		b []*discosrv.DatabaseAddress
	}{
		{
			a: nil,
			b: nil,
		},
		{
			a: []*discosrv.DatabaseAddress{{Address: "a", Expires: 9}, {Address: "b", Expires: 9}, {Address: "c", Expires: 9}},
			b: []*discosrv.DatabaseAddress{},
		},
		{
			a: []*discosrv.DatabaseAddress{{Address: "a", Expires: 10}},
			b: []*discosrv.DatabaseAddress{{Address: "a", Expires: 10}},
		},
		{
			a: []*discosrv.DatabaseAddress{{Address: "a", Expires: 10}, {Address: "b", Expires: 10}, {Address: "c", Expires: 10}},
			b: []*discosrv.DatabaseAddress{{Address: "a", Expires: 10}, {Address: "b", Expires: 10}, {Address: "c", Expires: 10}},
		},
		{
			a: []*discosrv.DatabaseAddress{{Address: "a", Expires: 5}, {Address: "b", Expires: 15}, {Address: "c", Expires: 5}, {Address: "d", Expires: 15}, {Address: "e", Expires: 5}},
			b: []*discosrv.DatabaseAddress{{Address: "b", Expires: 15}, {Address: "d", Expires: 15}},
		},
	}

	for _, tc := range cases {
		res := expire(tc.a, time.Unix(0, 10))
		if fmt.Sprint(res) != fmt.Sprint(tc.b) {
			t.Errorf("Incorrect result %v, expected %v", res, tc.b)
		}
	}
}

func TestMerge(t *testing.T) {
	cases := []struct {
		a, b, res []*discosrv.DatabaseAddress
	}{
		{nil, nil, nil},
		{
			nil,
			[]*discosrv.DatabaseAddress{{Address: "a", Expires: 10}},
			[]*discosrv.DatabaseAddress{{Address: "a", Expires: 10}},
		},
		{
			nil,
			[]*discosrv.DatabaseAddress{{Address: "a", Expires: 10}, {Address: "b", Expires: 10}, {Address: "c", Expires: 10}},
			[]*discosrv.DatabaseAddress{{Address: "a", Expires: 10}, {Address: "b", Expires: 10}, {Address: "c", Expires: 10}},
		},
		{
			[]*discosrv.DatabaseAddress{{Address: "a", Expires: 10}},
			[]*discosrv.DatabaseAddress{{Address: "a", Expires: 15}},
			[]*discosrv.DatabaseAddress{{Address: "a", Expires: 15}},
		},
		{
			[]*discosrv.DatabaseAddress{{Address: "a", Expires: 10}},
			[]*discosrv.DatabaseAddress{{Address: "b", Expires: 15}},
			[]*discosrv.DatabaseAddress{{Address: "a", Expires: 10}, {Address: "b", Expires: 15}},
		},
		{
			[]*discosrv.DatabaseAddress{{Address: "a", Expires: 10}, {Address: "b", Expires: 15}},
			[]*discosrv.DatabaseAddress{{Address: "a", Expires: 15}, {Address: "b", Expires: 15}},
			[]*discosrv.DatabaseAddress{{Address: "a", Expires: 15}, {Address: "b", Expires: 15}},
		},
		{
			[]*discosrv.DatabaseAddress{{Address: "a", Expires: 10}, {Address: "b", Expires: 15}},
			[]*discosrv.DatabaseAddress{{Address: "b", Expires: 15}, {Address: "c", Expires: 20}},
			[]*discosrv.DatabaseAddress{{Address: "a", Expires: 10}, {Address: "b", Expires: 15}, {Address: "c", Expires: 20}},
		},
		{
			[]*discosrv.DatabaseAddress{{Address: "a", Expires: 10}, {Address: "b", Expires: 15}},
			[]*discosrv.DatabaseAddress{{Address: "b", Expires: 5}, {Address: "c", Expires: 20}},
			[]*discosrv.DatabaseAddress{{Address: "a", Expires: 10}, {Address: "b", Expires: 15}, {Address: "c", Expires: 20}},
		},
		{
			[]*discosrv.DatabaseAddress{{Address: "y", Expires: 10}, {Address: "z", Expires: 10}},
			[]*discosrv.DatabaseAddress{{Address: "a", Expires: 5}, {Address: "b", Expires: 15}},
			[]*discosrv.DatabaseAddress{{Address: "a", Expires: 5}, {Address: "b", Expires: 15}, {Address: "y", Expires: 10}, {Address: "z", Expires: 10}},
		},
		{
			[]*discosrv.DatabaseAddress{{Address: "a", Expires: 10}, {Address: "b", Expires: 15}, {Address: "d", Expires: 10}},
			[]*discosrv.DatabaseAddress{{Address: "b", Expires: 5}, {Address: "c", Expires: 20}},
			[]*discosrv.DatabaseAddress{{Address: "a", Expires: 10}, {Address: "b", Expires: 15}, {Address: "c", Expires: 20}, {Address: "d", Expires: 10}},
		},
	}

	for _, tc := range cases {
		rec := merge(&discosrv.DatabaseRecord{Addresses: tc.a}, &discosrv.DatabaseRecord{Addresses: tc.b})
		if fmt.Sprint(rec.Addresses) != fmt.Sprint(tc.res) {
			t.Errorf("Incorrect result %v, expected %v", rec.Addresses, tc.res)
		}
		rec = merge(&discosrv.DatabaseRecord{Addresses: tc.b}, &discosrv.DatabaseRecord{Addresses: tc.a})
		if fmt.Sprint(rec.Addresses) != fmt.Sprint(tc.res) {
			t.Errorf("Incorrect result %v, expected %v", rec.Addresses, tc.res)
		}
	}
}

func BenchmarkMergeEqual(b *testing.B) {
	for i := 0; i < b.N; i++ {
		ar := []*discosrv.DatabaseAddress{{Address: "a", Expires: 10}, {Address: "b", Expires: 15}}
		br := []*discosrv.DatabaseAddress{{Address: "a", Expires: 15}, {Address: "b", Expires: 10}}
		res := merge(&discosrv.DatabaseRecord{Addresses: ar}, &discosrv.DatabaseRecord{Addresses: br})
		if len(res.Addresses) != 2 {
			b.Fatal("wrong length")
		}
		if res.Addresses[0].Address != "a" || res.Addresses[1].Address != "b" {
			b.Fatal("wrong address")
		}
		if res.Addresses[0].Expires != 15 || res.Addresses[1].Expires != 15 {
			b.Fatal("wrong expiry")
		}
	}
	b.ReportAllocs() // should be zero per operation
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
