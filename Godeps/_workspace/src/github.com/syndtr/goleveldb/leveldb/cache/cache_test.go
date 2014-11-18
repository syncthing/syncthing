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
	created  uint
	released uint
}

func (c *testingCacheObjectCounter) createOne() {
	c.created++
}

func (c *testingCacheObjectCounter) releaseOne() {
	c.released++
}

type testingCacheObject struct {
	t   *testing.T
	cnt *testingCacheObjectCounter

	ns, key uint64

	releaseCalled bool
}

func (x *testingCacheObject) Release() {
	if !x.releaseCalled {
		x.releaseCalled = true
		x.cnt.releaseOne()
	} else {
		x.t.Errorf("duplicate setfin NS#%d KEY#%d", x.ns, x.key)
	}
}

func TestLRUCache_ConcurrentSetGet(t *testing.T) {
	runtime.GOMAXPROCS(runtime.NumCPU())

	seed := time.Now().UnixNano()
	t.Logf("seed=%d", seed)

	const (
		N = 2000000
		M = 4000
		C = 3
	)

	var set, get uint32

	wg := &sync.WaitGroup{}
	c := NewLRUCache(M / 4)
	for ni := uint64(0); ni < C; ni++ {
		r0 := rand.New(rand.NewSource(seed + int64(ni)))
		r1 := rand.New(rand.NewSource(seed + int64(ni) + 1))
		ns := c.GetNamespace(ni)

		wg.Add(2)
		go func(ns Namespace, r *rand.Rand) {
			for i := 0; i < N; i++ {
				x := uint64(r.Int63n(M))
				o := ns.Get(x, func() (int, interface{}) {
					atomic.AddUint32(&set, 1)
					return 1, x
				})
				if v := o.Value().(uint64); v != x {
					t.Errorf("#%d invalid value, got=%d", x, v)
				}
				o.Release()
			}
			wg.Done()
		}(ns, r0)
		go func(ns Namespace, r *rand.Rand) {
			for i := 0; i < N; i++ {
				x := uint64(r.Int63n(M))
				o := ns.Get(x, nil)
				if o != nil {
					atomic.AddUint32(&get, 1)
					if v := o.Value().(uint64); v != x {
						t.Errorf("#%d invalid value, got=%d", x, v)
					}
					o.Release()
				}
			}
			wg.Done()
		}(ns, r1)
	}

	wg.Wait()

	t.Logf("set=%d get=%d", set, get)
}

func TestLRUCache_Finalizer(t *testing.T) {
	const (
		capacity   = 100
		goroutines = 100
		iterations = 10000
		keymax     = 8000
	)

	cnt := &testingCacheObjectCounter{}

	c := NewLRUCache(capacity)

	type instance struct {
		seed       int64
		rnd        *rand.Rand
		nsid       uint64
		ns         Namespace
		effective  int
		handles    []Handle
		handlesMap map[uint64]int

		delete          bool
		purge           bool
		zap             bool
		wantDel         int
		delfinCalled    int
		delfinCalledAll int
		delfinCalledEff int
		purgefinCalled  int
	}

	instanceGet := func(p *instance, key uint64) {
		h := p.ns.Get(key, func() (charge int, value interface{}) {
			to := &testingCacheObject{
				t: t, cnt: cnt,
				ns:  p.nsid,
				key: key,
			}
			p.effective++
			cnt.createOne()
			return 1, releaserFunc{func() {
				to.Release()
				p.effective--
			}, to}
		})
		p.handles = append(p.handles, h)
		p.handlesMap[key] = p.handlesMap[key] + 1
	}
	instanceRelease := func(p *instance, i int) {
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

	seed := time.Now().UnixNano()
	t.Logf("seed=%d", seed)

	instances := make([]*instance, goroutines)
	for i := range instances {
		p := &instance{}
		p.handlesMap = make(map[uint64]int)
		p.seed = seed + int64(i)
		p.rnd = rand.New(rand.NewSource(p.seed))
		p.nsid = uint64(i)
		p.ns = c.GetNamespace(p.nsid)
		p.delete = i%6 == 0
		p.purge = i%8 == 0
		p.zap = i%12 == 0 || i%3 == 0
		instances[i] = p
	}

	runr := rand.New(rand.NewSource(seed - 1))
	run := func(rnd *rand.Rand, x []*instance, init func(p *instance) bool, fn func(p *instance, i int) bool) {
		var (
			rx []*instance
			rn []int
		)
		if init == nil {
			rx = append([]*instance{}, x...)
			rn = make([]int, len(x))
		} else {
			for _, p := range x {
				if init(p) {
					rx = append(rx, p)
					rn = append(rn, 0)
				}
			}
		}
		for len(rx) > 0 {
			i := rand.Intn(len(rx))
			if fn(rx[i], rn[i]) {
				rn[i]++
			} else {
				rx = append(rx[:i], rx[i+1:]...)
				rn = append(rn[:i], rn[i+1:]...)
			}
		}
	}

	// Get and release.
	run(runr, instances, nil, func(p *instance, i int) bool {
		if i < iterations {
			if len(p.handles) == 0 || p.rnd.Int()%2 == 0 {
				instanceGet(p, uint64(p.rnd.Intn(keymax)))
			} else {
				instanceRelease(p, p.rnd.Intn(len(p.handles)))
			}
			return true
		} else {
			return false
		}
	})

	if used, cap := c.Used(), c.Capacity(); used > cap {
		t.Errorf("Used > capacity, used=%d cap=%d", used, cap)
	}

	// Check effective objects.
	for i, p := range instances {
		if int(p.effective) < len(p.handlesMap) {
			t.Errorf("#%d effective objects < acquired handle, eo=%d ah=%d", i, p.effective, len(p.handlesMap))
		}
	}

	if want := int(cnt.created - cnt.released); c.Size() != want {
		t.Errorf("Invalid cache size, want=%d got=%d", want, c.Size())
	}

	// First delete.
	run(runr, instances, func(p *instance) bool {
		p.wantDel = p.effective
		return p.delete
	}, func(p *instance, i int) bool {
		key := uint64(i)
		if key < keymax {
			_, wantExist := p.handlesMap[key]
			gotExist := p.ns.Delete(key, func(exist, pending bool) {
				p.delfinCalledAll++
				if exist {
					p.delfinCalledEff++
				}
			})
			if !gotExist && wantExist {
				t.Errorf("delete on NS#%d KEY#%d not found", p.nsid, key)
			}
			return true
		} else {
			return false
		}
	})

	// Second delete.
	run(runr, instances, func(p *instance) bool {
		p.delfinCalled = 0
		return p.delete
	}, func(p *instance, i int) bool {
		key := uint64(i)
		if key < keymax {
			gotExist := p.ns.Delete(key, func(exist, pending bool) {
				if exist && !pending {
					t.Errorf("delete fin on NS#%d KEY#%d exist and not pending for deletion", p.nsid, key)
				}
				p.delfinCalled++
			})
			if gotExist {
				t.Errorf("delete on NS#%d KEY#%d found", p.nsid, key)
			}
			return true
		} else {
			if p.delfinCalled != keymax {
				t.Errorf("(2) NS#%d not all delete fin called, diff=%d", p.nsid, keymax-p.delfinCalled)
			}
			return false
		}
	})

	// Purge.
	run(runr, instances, func(p *instance) bool {
		return p.purge
	}, func(p *instance, i int) bool {
		p.ns.Purge(func(ns, key uint64) {
			p.purgefinCalled++
		})
		return false
	})

	if want := int(cnt.created - cnt.released); c.Size() != want {
		t.Errorf("Invalid cache size, want=%d got=%d", want, c.Size())
	}

	// Release.
	run(runr, instances, func(p *instance) bool {
		return !p.zap
	}, func(p *instance, i int) bool {
		if len(p.handles) > 0 {
			instanceRelease(p, len(p.handles)-1)
			return true
		} else {
			return false
		}
	})

	if want := int(cnt.created - cnt.released); c.Size() != want {
		t.Errorf("Invalid cache size, want=%d got=%d", want, c.Size())
	}

	// Zap.
	run(runr, instances, func(p *instance) bool {
		return p.zap
	}, func(p *instance, i int) bool {
		p.ns.Zap()
		p.handles = nil
		p.handlesMap = nil
		return false
	})

	if want := int(cnt.created - cnt.released); c.Size() != want {
		t.Errorf("Invalid cache size, want=%d got=%d", want, c.Size())
	}

	if notrel, used := int(cnt.created-cnt.released), c.Used(); notrel != used {
		t.Errorf("Invalid used value, want=%d got=%d", notrel, used)
	}

	c.Purge(nil)

	for _, p := range instances {
		if p.delete {
			if p.delfinCalledAll != keymax {
				t.Errorf("#%d not all delete fin called, purge=%v zap=%v diff=%d", p.nsid, p.purge, p.zap, keymax-p.delfinCalledAll)
			}
			if p.delfinCalledEff != p.wantDel {
				t.Errorf("#%d not all effective delete fin called, diff=%d", p.nsid, p.wantDel-p.delfinCalledEff)
			}
			if p.purge && p.purgefinCalled > 0 {
				t.Errorf("#%d some purge fin called, delete=%v zap=%v n=%d", p.nsid, p.delete, p.zap, p.purgefinCalled)
			}
		} else {
			if p.purge {
				if p.purgefinCalled != p.wantDel {
					t.Errorf("#%d not all purge fin called, delete=%v zap=%v diff=%d", p.nsid, p.delete, p.zap, p.wantDel-p.purgefinCalled)
				}
			}
		}
	}

	if cnt.created != cnt.released {
		t.Errorf("Some cache object weren't released, created=%d released=%d", cnt.created, cnt.released)
	}
}

func BenchmarkLRUCache_Set(b *testing.B) {
	c := NewLRUCache(0)
	ns := c.GetNamespace(0)
	b.ResetTimer()
	for i := uint64(0); i < uint64(b.N); i++ {
		set(ns, i, "", 1, nil)
	}
}

func BenchmarkLRUCache_Get(b *testing.B) {
	c := NewLRUCache(0)
	ns := c.GetNamespace(0)
	b.ResetTimer()
	for i := uint64(0); i < uint64(b.N); i++ {
		set(ns, i, "", 1, nil)
	}
	b.ResetTimer()
	for i := uint64(0); i < uint64(b.N); i++ {
		ns.Get(i, nil)
	}
}

func BenchmarkLRUCache_Get2(b *testing.B) {
	c := NewLRUCache(0)
	ns := c.GetNamespace(0)
	b.ResetTimer()
	for i := uint64(0); i < uint64(b.N); i++ {
		set(ns, i, "", 1, nil)
	}
	b.ResetTimer()
	for i := uint64(0); i < uint64(b.N); i++ {
		ns.Get(i, func() (charge int, value interface{}) {
			return 0, nil
		})
	}
}

func BenchmarkLRUCache_Release(b *testing.B) {
	c := NewLRUCache(0)
	ns := c.GetNamespace(0)
	handles := make([]Handle, b.N)
	for i := uint64(0); i < uint64(b.N); i++ {
		handles[i] = set(ns, i, "", 1, nil)
	}
	b.ResetTimer()
	for _, h := range handles {
		h.Release()
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
		set(ns, i, "", 1, nil).Release()
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
		set(ns, i, "", 1, nil).Release()
	}

	for i := uint64(0); i < uint64(nb); i++ {
		set(ns, i, "", 1, nil).Release()
	}
}
