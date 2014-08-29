// Copyright (c) 2012, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cache

import (
	"fmt"
	"math/rand"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type releaserFunc struct {
	fn    func()
	value interface{}
}

func (r releaserFunc) Release() {
	if r.fn != nil {
		r.fn()
	}
}

func set(ns Namespace, key uint64, value interface{}, charge int, relf func()) Handle {
	return ns.Get(key, func() (int, interface{}) {
		if relf != nil {
			return charge, releaserFunc{relf, value}
		} else {
			return charge, value
		}
	})
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
			h := ns.Get(y.key, nil)
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
		ns.Delete(x.key, func(exist, pending bool) {
			finalizerOk = true
		})

		if !finalizerOk {
			t.Errorf("case %d delete finalizer not executed", i)
		}

		for j, y := range cases {
			h := ns.Get(y.key, nil)
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
	c := NewLRUCache(12)
	ns := c.GetNamespace(0)
	o1 := set(ns, 1, 1, 1, nil)
	set(ns, 2, 2, 1, nil).Release()
	set(ns, 3, 3, 1, nil).Release()
	set(ns, 4, 4, 1, nil).Release()
	set(ns, 5, 5, 1, nil).Release()
	if h := ns.Get(2, nil); h != nil { // 1,3,4,5,2
		h.Release()
	}
	set(ns, 9, 9, 10, nil).Release() // 5,2,9

	for _, key := range []uint64{9, 2, 5, 1} {
		h := ns.Get(key, nil)
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
		h := ns.Get(key, nil)
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
		h := ns.Get(key, nil)
		if h != nil {
			t.Errorf("hit for key '%d'", key)
			if x := h.Value().(int); x != int(key) {
				t.Errorf("invalid value for key '%d' want '%d', got '%d'", key, key, x)
			}
			h.Release()
		}
	}
}

func TestLRUCache_SetGet(t *testing.T) {
	c := NewLRUCache(13)
	ns := c.GetNamespace(0)
	for i := 0; i < 200; i++ {
		n := uint64(rand.Intn(99999) % 20)
		set(ns, n, n, 1, nil).Release()
		if h := ns.Get(n, nil); h != nil {
			if h.Value() == nil {
				t.Errorf("key '%d' contains nil value", n)
			} else {
				if x := h.Value().(uint64); x != n {
					t.Errorf("invalid value for key '%d' want '%d', got '%d'", n, n, x)
				}
			}
			h.Release()
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
	for _, key := range []uint64{1, 2, 3} {
		h := ns1.Get(key, nil)
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
	o2.Release()
	for _, key := range []uint64{1, 2} {
		h := ns1.Get(key, nil)
		if h != nil {
			t.Errorf("hit for key '%d'", key)
			if x := h.Value().(int); x != int(key) {
				t.Errorf("invalid value for key '%d' want '%d', got '%d'", key, key, x)
			}
			h.Release()
		}
	}
}

type testingCacheObjectCounter struct {
	created  uint32
	released uint32
}

func (c *testingCacheObjectCounter) createOne() {
	atomic.AddUint32(&c.created, 1)
}

func (c *testingCacheObjectCounter) releaseOne() {
	atomic.AddUint32(&c.released, 1)
}

type testingCacheObject struct {
	t   *testing.T
	cnt *testingCacheObjectCounter

	ns, key uint64

	releaseCalled uint32
}

func (x *testingCacheObject) Release() {
	if atomic.CompareAndSwapUint32(&x.releaseCalled, 0, 1) {
		x.cnt.releaseOne()
	} else {
		x.t.Errorf("duplicate setfin NS#%d KEY#%s", x.ns, x.key)
	}
}

func TestLRUCache_Finalizer(t *testing.T) {
	const (
		capacity   = 100
		goroutines = 100
		iterations = 10000
		keymax     = 8000
	)

	runtime.GOMAXPROCS(runtime.NumCPU())
	defer runtime.GOMAXPROCS(1)

	wg := &sync.WaitGroup{}
	cnt := &testingCacheObjectCounter{}

	c := NewLRUCache(capacity)

	type instance struct {
		seed       int64
		rnd        *rand.Rand
		ns         uint64
		effective  int32
		handles    []Handle
		handlesMap map[uint64]int

		delete          bool
		purge           bool
		zap             bool
		wantDel         int32
		delfinCalledAll int32
		delfinCalledEff int32
		purgefinCalled  int32
	}

	instanceGet := func(p *instance, ns Namespace, key uint64) {
		h := ns.Get(key, func() (charge int, value interface{}) {
			to := &testingCacheObject{
				t: t, cnt: cnt,
				ns:  p.ns,
				key: key,
			}
			atomic.AddInt32(&p.effective, 1)
			cnt.createOne()
			return 1, releaserFunc{func() {
				to.Release()
				atomic.AddInt32(&p.effective, -1)
			}, to}
		})
		p.handles = append(p.handles, h)
		p.handlesMap[key] = p.handlesMap[key] + 1
	}
	instanceRelease := func(p *instance, ns Namespace, i int) {
		h := p.handles[i]
		key := h.Value().(releaserFunc).value.(*testingCacheObject).key
		if n := p.handlesMap[key]; n == 0 {
			t.Fatal("key ref == 0")
		} else if n > 1 {
			p.handlesMap[key] = n - 1
		} else {
			delete(p.handlesMap, key)
		}
		h.Release()
		p.handles = append(p.handles[:i], p.handles[i+1:]...)
		p.handles[len(p.handles) : len(p.handles)+1][0] = nil
	}

	seeds := make([]int64, goroutines)
	instances := make([]instance, goroutines)
	for i := range instances {
		p := &instances[i]
		p.handlesMap = make(map[uint64]int)
		if seeds[i] == 0 {
			seeds[i] = time.Now().UnixNano()
		}
		p.seed = seeds[i]
		p.rnd = rand.New(rand.NewSource(p.seed))
		p.ns = uint64(i)
		p.delete = i%6 == 0
		p.purge = i%8 == 0
		p.zap = i%12 == 0 || i%3 == 0
	}

	seedsStr := make([]string, len(seeds))
	for i, seed := range seeds {
		seedsStr[i] = fmt.Sprint(seed)
	}
	t.Logf("seeds := []int64{%s}", strings.Join(seedsStr, ", "))

	// Get and release.
	for i := range instances {
		p := &instances[i]

		wg.Add(1)
		go func(p *instance) {
			defer wg.Done()

			ns := c.GetNamespace(p.ns)
			for i := 0; i < iterations; i++ {
				if len(p.handles) == 0 || p.rnd.Int()%2 == 0 {
					instanceGet(p, ns, uint64(p.rnd.Intn(keymax)))
				} else {
					instanceRelease(p, ns, p.rnd.Intn(len(p.handles)))
				}
			}
		}(p)
	}
	wg.Wait()

	if used, cap := c.Used(), c.Capacity(); used > cap {
		t.Errorf("Used > capacity, used=%d cap=%d", used, cap)
	}

	// Check effective objects.
	for i := range instances {
		p := &instances[i]
		if int(p.effective) < len(p.handlesMap) {
			t.Errorf("#%d effective objects < acquired handle, eo=%d ah=%d", i, p.effective, len(p.handlesMap))
		}
	}

	if want := int(cnt.created - cnt.released); c.Size() != want {
		t.Errorf("Invalid cache size, want=%d got=%d", want, c.Size())
	}

	// Delete and purge.
	for i := range instances {
		p := &instances[i]
		p.wantDel = p.effective

		wg.Add(1)
		go func(p *instance) {
			defer wg.Done()

			ns := c.GetNamespace(p.ns)

			if p.delete {
				for key := uint64(0); key < keymax; key++ {
					_, wantExist := p.handlesMap[key]
					gotExist := ns.Delete(key, func(exist, pending bool) {
						atomic.AddInt32(&p.delfinCalledAll, 1)
						if exist {
							atomic.AddInt32(&p.delfinCalledEff, 1)
						}
					})
					if !gotExist && wantExist {
						t.Errorf("delete on NS#%d KEY#%d not found", p.ns, key)
					}
				}

				var delfinCalled int
				for key := uint64(0); key < keymax; key++ {
					func(key uint64) {
						gotExist := ns.Delete(key, func(exist, pending bool) {
							if exist && !pending {
								t.Errorf("delete fin on NS#%d KEY#%d exist and not pending for deletion", p.ns, key)
							}
							delfinCalled++
						})
						if gotExist {
							t.Errorf("delete on NS#%d KEY#%d found", p.ns, key)
						}
					}(key)
				}
				if delfinCalled != keymax {
					t.Errorf("(2) #%d not all delete fin called, diff=%d", p.ns, keymax-delfinCalled)
				}
			}

			if p.purge {
				ns.Purge(func(ns, key uint64) {
					atomic.AddInt32(&p.purgefinCalled, 1)
				})
			}
		}(p)
	}
	wg.Wait()

	if want := int(cnt.created - cnt.released); c.Size() != want {
		t.Errorf("Invalid cache size, want=%d got=%d", want, c.Size())
	}

	// Release.
	for i := range instances {
		p := &instances[i]

		if !p.zap {
			wg.Add(1)
			go func(p *instance) {
				defer wg.Done()

				ns := c.GetNamespace(p.ns)
				for i := len(p.handles) - 1; i >= 0; i-- {
					instanceRelease(p, ns, i)
				}
			}(p)
		}
	}
	wg.Wait()

	if want := int(cnt.created - cnt.released); c.Size() != want {
		t.Errorf("Invalid cache size, want=%d got=%d", want, c.Size())
	}

	// Zap.
	for i := range instances {
		p := &instances[i]

		if p.zap {
			wg.Add(1)
			go func(p *instance) {
				defer wg.Done()

				ns := c.GetNamespace(p.ns)
				ns.Zap()

				p.handles = nil
				p.handlesMap = nil
			}(p)
		}
	}
	wg.Wait()

	if want := int(cnt.created - cnt.released); c.Size() != want {
		t.Errorf("Invalid cache size, want=%d got=%d", want, c.Size())
	}

	if notrel, used := int(cnt.created-cnt.released), c.Used(); notrel != used {
		t.Errorf("Invalid used value, want=%d got=%d", notrel, used)
	}

	c.Purge(nil)

	for i := range instances {
		p := &instances[i]

		if p.delete {
			if p.delfinCalledAll != keymax {
				t.Errorf("#%d not all delete fin called, purge=%v zap=%v diff=%d", p.ns, p.purge, p.zap, keymax-p.delfinCalledAll)
			}
			if p.delfinCalledEff != p.wantDel {
				t.Errorf("#%d not all effective delete fin called, diff=%d", p.ns, p.wantDel-p.delfinCalledEff)
			}
			if p.purge && p.purgefinCalled > 0 {
				t.Errorf("#%d some purge fin called, delete=%v zap=%v n=%d", p.ns, p.delete, p.zap, p.purgefinCalled)
			}
		} else {
			if p.purge {
				if p.purgefinCalled != p.wantDel {
					t.Errorf("#%d not all purge fin called, delete=%v zap=%v diff=%d", p.ns, p.delete, p.zap, p.wantDel-p.purgefinCalled)
				}
			}
		}
	}

	if cnt.created != cnt.released {
		t.Errorf("Some cache object weren't released, created=%d released=%d", cnt.created, cnt.released)
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
