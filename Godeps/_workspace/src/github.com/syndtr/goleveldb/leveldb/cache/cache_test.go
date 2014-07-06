// Copyright (c) 2012, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cache

import (
	"math/rand"
	"testing"
)

func set(ns Namespace, key uint64, value interface{}, charge int, fin func()) Object {
	obj, _ := ns.Get(key, func() (bool, interface{}, int, SetFin) {
		return true, value, charge, fin
	})
	return obj
}

func TestCache_HitMiss(t *testing.T) {
	cases := []struct {
		key   uint64
		value string
	}{
		{1, "vvvvvvvvv"},
		{100, "v1"},
		{0, "v2"},
		{12346, "v3"},
		{777, "v4"},
		{999, "v5"},
		{7654, "v6"},
		{2, "v7"},
		{3, "v8"},
		{9, "v9"},
	}

	setfin := 0
	c := NewLRUCache(1000)
	ns := c.GetNamespace(0)
	for i, x := range cases {
		set(ns, x.key, x.value, len(x.value), func() {
			setfin++
		}).Release()
		for j, y := range cases {
			r, ok := ns.Get(y.key, nil)
			if j <= i {
				// should hit
				if !ok {
					t.Errorf("case '%d' iteration '%d' is miss", i, j)
				} else if r.Value().(string) != y.value {
					t.Errorf("case '%d' iteration '%d' has invalid value got '%s', want '%s'", i, j, r.Value().(string), y.value)
				}
			} else {
				// should miss
				if ok {
					t.Errorf("case '%d' iteration '%d' is hit , value '%s'", i, j, r.Value().(string))
				}
			}
			if ok {
				r.Release()
			}
		}
	}

	for i, x := range cases {
		finalizerOk := false
		ns.Delete(x.key, func(exist bool) {
			finalizerOk = true
		})

		if !finalizerOk {
			t.Errorf("case %d delete finalizer not executed", i)
		}

		for j, y := range cases {
			r, ok := ns.Get(y.key, nil)
			if j > i {
				// should hit
				if !ok {
					t.Errorf("case '%d' iteration '%d' is miss", i, j)
				} else if r.Value().(string) != y.value {
					t.Errorf("case '%d' iteration '%d' has invalid value got '%s', want '%s'", i, j, r.Value().(string), y.value)
				}
			} else {
				// should miss
				if ok {
					t.Errorf("case '%d' iteration '%d' is hit, value '%s'", i, j, r.Value().(string))
				}
			}
			if ok {
				r.Release()
			}
		}
	}

	if setfin != len(cases) {
		t.Errorf("some set finalizer may not be executed, want=%d got=%d", len(cases), setfin)
	}
}

func TestLRUCache_Eviction(t *testing.T) {
	c := NewLRUCache(12)
	ns := c.GetNamespace(0)
	o1 := set(ns, 1, 1, 1, nil)
	set(ns, 2, 2, 1, nil).Release()
	set(ns, 3, 3, 1, nil).Release()
	set(ns, 4, 4, 1, nil).Release()
	set(ns, 5, 5, 1, nil).Release()
	if r, ok := ns.Get(2, nil); ok { // 1,3,4,5,2
		r.Release()
	}
	set(ns, 9, 9, 10, nil).Release() // 5,2,9

	for _, x := range []uint64{9, 2, 5, 1} {
		r, ok := ns.Get(x, nil)
		if !ok {
			t.Errorf("miss for key '%d'", x)
		} else {
			if r.Value().(int) != int(x) {
				t.Errorf("invalid value for key '%d' want '%d', got '%d'", x, x, r.Value().(int))
			}
			r.Release()
		}
	}
	o1.Release()
	for _, x := range []uint64{1, 2, 5} {
		r, ok := ns.Get(x, nil)
		if !ok {
			t.Errorf("miss for key '%d'", x)
		} else {
			if r.Value().(int) != int(x) {
				t.Errorf("invalid value for key '%d' want '%d', got '%d'", x, x, r.Value().(int))
			}
			r.Release()
		}
	}
	for _, x := range []uint64{3, 4, 9} {
		r, ok := ns.Get(x, nil)
		if ok {
			t.Errorf("hit for key '%d'", x)
			if r.Value().(int) != int(x) {
				t.Errorf("invalid value for key '%d' want '%d', got '%d'", x, x, r.Value().(int))
			}
			r.Release()
		}
	}
}

func TestLRUCache_SetGet(t *testing.T) {
	c := NewLRUCache(13)
	ns := c.GetNamespace(0)
	for i := 0; i < 200; i++ {
		n := uint64(rand.Intn(99999) % 20)
		set(ns, n, n, 1, nil).Release()
		if p, ok := ns.Get(n, nil); ok {
			if p.Value() == nil {
				t.Errorf("key '%d' contains nil value", n)
			} else {
				got := p.Value().(uint64)
				if got != n {
					t.Errorf("invalid value for key '%d' want '%d', got '%d'", n, n, got)
				}
			}
			p.Release()
		} else {
			t.Errorf("key '%d' doesn't exist", n)
		}
	}
}

func TestLRUCache_Purge(t *testing.T) {
	c := NewLRUCache(3)
	ns1 := c.GetNamespace(0)
	o1 := set(ns1, 1, 1, 1, nil)
	o2 := set(ns1, 2, 2, 1, nil)
	ns1.Purge(nil)
	set(ns1, 3, 3, 1, nil).Release()
	for _, x := range []uint64{1, 2, 3} {
		r, ok := ns1.Get(x, nil)
		if !ok {
			t.Errorf("miss for key '%d'", x)
		} else {
			if r.Value().(int) != int(x) {
				t.Errorf("invalid value for key '%d' want '%d', got '%d'", x, x, r.Value().(int))
			}
			r.Release()
		}
	}
	o1.Release()
	o2.Release()
	for _, x := range []uint64{1, 2} {
		r, ok := ns1.Get(x, nil)
		if ok {
			t.Errorf("hit for key '%d'", x)
			if r.Value().(int) != int(x) {
				t.Errorf("invalid value for key '%d' want '%d', got '%d'", x, x, r.Value().(int))
			}
			r.Release()
		}
	}
}

func BenchmarkLRUCache_SetRelease(b *testing.B) {
	capacity := b.N / 100
	if capacity <= 0 {
		capacity = 10
	}
	c := NewLRUCache(capacity)
	ns := c.GetNamespace(0)
	b.ResetTimer()
	for i := uint64(0); i < uint64(b.N); i++ {
		set(ns, i, nil, 1, nil).Release()
	}
}

func BenchmarkLRUCache_SetReleaseTwice(b *testing.B) {
	capacity := b.N / 100
	if capacity <= 0 {
		capacity = 10
	}
	c := NewLRUCache(capacity)
	ns := c.GetNamespace(0)
	b.ResetTimer()

	na := b.N / 2
	nb := b.N - na

	for i := uint64(0); i < uint64(na); i++ {
		set(ns, i, nil, 1, nil).Release()
	}

	for i := uint64(0); i < uint64(nb); i++ {
		set(ns, i, nil, 1, nil).Release()
	}
}
