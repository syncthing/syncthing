// Copyright 2014 The b Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package b

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"path"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/cznic/mathutil"
	"github.com/cznic/strutil"
)

var caller = func(s string, va ...interface{}) {
	_, fn, fl, _ := runtime.Caller(2)
	fmt.Printf("%s:%d: ", path.Base(fn), fl)
	fmt.Printf(s, va...)
	fmt.Println()
}

func dbg(s string, va ...interface{}) {
	if s == "" {
		s = strings.Repeat("%v ", len(va))
	}
	_, fn, fl, _ := runtime.Caller(1)
	fmt.Printf("%s:%d: ", path.Base(fn), fl)
	fmt.Printf(s, va...)
	fmt.Println()
}

func TODO(...interface{}) string {
	_, fn, fl, _ := runtime.Caller(1)
	return fmt.Sprintf("TODO: %s:%d:\n", path.Base(fn), fl)
}

func use(...interface{}) {}

// ============================================================================

func isNil(p interface{}) bool {
	switch x := p.(type) {
	case *x:
		if x == nil {
			return true
		}
	case *d:
		if x == nil {
			return true
		}
	}
	return false
}

func (t *Tree) dump() string {
	var buf bytes.Buffer
	f := strutil.IndentFormatter(&buf, "\t")

	num := map[interface{}]int{}
	visited := map[interface{}]bool{}

	handle := func(p interface{}) int {
		if isNil(p) {
			return 0
		}

		if n, ok := num[p]; ok {
			return n
		}

		n := len(num) + 1
		num[p] = n
		return n
	}

	var pagedump func(interface{}, string)
	pagedump = func(p interface{}, pref string) {
		if isNil(p) || visited[p] {
			return
		}

		visited[p] = true
		switch x := p.(type) {
		case *x:
			h := handle(p)
			n := 0
			for i, v := range x.x {
				if v.ch != nil || v.k != nil {
					n = i + 1
				}
			}
			f.Format("%sX#%d(%p) n %d:%d {", pref, h, x, x.c, n)
			a := []interface{}{}
			for i, v := range x.x[:n] {
				a = append(a, v.ch)
				if i != 0 {
					f.Format(" ")
				}
				f.Format("(C#%d K %v)", handle(v.ch), v.k)
			}
			f.Format("}\n")
			for _, p := range a {
				pagedump(p, pref+". ")
			}
		case *d:
			h := handle(p)
			n := 0
			for i, v := range x.d {
				if v.k != nil || v.v != nil {
					n = i + 1
				}
			}
			f.Format("%sD#%d(%p) P#%d N#%d n %d:%d {", pref, h, x, handle(x.p), handle(x.n), x.c, n)
			for i, v := range x.d[:n] {
				if i != 0 {
					f.Format(" ")
				}
				f.Format("%v:%v", v.k, v.v)
			}
			f.Format("}\n")
		}
	}

	pagedump(t.r, "")
	s := buf.String()
	if s != "" {
		s = s[:len(s)-1]
	}
	return s
}

func rng() *mathutil.FC32 {
	x, err := mathutil.NewFC32(math.MinInt32/4, math.MaxInt32/4, false)
	if err != nil {
		panic(err)
	}

	return x
}

func cmp(a, b interface{}) int {
	return a.(int) - b.(int)
}

func TestGet0(t *testing.T) {
	r := TreeNew(cmp)
	if g, e := r.Len(), 0; g != e {
		t.Fatal(g, e)
	}

	_, ok := r.Get(42)
	if ok {
		t.Fatal(ok)
	}

}

func TestSetGet0(t *testing.T) {
	r := TreeNew(cmp)
	set := r.Set
	set(42, 314)
	if g, e := r.Len(), 1; g != e {
		t.Fatal(g, e)
	}

	v, ok := r.Get(42)
	if !ok {
		t.Fatal(ok)
	}

	if g, e := v.(int), 314; g != e {
		t.Fatal(g, e)
	}

	set(42, 278)
	if g, e := r.Len(), 1; g != e {
		t.Fatal(g, e)
	}

	v, ok = r.Get(42)
	if !ok {
		t.Fatal(ok)
	}

	if g, e := v.(int), 278; g != e {
		t.Fatal(g, e)
	}

	set(420, 0.5)
	if g, e := r.Len(), 2; g != e {
		t.Fatal(g, e)
	}

	v, ok = r.Get(42)
	if !ok {
		t.Fatal(ok)
	}

	if g, e := v.(int), 278; g != e {
		t.Fatal(g, e)
	}

	v, ok = r.Get(420)
	if !ok {
		t.Fatal(ok)
	}

	if g, e := v.(float64), 0.5; g != e {
		t.Fatal(g, e)
	}
}

func TestSetGet1(t *testing.T) {
	const N = 40000
	for _, x := range []int{0, -1, 0x555555, 0xaaaaaa, 0x333333, 0xcccccc, 0x314159} {
		r := TreeNew(cmp)
		set := r.Set
		a := make([]int, N)
		for i := range a {
			a[i] = (i ^ x) << 1
		}
		for i, k := range a {
			set(k, k^x)
			if g, e := r.Len(), i+1; g != e {
				t.Fatal(i, g, e)
			}
		}

		for i, k := range a {
			v, ok := r.Get(k)
			if !ok {
				t.Fatal(i, k, v, ok)
			}

			if g, e := v.(int), k^x; g != e {
				t.Fatal(i, g, e)
			}

			k |= 1
			_, ok = r.Get(k)
			if ok {
				t.Fatal(i, k)
			}

		}

		for _, k := range a {
			r.Set(k, (k^x)+42)
		}

		for i, k := range a {
			v, ok := r.Get(k)
			if !ok {
				t.Fatal(i, k, v, ok)
			}

			if g, e := v.(int), k^x+42; g != e {
				t.Fatal(i, g, e)
			}

			k |= 1
			_, ok = r.Get(k)
			if ok {
				t.Fatal(i, k)
			}
		}
	}
}

func TestPrealloc(*testing.T) {
	const n = 2e6
	rng := rng()
	a := make([]int, n)
	for i := range a {
		a[i] = rng.Next()
	}
	r := TreeNew(cmp)
	for _, v := range a {
		r.Set(v, 0)
	}
	r.Close()
}

func BenchmarkSetSeq1e3(b *testing.B) {
	benchmarkSetSeq(b, 1e3)
}

func BenchmarkSetSeq1e4(b *testing.B) {
	benchmarkSetSeq(b, 1e4)
}

func BenchmarkSetSeq1e5(b *testing.B) {
	benchmarkSetSeq(b, 1e5)
}

func BenchmarkSetSeq1e6(b *testing.B) {
	benchmarkSetSeq(b, 1e6)
}

func benchmarkSetSeq(b *testing.B, n int) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		r := TreeNew(cmp)
		debug.FreeOSMemory()
		b.StartTimer()
		for j := 0; j < n; j++ {
			r.Set(j, j)
		}
		b.StopTimer()
		r.Close()
	}
	b.StopTimer()
}

func BenchmarkGetSeq1e3(b *testing.B) {
	benchmarkGetSeq(b, 1e3)
}

func BenchmarkGetSeq1e4(b *testing.B) {
	benchmarkGetSeq(b, 1e4)
}

func BenchmarkGetSeq1e5(b *testing.B) {
	benchmarkGetSeq(b, 1e5)
}

func BenchmarkGetSeq1e6(b *testing.B) {
	benchmarkGetSeq(b, 1e6)
}

func benchmarkGetSeq(b *testing.B, n int) {
	r := TreeNew(cmp)
	for i := 0; i < n; i++ {
		r.Set(i, i)
	}
	debug.FreeOSMemory()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := 0; j < n; j++ {
			r.Get(j)
		}
	}
	b.StopTimer()
	r.Close()
}

func BenchmarkSetRnd1e3(b *testing.B) {
	benchmarkSetRnd(b, 1e3)
}

func BenchmarkSetRnd1e4(b *testing.B) {
	benchmarkSetRnd(b, 1e4)
}

func BenchmarkSetRnd1e5(b *testing.B) {
	benchmarkSetRnd(b, 1e5)
}

func BenchmarkSetRnd1e6(b *testing.B) {
	benchmarkSetRnd(b, 1e6)
}

func benchmarkSetRnd(b *testing.B, n int) {
	rng := rng()
	a := make([]int, n)
	for i := range a {
		a[i] = rng.Next()
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		r := TreeNew(cmp)
		debug.FreeOSMemory()
		b.StartTimer()
		for _, v := range a {
			r.Set(v, 0)
		}
		b.StopTimer()
		r.Close()
	}
	b.StopTimer()
}

func BenchmarkGetRnd1e3(b *testing.B) {
	benchmarkGetRnd(b, 1e3)
}

func BenchmarkGetRnd1e4(b *testing.B) {
	benchmarkGetRnd(b, 1e4)
}

func BenchmarkGetRnd1e5(b *testing.B) {
	benchmarkGetRnd(b, 1e5)
}

func BenchmarkGetRnd1e6(b *testing.B) {
	benchmarkGetRnd(b, 1e6)
}

func benchmarkGetRnd(b *testing.B, n int) {
	r := TreeNew(cmp)
	rng := rng()
	a := make([]int, n)
	for i := range a {
		a[i] = rng.Next()
	}
	for _, v := range a {
		r.Set(v, 0)
	}
	debug.FreeOSMemory()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, v := range a {
			r.Get(v)
		}
	}
	b.StopTimer()
	r.Close()
}

func TestSetGet2(t *testing.T) {
	const N = 40000
	for _, x := range []int{0, -1, 0x555555, 0xaaaaaa, 0x333333, 0xcccccc, 0x314159} {
		rng := rng()
		r := TreeNew(cmp)
		set := r.Set
		a := make([]int, N)
		for i := range a {
			a[i] = (rng.Next() ^ x) << 1
		}
		for i, k := range a {
			set(k, k^x)
			if g, e := r.Len(), i+1; g != e {
				t.Fatal(i, x, g, e)
			}
		}

		for i, k := range a {
			v, ok := r.Get(k)
			if !ok {
				t.Fatal(i, k, v, ok)
			}

			if g, e := v.(int), k^x; g != e {
				t.Fatal(i, g, e)
			}

			k |= 1
			_, ok = r.Get(k)
			if ok {
				t.Fatal(i, k)
			}
		}

		for _, k := range a {
			r.Set(k, (k^x)+42)
		}

		for i, k := range a {
			v, ok := r.Get(k)
			if !ok {
				t.Fatal(i, k, v, ok)
			}

			if g, e := v.(int), k^x+42; g != e {
				t.Fatal(i, g, e)
			}

			k |= 1
			_, ok = r.Get(k)
			if ok {
				t.Fatal(i, k)
			}
		}
	}
}

func TestSetGet3(t *testing.T) {
	r := TreeNew(cmp)
	set := r.Set
	var i int
	for i = 0; ; i++ {
		set(i, -i)
		if _, ok := r.r.(*x); ok {
			break
		}
	}
	for j := 0; j <= i; j++ {
		set(j, j)
	}

	for j := 0; j <= i; j++ {
		v, ok := r.Get(j)
		if !ok {
			t.Fatal(j)
		}

		if g, e := v.(int), j; g != e {
			t.Fatal(g, e)
		}
	}
}

func TestDelete0(t *testing.T) {
	r := TreeNew(cmp)
	if ok := r.Delete(0); ok {
		t.Fatal(ok)
	}

	if g, e := r.Len(), 0; g != e {
		t.Fatal(g, e)
	}

	r.Set(0, 0)
	if ok := r.Delete(1); ok {
		t.Fatal(ok)
	}

	if g, e := r.Len(), 1; g != e {
		t.Fatal(g, e)
	}

	if ok := r.Delete(0); !ok {
		t.Fatal(ok)
	}

	if g, e := r.Len(), 0; g != e {
		t.Fatal(g, e)
	}

	if ok := r.Delete(0); ok {
		t.Fatal(ok)
	}

	r.Set(0, 0)
	r.Set(1, 1)
	if ok := r.Delete(1); !ok {
		t.Fatal(ok)
	}

	if g, e := r.Len(), 1; g != e {
		t.Fatal(g, e)
	}

	if ok := r.Delete(1); ok {
		t.Fatal(ok)
	}

	if ok := r.Delete(0); !ok {
		t.Fatal(ok)
	}

	if g, e := r.Len(), 0; g != e {
		t.Fatal(g, e)
	}

	if ok := r.Delete(0); ok {
		t.Fatal(ok)
	}

	r.Set(0, 0)
	r.Set(1, 1)
	if ok := r.Delete(0); !ok {
		t.Fatal(ok)
	}

	if g, e := r.Len(), 1; g != e {
		t.Fatal(g, e)
	}

	if ok := r.Delete(0); ok {
		t.Fatal(ok)
	}

	if ok := r.Delete(1); !ok {
		t.Fatal(ok)
	}

	if g, e := r.Len(), 0; g != e {
		t.Fatal(g, e)
	}

	if ok := r.Delete(1); ok {
		t.Fatal(ok)
	}
}

func TestDelete1(t *testing.T) {
	const N = 130000
	for _, x := range []int{0, -1, 0x555555, 0xaaaaaa, 0x333333, 0xcccccc, 0x314159} {
		r := TreeNew(cmp)
		set := r.Set
		a := make([]int, N)
		for i := range a {
			a[i] = (i ^ x) << 1
		}
		for _, k := range a {
			set(k, 0)
		}

		for i, k := range a {
			ok := r.Delete(k)
			if !ok {
				t.Fatal(i, x, k)
			}

			if g, e := r.Len(), N-i-1; g != e {
				t.Fatal(i, g, e)
			}
		}
	}
}

func BenchmarkDelSeq1e3(b *testing.B) {
	benchmarkDelSeq(b, 1e3)
}

func BenchmarkDelSeq1e4(b *testing.B) {
	benchmarkDelSeq(b, 1e4)
}

func BenchmarkDelSeq1e5(b *testing.B) {
	benchmarkDelSeq(b, 1e5)
}

func BenchmarkDelSeq1e6(b *testing.B) {
	benchmarkDelSeq(b, 1e6)
}

func benchmarkDelSeq(b *testing.B, n int) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		r := TreeNew(cmp)
		for i := 0; i < n; i++ {
			r.Set(i, i)
		}
		debug.FreeOSMemory()
		b.StartTimer()
		for j := 0; j < n; j++ {
			r.Delete(j)
		}
	}
	b.StopTimer()
}

func BenchmarkDelRnd1e3(b *testing.B) {
	benchmarkDelRnd(b, 1e3)
}

func BenchmarkDelRnd1e4(b *testing.B) {
	benchmarkDelRnd(b, 1e4)
}

func BenchmarkDelRnd1e5(b *testing.B) {
	benchmarkDelRnd(b, 1e5)
}

func BenchmarkDelRnd1e6(b *testing.B) {
	benchmarkDelRnd(b, 1e6)
}

func benchmarkDelRnd(b *testing.B, n int) {
	rng := rng()
	a := make([]int, n)
	for i := range a {
		a[i] = rng.Next()
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		r := TreeNew(cmp)
		for _, v := range a {
			r.Set(v, 0)
		}
		debug.FreeOSMemory()
		b.StartTimer()
		for _, v := range a {
			r.Delete(v)
		}
		b.StopTimer()
		r.Close()
	}
	b.StopTimer()
}

func TestDelete2(t *testing.T) {
	const N = 100000
	for _, x := range []int{0, -1, 0x555555, 0xaaaaaa, 0x333333, 0xcccccc, 0x314159} {
		r := TreeNew(cmp)
		set := r.Set
		a := make([]int, N)
		rng := rng()
		for i := range a {
			a[i] = (rng.Next() ^ x) << 1
		}
		for _, k := range a {
			set(k, 0)
		}

		for i, k := range a {
			ok := r.Delete(k)
			if !ok {
				t.Fatal(i, x, k)
			}

			if g, e := r.Len(), N-i-1; g != e {
				t.Fatal(i, g, e)
			}
		}
	}
}

func TestEnumeratorNext(t *testing.T) {
	// seeking within 3 keys: 10, 20, 30
	table := []struct {
		k    int
		hit  bool
		keys []int
	}{
		{5, false, []int{10, 20, 30}},
		{10, true, []int{10, 20, 30}},
		{15, false, []int{20, 30}},
		{20, true, []int{20, 30}},
		{25, false, []int{30}},
		{30, true, []int{30}},
		{35, false, []int{}},
	}

	for i, test := range table {
		up := test.keys
		r := TreeNew(cmp)

		r.Set(10, 100)
		r.Set(20, 200)
		r.Set(30, 300)

		for verChange := 0; verChange < 16; verChange++ {
			en, hit := r.Seek(test.k)

			if g, e := hit, test.hit; g != e {
				t.Fatal(i, g, e)
			}

			j := 0
			for {
				if verChange&(1<<uint(j)) != 0 {
					r.Set(20, 200)
				}

				k, v, err := en.Next()
				if err != nil {
					if err != io.EOF {
						t.Fatal(i, err)
					}

					break
				}

				if j >= len(up) {
					t.Fatal(i, j, verChange)
				}

				if g, e := k.(int), up[j]; g != e {
					t.Fatal(i, j, verChange, g, e)
				}

				if g, e := v.(int), 10*up[j]; g != e {
					t.Fatal(i, g, e)
				}

				j++

			}

			if g, e := j, len(up); g != e {
				t.Fatal(i, j, g, e)
			}
		}

	}
}

func TestEnumeratorPrev(t *testing.T) {
	// seeking within 3 keys: 10, 20, 30
	table := []struct {
		k    int
		hit  bool
		keys []int
	}{
		{5, false, []int{10}},
		{10, true, []int{10}},
		{15, false, []int{20, 10}},
		{20, true, []int{20, 10}},
		{25, false, []int{30, 20, 10}},
		{30, true, []int{30, 20, 10}},
		{35, false, []int{}},
	}

	for i, test := range table {
		dn := test.keys
		r := TreeNew(cmp)

		r.Set(10, 100)
		r.Set(20, 200)
		r.Set(30, 300)

		for verChange := 0; verChange < 16; verChange++ {
			en, hit := r.Seek(test.k)

			if g, e := hit, test.hit; g != e {
				t.Fatal(i, g, e)
			}

			j := 0
			for {
				if verChange&(1<<uint(j)) != 0 {
					r.Set(20, 200)
				}

				k, v, err := en.Prev()
				if err != nil {
					if err != io.EOF {
						t.Fatal(i, err)
					}

					break
				}

				if j >= len(dn) {
					t.Fatal(i, j, verChange)
				}

				if g, e := k.(int), dn[j]; g != e {
					t.Fatal(i, j, verChange, g, e)
				}

				if g, e := v.(int), 10*dn[j]; g != e {
					t.Fatal(i, g, e)
				}

				j++

			}

			if g, e := j, len(dn); g != e {
				t.Fatal(i, j, g, e)
			}
		}

	}
}

func BenchmarkSeekSeq1e3(b *testing.B) {
	benchmarkSeekSeq(b, 1e3)
}

func BenchmarkSeekSeq1e4(b *testing.B) {
	benchmarkSeekSeq(b, 1e4)
}

func BenchmarkSeekSeq1e5(b *testing.B) {
	benchmarkSeekSeq(b, 1e5)
}

func BenchmarkSeekSeq1e6(b *testing.B) {
	benchmarkSeekSeq(b, 1e6)
}

func benchmarkSeekSeq(b *testing.B, n int) {
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		t := TreeNew(cmp)
		for j := 0; j < n; j++ {
			t.Set(j, 0)
		}
		debug.FreeOSMemory()
		b.StartTimer()
		for j := 0; j < n; j++ {
			e, _ := t.Seek(j)
			e.Close()
		}
		b.StopTimer()
		t.Close()
	}
	b.StopTimer()
}

func BenchmarkSeekRnd1e3(b *testing.B) {
	benchmarkSeekRnd(b, 1e3)
}

func BenchmarkSeekRnd1e4(b *testing.B) {
	benchmarkSeekRnd(b, 1e4)
}

func BenchmarkSeekRnd1e5(b *testing.B) {
	benchmarkSeekRnd(b, 1e5)
}

func BenchmarkSeekRnd1e6(b *testing.B) {
	benchmarkSeekRnd(b, 1e6)
}

func benchmarkSeekRnd(b *testing.B, n int) {
	r := TreeNew(cmp)
	rng := rng()
	a := make([]int, n)
	for i := range a {
		a[i] = rng.Next()
	}
	for _, v := range a {
		r.Set(v, 0)
	}
	debug.FreeOSMemory()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, v := range a {
			e, _ := r.Seek(v)
			e.Close()
		}
	}
	b.StopTimer()
	r.Close()
}

func BenchmarkNext1e3(b *testing.B) {
	benchmarkNext(b, 1e3)
}

func BenchmarkNext1e4(b *testing.B) {
	benchmarkNext(b, 1e4)
}

func BenchmarkNext1e5(b *testing.B) {
	benchmarkNext(b, 1e5)
}

func BenchmarkNext1e6(b *testing.B) {
	benchmarkNext(b, 1e6)
}

func benchmarkNext(b *testing.B, n int) {
	t := TreeNew(cmp)
	for i := 0; i < n; i++ {
		t.Set(i, 0)
	}
	debug.FreeOSMemory()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		en, err := t.SeekFirst()
		if err != nil {
			b.Fatal(err)
		}

		m := 0
		for {
			if _, _, err = en.Next(); err != nil {
				break
			}
			m++
		}
		if m != n {
			b.Fatal(m)
		}
	}
	b.StopTimer()
	t.Close()
}

func BenchmarkPrev1e3(b *testing.B) {
	benchmarkPrev(b, 1e3)
}

func BenchmarkPrev1e4(b *testing.B) {
	benchmarkPrev(b, 1e4)
}

func BenchmarkPrev1e5(b *testing.B) {
	benchmarkPrev(b, 1e5)
}

func BenchmarkPrev1e6(b *testing.B) {
	benchmarkPrev(b, 1e6)
}

func benchmarkPrev(b *testing.B, n int) {
	t := TreeNew(cmp)
	for i := 0; i < n; i++ {
		t.Set(i, 0)
	}
	debug.FreeOSMemory()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		en, err := t.SeekLast()
		if err != nil {
			b.Fatal(err)
		}

		m := 0
		for {
			if _, _, err = en.Prev(); err != nil {
				break
			}
			m++
		}
		if m != n {
			b.Fatal(m)
		}
	}
}

func TestSeekFirst0(t *testing.T) {
	b := TreeNew(cmp)
	_, err := b.SeekFirst()
	if g, e := err, io.EOF; g != e {
		t.Fatal(g, e)
	}
}

func TestSeekFirst1(t *testing.T) {
	b := TreeNew(cmp)
	b.Set(1, 10)
	en, err := b.SeekFirst()
	if err != nil {
		t.Fatal(err)
	}

	k, v, err := en.Next()
	if k != 1 || v != 10 || err != nil {
		t.Fatal(k, v, err)
	}

	k, v, err = en.Next()
	if err == nil {
		t.Fatal(k, v, err)
	}
}

func TestSeekFirst2(t *testing.T) {
	b := TreeNew(cmp)
	b.Set(1, 10)
	b.Set(2, 20)
	en, err := b.SeekFirst()
	if err != nil {
		t.Fatal(err)
	}

	k, v, err := en.Next()
	if k != 1 || v != 10 || err != nil {
		t.Fatal(k, v, err)
	}

	k, v, err = en.Next()
	if k != 2 || v != 20 || err != nil {
		t.Fatal(k, v, err)
	}

	k, v, err = en.Next()
	if err == nil {
		t.Fatal(k, v, err)
	}
}

func TestSeekFirst3(t *testing.T) {
	b := TreeNew(cmp)
	b.Set(2, 20)
	b.Set(3, 30)
	b.Set(1, 10)
	en, err := b.SeekFirst()
	if err != nil {
		t.Fatal(err)
	}

	k, v, err := en.Next()
	if k != 1 || v != 10 || err != nil {
		t.Fatal(k, v, err)
	}

	k, v, err = en.Next()
	if k != 2 || v != 20 || err != nil {
		t.Fatal(k, v, err)
	}

	k, v, err = en.Next()
	if k != 3 || v != 30 || err != nil {
		t.Fatal(k, v, err)
	}

	k, v, err = en.Next()
	if err == nil {
		t.Fatal(k, v, err)
	}
}

func TestSeekLast0(t *testing.T) {
	b := TreeNew(cmp)
	_, err := b.SeekLast()
	if g, e := err, io.EOF; g != e {
		t.Fatal(g, e)
	}
}

func TestSeekLast1(t *testing.T) {
	b := TreeNew(cmp)
	b.Set(1, 10)
	en, err := b.SeekLast()
	if err != nil {
		t.Fatal(err)
	}

	k, v, err := en.Prev()
	if k != 1 || v != 10 || err != nil {
		t.Fatal(k, v, err)
	}

	k, v, err = en.Prev()
	if err == nil {
		t.Fatal(k, v, err)
	}
}

func TestSeekLast2(t *testing.T) {
	b := TreeNew(cmp)
	b.Set(1, 10)
	b.Set(2, 20)
	en, err := b.SeekLast()
	if err != nil {
		t.Fatal(err)
	}

	k, v, err := en.Prev()
	if k != 2 || v != 20 || err != nil {
		t.Fatal(k, v, err)
	}

	k, v, err = en.Prev()
	if k != 1 || v != 10 || err != nil {
		t.Fatal(k, v, err)
	}

	k, v, err = en.Prev()
	if err == nil {
		t.Fatal(k, v, err)
	}
}

func TestSeekLast3(t *testing.T) {
	b := TreeNew(cmp)
	b.Set(2, 20)
	b.Set(3, 30)
	b.Set(1, 10)
	en, err := b.SeekLast()
	if err != nil {
		t.Fatal(err)
	}

	k, v, err := en.Prev()
	if k != 3 || v != 30 || err != nil {
		t.Fatal(k, v, err)
	}

	k, v, err = en.Prev()
	if k != 2 || v != 20 || err != nil {
		t.Fatal(k, v, err)
	}

	k, v, err = en.Prev()
	if k != 1 || v != 10 || err != nil {
		t.Fatal(k, v, err)
	}

	k, v, err = en.Prev()
	if err == nil {
		t.Fatal(k, v, err)
	}
}

func TestPut(t *testing.T) {
	tab := []struct {
		pre    []int // even index: K, odd index: V
		newK   int   // Put(newK, ...
		oldV   int   // Put()->oldV
		exists bool  // upd(exists)
		write  bool  // upd()->write
		post   []int // even index: K, odd index: V
	}{
		// 0
		{
			[]int{},
			1, 0, false, false,
			[]int{},
		},
		{
			[]int{},
			1, 0, false, true,
			[]int{1, -1},
		},
		{
			[]int{1, 10},
			0, 0, false, false,
			[]int{1, 10},
		},
		{
			[]int{1, 10},
			0, 0, false, true,
			[]int{0, -1, 1, 10},
		},
		{
			[]int{1, 10},
			1, 10, true, false,
			[]int{1, 10},
		},

		// 5
		{
			[]int{1, 10},
			1, 10, true, true,
			[]int{1, -1},
		},
		{
			[]int{1, 10},
			2, 0, false, false,
			[]int{1, 10},
		},
		{
			[]int{1, 10},
			2, 0, false, true,
			[]int{1, 10, 2, -1},
		},
	}

	for iTest, test := range tab {
		tr := TreeNew(cmp)
		for i := 0; i < len(test.pre); i += 2 {
			k, v := test.pre[i], test.pre[i+1]
			tr.Set(k, v)
		}

		oldV, written := tr.Put(test.newK, func(old interface{}, exists bool) (newV interface{}, write bool) {
			if g, e := exists, test.exists; g != e {
				t.Fatal(iTest, g, e)
			}

			if exists {
				if g, e := old.(int), test.oldV; g != e {
					t.Fatal(iTest, g, e)
				}
			}
			return -1, test.write
		})
		if test.exists {
			if g, e := oldV.(int), test.oldV; g != e {
				t.Fatal(iTest, g, e)
			}
		}

		if g, e := written, test.write; g != e {
			t.Fatal(iTest, g, e)
		}

		n := len(test.post)
		en, err := tr.SeekFirst()
		if err != nil {
			if n == 0 && err == io.EOF {
				continue
			}

			t.Fatal(iTest, err)
		}

		for i := 0; i < len(test.post); i += 2 {
			k, v, err := en.Next()
			if err != nil {
				t.Fatal(iTest, err)
			}

			if g, e := k.(int), test.post[i]; g != e {
				t.Fatal(iTest, g, e)
			}

			if g, e := v.(int), test.post[i+1]; g != e {
				t.Fatal(iTest, g, e)
			}
		}

		_, _, err = en.Next()
		if g, e := err, io.EOF; g != e {
			t.Fatal(iTest, g, e)
		}
	}
}
