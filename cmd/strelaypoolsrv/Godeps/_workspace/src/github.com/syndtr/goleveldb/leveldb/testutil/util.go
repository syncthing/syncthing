// Copyright (c) 2014, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testutil

import (
	"bytes"
	"flag"
	"math/rand"
	"reflect"
	"sync"

	"github.com/onsi/ginkgo/config"

	"github.com/syndtr/goleveldb/leveldb/comparer"
)

var (
	runfn = make(map[string][]func())
	runmu sync.Mutex
)

func Defer(args ...interface{}) bool {
	var (
		group string
		fn    func()
	)
	for _, arg := range args {
		v := reflect.ValueOf(arg)
		switch v.Kind() {
		case reflect.String:
			group = v.String()
		case reflect.Func:
			r := reflect.ValueOf(&fn).Elem()
			r.Set(v)
		}
	}
	if fn != nil {
		runmu.Lock()
		runfn[group] = append(runfn[group], fn)
		runmu.Unlock()
	}
	return true
}

func RunDefer(groups ...string) bool {
	if len(groups) == 0 {
		groups = append(groups, "")
	}
	runmu.Lock()
	var runfn_ []func()
	for _, group := range groups {
		runfn_ = append(runfn_, runfn[group]...)
		delete(runfn, group)
	}
	runmu.Unlock()
	for _, fn := range runfn_ {
		fn()
	}
	return runfn_ != nil
}

func RandomSeed() int64 {
	if !flag.Parsed() {
		panic("random seed not initialized")
	}
	return config.GinkgoConfig.RandomSeed
}

func NewRand() *rand.Rand {
	return rand.New(rand.NewSource(RandomSeed()))
}

var cmp = comparer.DefaultComparer

func BytesSeparator(a, b []byte) []byte {
	if bytes.Equal(a, b) {
		return b
	}
	i, n := 0, len(a)
	if n > len(b) {
		n = len(b)
	}
	for ; i < n && (a[i] == b[i]); i++ {
	}
	x := append([]byte{}, a[:i]...)
	if i < n {
		if c := a[i] + 1; c < b[i] {
			return append(x, c)
		}
		x = append(x, a[i])
		i++
	}
	for ; i < len(a); i++ {
		if c := a[i]; c < 0xff {
			return append(x, c+1)
		} else {
			x = append(x, c)
		}
	}
	if len(b) > i && b[i] > 0 {
		return append(x, b[i]-1)
	}
	return append(x, 'x')
}

func BytesAfter(b []byte) []byte {
	var x []byte
	for _, c := range b {
		if c < 0xff {
			return append(x, c+1)
		} else {
			x = append(x, c)
		}
	}
	return append(x, 'x')
}

func RandomIndex(rnd *rand.Rand, n, round int, fn func(i int)) {
	if rnd == nil {
		rnd = NewRand()
	}
	for x := 0; x < round; x++ {
		fn(rnd.Intn(n))
	}
	return
}

func ShuffledIndex(rnd *rand.Rand, n, round int, fn func(i int)) {
	if rnd == nil {
		rnd = NewRand()
	}
	for x := 0; x < round; x++ {
		for _, i := range rnd.Perm(n) {
			fn(i)
		}
	}
	return
}

func RandomRange(rnd *rand.Rand, n, round int, fn func(start, limit int)) {
	if rnd == nil {
		rnd = NewRand()
	}
	for x := 0; x < round; x++ {
		start := rnd.Intn(n)
		length := 0
		if j := n - start; j > 0 {
			length = rnd.Intn(j)
		}
		fn(start, start+length)
	}
	return
}

func Max(x, y int) int {
	if x > y {
		return x
	}
	return y
}

func Min(x, y int) int {
	if x < y {
		return x
	}
	return y
}
