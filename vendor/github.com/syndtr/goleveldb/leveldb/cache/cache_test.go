// Copyright (c) 2012, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cache

import (
	"math/rand"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"
)

type int32o int32

func (o *int32o) acquire() {
	if atomic.AddInt32((*int32)(o), 1) != 1 {
		panic("BUG: invalid ref")
	}
}

func (o *int32o) Release() {
	if atomic.AddInt32((*int32)(o), -1) != 0 {
		panic("BUG: invalid ref")
	}
}

type releaserFunc struct {
	fn    func()
	value Value
}

func (r releaserFunc) Release() {
	if r.fn != nil {
		r.fn()
	}
}

func set(c *Cache, ns, key uint64, value Value, charge int, relf func()) *Handle {
	return c.Get(ns, key, func() (int, Value) {
		if relf != nil {
			return charge, releaserFunc{relf, value}
		}
		return charge, value
	})
}

type cacheMapTestParams struct {
	nobjects, nhandles, concurrent, repeat int
}

func TestCacheMap(t *testing.T) {
	runtime.GOMAXPROCS(runtime.NumCPU())

	var params []cacheMapTestParams
	if testing.Short() {
		params = []cacheMapTestParams{
			{1000, 100, 20, 3},
			{10000, 300, 50, 10},
		}
	} else {
		params = []cacheMapTestParams{
			{10000, 400, 50, 3},
			{100000, 1000, 100, 10},
		}
	}

	var (
		objects [][]int32o
		handles [][]unsafe.Pointer
	)

	for _, x := range params {
		objects = append(objects, make([]int32o, x.nobjects))
		handles = append(handles, make([]unsafe.Pointer, x.nhandles))
	}

	c := NewCache(nil)

	wg := new(sync.WaitGroup)
	var done int32

	for ns, x := range params {
		for i := 0; i < x.concurrent; i++ {
			wg.Add(1)
			go func(ns, i, repeat int, objects []int32o, handles []unsafe.Pointer) {
				defer wg.Done()
				r := rand.New(rand.NewSource(time.Now().UnixNano()))

				for j := len(objects) * repeat; j >= 0; j-- {
					key := uint64(r.Intn(len(objects)))
					h := c.Get(uint64(ns), key, func() (int, Value) {
						o := &objects[key]
						o.acquire()
						return 1, o
					})
					if v := h.Value().(*int32o); v != &objects[key] {
						t.Fatalf("#%d invalid value: want=%p got=%p", ns, &objects[key], v)
					}
					if objects[key] != 1 {
						t.Fatalf("#%d invalid object %d: %d", ns, key, objects[key])
					}
					if !atomic.CompareAndSwapPointer(&handles[r.Intn(len(handles))], nil, unsafe.Pointer(h)) {
						h.Release()
					}
				}
			}(ns, i, x.repeat, objects[ns], handles[ns])
		}

		go func(handles []unsafe.Pointer) {
			r := rand.New(rand.NewSource(time.Now().UnixNano()))

			for atomic.LoadInt32(&done) == 0 {
				i := r.Intn(len(handles))
				h := (*Handle)(atomic.LoadPointer(&handles[i]))
				if h != nil && atomic.CompareAndSwapPointer(&handles[i], unsafe.Pointer(h), nil) {
					h.Release()
				}
				time.Sleep(time.Millisecond)
			}
		}(handles[ns])
	}

	go func() {
		handles := make([]*Handle, 100000)
		for atomic.LoadInt32(&done) == 0 {
			for i := range handles {
				handles[i] = c.Get(999999999, uint64(i), func() (int, Value) {
					return 1, 1
				})
			}
			for _, h := range handles {
				h.Release()
			}
		}
	}()

	wg.Wait()

	atomic.StoreInt32(&done, 1)

	for _, handles0 := range handles {
		for i := range handles0 {
			h := (*Handle)(atomic.LoadPointer(&handles0[i]))
			if h != nil && atomic.CompareAndSwapPointer(&handles0[i], unsafe.Pointer(h), nil) {
				h.Release()
			}
		}
	}

	for ns, objects0 := range objects {
		for i, o := range objects0 {
			if o != 0 {
				t.Fatalf("invalid object #%d.%d: ref=%d", ns, i, o)
			}
		}
	}
}

func TestCacheMap_NodesAndSize(t *testing.T) {
	c := NewCache(nil)
	if c.Nodes() != 0 {
		t.Errorf("invalid nodes counter: want=%d got=%d", 0, c.Nodes())
	}
	if c.Size() != 0 {
		t.Errorf("invalid size counter: want=%d got=%d", 0, c.Size())
	}
	set(c, 0, 1, 1, 1, nil)
	set(c, 0, 2, 2, 2, nil)
	set(c, 1, 1, 3, 3, nil)
	set(c, 2, 1, 4, 1, nil)
	if c.Nodes() != 4 {
		t.Errorf("invalid nodes counter: want=%d got=%d", 4, c.Nodes())
	}
	if c.Size() != 7 {
		t.Errorf("invalid size counter: want=%d got=%d", 4, c.Size())
	}
}

func TestLRUCache_Capacity(t *testing.T) {
	c := NewCache(NewLRU(10))
	if c.Capacity() != 10 {
		t.Errorf("invalid capacity: want=%d got=%d", 10, c.Capacity())
	}
	set(c, 0, 1, 1, 1, nil).Release()
	set(c, 0, 2, 2, 2, nil).Release()
	set(c, 1, 1, 3, 3, nil).Release()
	set(c, 2, 1, 4, 1, nil).Release()
	set(c, 2, 2, 5, 1, nil).Release()
	set(c, 2, 3, 6, 1, nil).Release()
	set(c, 2, 4, 7, 1, nil).Release()
	set(c, 2, 5, 8, 1, nil).Release()
	if c.Nodes() != 7 {
		t.Errorf("invalid nodes counter: want=%d got=%d", 7, c.Nodes())
	}
	if c.Size() != 10 {
		t.Errorf("invalid size counter: want=%d got=%d", 10, c.Size())
	}
	c.SetCapacity(9)
	if c.Capacity() != 9 {
		t.Errorf("invalid capacity: want=%d got=%d", 9, c.Capacity())
	}
	if c.Nodes() != 6 {
		t.Errorf("invalid nodes counter: want=%d got=%d", 6, c.Nodes())
	}
	if c.Size() != 8 {
		t.Errorf("invalid size counter: want=%d got=%d", 8, c.Size())
	}
}

func TestCacheMap_NilValue(t *testing.T) {
	c := NewCache(NewLRU(10))
	h := c.Get(0, 0, func() (size int, value Value) {
		return 1, nil
	})
	if h != nil {
		t.Error("cache handle is non-nil")
	}
	if c.Nodes() != 0 {
		t.Errorf("invalid nodes counter: want=%d got=%d", 0, c.Nodes())
	}
	if c.Size() != 0 {
		t.Errorf("invalid size counter: want=%d got=%d", 0, c.Size())
	}
}

func TestLRUCache_GetLatency(t *testing.T) {
	runtime.GOMAXPROCS(runtime.NumCPU())

	const (
		concurrentSet = 30
		concurrentGet = 3
		duration      = 3 * time.Second
		delay         = 3 * time.Millisecond
		maxkey        = 100000
	)

	var (
		set, getHit, getAll        int32
		getMaxLatency, getDuration int64
	)

	c := NewCache(NewLRU(5000))
	wg := &sync.WaitGroup{}
	until := time.Now().Add(duration)
	for i := 0; i < concurrentSet; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r := rand.New(rand.NewSource(time.Now().UnixNano()))
			for time.Now().Before(until) {
				c.Get(0, uint64(r.Intn(maxkey)), func() (int, Value) {
					time.Sleep(delay)
					atomic.AddInt32(&set, 1)
					return 1, 1
				}).Release()
			}
		}(i)
	}
	for i := 0; i < concurrentGet; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r := rand.New(rand.NewSource(time.Now().UnixNano()))
			for {
				mark := time.Now()
				if mark.Before(until) {
					h := c.Get(0, uint64(r.Intn(maxkey)), nil)
					latency := int64(time.Now().Sub(mark))
					m := atomic.LoadInt64(&getMaxLatency)
					if latency > m {
						atomic.CompareAndSwapInt64(&getMaxLatency, m, latency)
					}
					atomic.AddInt64(&getDuration, latency)
					if h != nil {
						atomic.AddInt32(&getHit, 1)
						h.Release()
					}
					atomic.AddInt32(&getAll, 1)
				} else {
					break
				}
			}
		}(i)
	}

	wg.Wait()
	getAvglatency := time.Duration(getDuration) / time.Duration(getAll)
	t.Logf("set=%d getHit=%d getAll=%d getMaxLatency=%v getAvgLatency=%v",
		set, getHit, getAll, time.Duration(getMaxLatency), getAvglatency)

	if getAvglatency > delay/3 {
		t.Errorf("get avg latency > %v: got=%v", delay/3, getAvglatency)
	}
}

func TestLRUCache_HitMiss(t *testing.T) {
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
	c := NewCache(NewLRU(1000))
	for i, x := range cases {
		set(c, 0, x.key, x.value, len(x.value), func() {
			setfin++
		}).Release()
		for j, y := range cases {
			h := c.Get(0, y.key, nil)
			if j <= i {
				// should hit
				if h == nil {
					t.Errorf("case '%d' iteration '%d' is miss", i, j)
				} else {
					if x := h.Value().(releaserFunc).value.(string); x != y.value {
						t.Errorf("case '%d' iteration '%d' has invalid value got '%s', want '%s'", i, j, x, y.value)
					}
				}
			} else {
				// should miss
				if h != nil {
					t.Errorf("case '%d' iteration '%d' is hit , value '%s'", i, j, h.Value().(releaserFunc).value.(string))
				}
			}
			if h != nil {
				h.Release()
			}
		}
	}

	for i, x := range cases {
		finalizerOk := false
		c.Delete(0, x.key, func() {
			finalizerOk = true
		})

		if !finalizerOk {
			t.Errorf("case %d delete finalizer not executed", i)
		}

		for j, y := range cases {
			h := c.Get(0, y.key, nil)
			if j > i {
				// should hit
				if h == nil {
					t.Errorf("case '%d' iteration '%d' is miss", i, j)
				} else {
					if x := h.Value().(releaserFunc).value.(string); x != y.value {
						t.Errorf("case '%d' iteration '%d' has invalid value got '%s', want '%s'", i, j, x, y.value)
					}
				}
			} else {
				// should miss
				if h != nil {
					t.Errorf("case '%d' iteration '%d' is hit, value '%s'", i, j, h.Value().(releaserFunc).value.(string))
				}
			}
			if h != nil {
				h.Release()
			}
		}
	}

	if setfin != len(cases) {
		t.Errorf("some set finalizer may not be executed, want=%d got=%d", len(cases), setfin)
	}
}

func TestLRUCache_Eviction(t *testing.T) {
	c := NewCache(NewLRU(12))
	o1 := set(c, 0, 1, 1, 1, nil)
	set(c, 0, 2, 2, 1, nil).Release()
	set(c, 0, 3, 3, 1, nil).Release()
	set(c, 0, 4, 4, 1, nil).Release()
	set(c, 0, 5, 5, 1, nil).Release()
	if h := c.Get(0, 2, nil); h != nil { // 1,3,4,5,2
		h.Release()
	}
	set(c, 0, 9, 9, 10, nil).Release() // 5,2,9

	for _, key := range []uint64{9, 2, 5, 1} {
		h := c.Get(0, key, nil)
		if h == nil {
			t.Errorf("miss for key '%d'", key)
		} else {
			if x := h.Value().(int); x != int(key) {
				t.Errorf("invalid value for key '%d' want '%d', got '%d'", key, key, x)
			}
			h.Release()
		}
	}
	o1.Release()
	for _, key := range []uint64{1, 2, 5} {
		h := c.Get(0, key, nil)
		if h == nil {
			t.Errorf("miss for key '%d'", key)
		} else {
			if x := h.Value().(int); x != int(key) {
				t.Errorf("invalid value for key '%d' want '%d', got '%d'", key, key, x)
			}
			h.Release()
		}
	}
	for _, key := range []uint64{3, 4, 9} {
		h := c.Get(0, key, nil)
		if h != nil {
			t.Errorf("hit for key '%d'", key)
			if x := h.Value().(int); x != int(key) {
				t.Errorf("invalid value for key '%d' want '%d', got '%d'", key, key, x)
			}
			h.Release()
		}
	}
}

func TestLRUCache_Evict(t *testing.T) {
	c := NewCache(NewLRU(6))
	set(c, 0, 1, 1, 1, nil).Release()
	set(c, 0, 2, 2, 1, nil).Release()
	set(c, 1, 1, 4, 1, nil).Release()
	set(c, 1, 2, 5, 1, nil).Release()
	set(c, 2, 1, 6, 1, nil).Release()
	set(c, 2, 2, 7, 1, nil).Release()

	for ns := 0; ns < 3; ns++ {
		for key := 1; key < 3; key++ {
			if h := c.Get(uint64(ns), uint64(key), nil); h != nil {
				h.Release()
			} else {
				t.Errorf("Cache.Get on #%d.%d return nil", ns, key)
			}
		}
	}

	if ok := c.Evict(0, 1); !ok {
		t.Error("first Cache.Evict on #0.1 return false")
	}
	if ok := c.Evict(0, 1); ok {
		t.Error("second Cache.Evict on #0.1 return true")
	}
	if h := c.Get(0, 1, nil); h != nil {
		t.Errorf("Cache.Get on #0.1 return non-nil: %v", h.Value())
	}

	c.EvictNS(1)
	if h := c.Get(1, 1, nil); h != nil {
		t.Errorf("Cache.Get on #1.1 return non-nil: %v", h.Value())
	}
	if h := c.Get(1, 2, nil); h != nil {
		t.Errorf("Cache.Get on #1.2 return non-nil: %v", h.Value())
	}

	c.EvictAll()
	for ns := 0; ns < 3; ns++ {
		for key := 1; key < 3; key++ {
			if h := c.Get(uint64(ns), uint64(key), nil); h != nil {
				t.Errorf("Cache.Get on #%d.%d return non-nil: %v", ns, key, h.Value())
			}
		}
	}
}

func TestLRUCache_Delete(t *testing.T) {
	delFuncCalled := 0
	delFunc := func() {
		delFuncCalled++
	}

	c := NewCache(NewLRU(2))
	set(c, 0, 1, 1, 1, nil).Release()
	set(c, 0, 2, 2, 1, nil).Release()

	if ok := c.Delete(0, 1, delFunc); !ok {
		t.Error("Cache.Delete on #1 return false")
	}
	if h := c.Get(0, 1, nil); h != nil {
		t.Errorf("Cache.Get on #1 return non-nil: %v", h.Value())
	}
	if ok := c.Delete(0, 1, delFunc); ok {
		t.Error("Cache.Delete on #1 return true")
	}

	h2 := c.Get(0, 2, nil)
	if h2 == nil {
		t.Error("Cache.Get on #2 return nil")
	}
	if ok := c.Delete(0, 2, delFunc); !ok {
		t.Error("(1) Cache.Delete on #2 return false")
	}
	if ok := c.Delete(0, 2, delFunc); !ok {
		t.Error("(2) Cache.Delete on #2 return false")
	}

	set(c, 0, 3, 3, 1, nil).Release()
	set(c, 0, 4, 4, 1, nil).Release()
	c.Get(0, 2, nil).Release()

	for key := 2; key <= 4; key++ {
		if h := c.Get(0, uint64(key), nil); h != nil {
			h.Release()
		} else {
			t.Errorf("Cache.Get on #%d return nil", key)
		}
	}

	h2.Release()
	if h := c.Get(0, 2, nil); h != nil {
		t.Errorf("Cache.Get on #2 return non-nil: %v", h.Value())
	}

	if delFuncCalled != 4 {
		t.Errorf("delFunc isn't called 4 times: got=%d", delFuncCalled)
	}
}

func TestLRUCache_Close(t *testing.T) {
	relFuncCalled := 0
	relFunc := func() {
		relFuncCalled++
	}
	delFuncCalled := 0
	delFunc := func() {
		delFuncCalled++
	}

	c := NewCache(NewLRU(2))
	set(c, 0, 1, 1, 1, relFunc).Release()
	set(c, 0, 2, 2, 1, relFunc).Release()

	h3 := set(c, 0, 3, 3, 1, relFunc)
	if h3 == nil {
		t.Error("Cache.Get on #3 return nil")
	}
	if ok := c.Delete(0, 3, delFunc); !ok {
		t.Error("Cache.Delete on #3 return false")
	}

	c.Close()

	if relFuncCalled != 3 {
		t.Errorf("relFunc isn't called 3 times: got=%d", relFuncCalled)
	}
	if delFuncCalled != 1 {
		t.Errorf("delFunc isn't called 1 times: got=%d", delFuncCalled)
	}
}
