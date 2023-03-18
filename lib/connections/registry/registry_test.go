// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package registry

import (
	"net"
	"testing"
)

func TestRegistry(t *testing.T) {
	r := New()

	want := func(i int) func(interface{}) bool {
		return func(x interface{}) bool { return x.(int) == i }
	}

	if res := r.Get("int", want(1)); res != nil {
		t.Error("unexpected")
	}

	r.Register("int", 1)
	r.Register("int", 11)
	r.Register("int4", 4)
	r.Register("int4", 44)
	r.Register("int6", 6)
	r.Register("int6", 66)

	if res := r.Get("int", want(1)).(int); res != 1 {
		t.Error("unexpected", res)
	}

	// int is prefix of int4, so returns 1
	if res := r.Get("int4", want(1)).(int); res != 1 {
		t.Error("unexpected", res)
	}

	r.Unregister("int", 1)

	if res := r.Get("int", want(1)).(int); res == 1 {
		t.Error("unexpected", res)
	}

	if res := r.Get("int6", want(6)).(int); res != 6 {
		t.Error("unexpected", res)
	}

	// Unregister 11, int should be impossible to find
	r.Unregister("int", 11)
	if res := r.Get("int", want(11)); res != nil {
		t.Error("unexpected")
	}

	// Unregister a second time does nothing.
	r.Unregister("int", 1)

	// Can have multiple of the same
	r.Register("int", 1)
	r.Register("int", 1)
	r.Unregister("int", 1)

	if res := r.Get("int4", want(1)).(int); res != 1 {
		t.Error("unexpected", res)
	}
}

func TestShortSchemeFirst(t *testing.T) {
	r := New()
	r.Register("foo", 0)
	r.Register("foobar", 1)

	// If we don't care about the value, we should get the one with "foo".
	res := r.Get("foo", func(interface{}) bool { return false })
	if res != 0 {
		t.Error("unexpected", res)
	}
}

func BenchmarkGet(b *testing.B) {
	r := New()
	for _, addr := range []string{"192.168.1.1", "172.1.1.1", "10.1.1.1"} {
		r.Register("tcp", &net.TCPAddr{IP: net.ParseIP(addr)})
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		r.Get("tcp", func(x interface{}) bool {
			return x.(*net.TCPAddr).IP.IsUnspecified()
		})
	}
}
