// Copyright (c) 2014, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testutil

import (
	"fmt"
	"math/rand"
	"sort"
	"strings"

	"github.com/syndtr/goleveldb/leveldb/util"
)

type KeyValueEntry struct {
	key, value []byte
}

type KeyValue struct {
	entries []KeyValueEntry
	nbytes  int
}

func (kv *KeyValue) Put(key, value []byte) {
	if n := len(kv.entries); n > 0 && cmp.Compare(kv.entries[n-1].key, key) >= 0 {
		panic(fmt.Sprintf("Put: keys are not in increasing order: %q, %q", kv.entries[n-1].key, key))
	}
	kv.entries = append(kv.entries, KeyValueEntry{key, value})
	kv.nbytes += len(key) + len(value)
}

func (kv *KeyValue) PutString(key, value string) {
	kv.Put([]byte(key), []byte(value))
}

func (kv *KeyValue) PutU(key, value []byte) bool {
	if i, exist := kv.Get(key); !exist {
		if i < kv.Len() {
			kv.entries = append(kv.entries[:i+1], kv.entries[i:]...)
			kv.entries[i] = KeyValueEntry{key, value}
		} else {
			kv.entries = append(kv.entries, KeyValueEntry{key, value})
		}
		kv.nbytes += len(key) + len(value)
		return true
	} else {
		kv.nbytes += len(value) - len(kv.ValueAt(i))
		kv.entries[i].value = value
	}
	return false
}

func (kv *KeyValue) PutUString(key, value string) bool {
	return kv.PutU([]byte(key), []byte(value))
}

func (kv *KeyValue) Delete(key []byte) (exist bool, value []byte) {
	i, exist := kv.Get(key)
	if exist {
		value = kv.entries[i].value
		kv.DeleteIndex(i)
	}
	return
}

func (kv *KeyValue) DeleteIndex(i int) bool {
	if i < kv.Len() {
		kv.nbytes -= len(kv.KeyAt(i)) + len(kv.ValueAt(i))
		kv.entries = append(kv.entries[:i], kv.entries[i+1:]...)
		return true
	}
	return false
}

func (kv KeyValue) Len() int {
	return len(kv.entries)
}

func (kv *KeyValue) Size() int {
	return kv.nbytes
}

func (kv KeyValue) KeyAt(i int) []byte {
	return kv.entries[i].key
}

func (kv KeyValue) ValueAt(i int) []byte {
	return kv.entries[i].value
}

func (kv KeyValue) Index(i int) (key, value []byte) {
	if i < 0 || i >= len(kv.entries) {
		panic(fmt.Sprintf("Index #%d: out of range", i))
	}
	return kv.entries[i].key, kv.entries[i].value
}

func (kv KeyValue) IndexInexact(i int) (key_, key, value []byte) {
	key, value = kv.Index(i)
	var key0 []byte
	var key1 = kv.KeyAt(i)
	if i > 0 {
		key0 = kv.KeyAt(i - 1)
	}
	key_ = BytesSeparator(key0, key1)
	return
}

func (kv KeyValue) IndexOrNil(i int) (key, value []byte) {
	if i >= 0 && i < len(kv.entries) {
		return kv.entries[i].key, kv.entries[i].value
	}
	return nil, nil
}

func (kv KeyValue) IndexString(i int) (key, value string) {
	key_, _value := kv.Index(i)
	return string(key_), string(_value)
}

func (kv KeyValue) Search(key []byte) int {
	return sort.Search(kv.Len(), func(i int) bool {
		return cmp.Compare(kv.KeyAt(i), key) >= 0
	})
}

func (kv KeyValue) SearchString(key string) int {
	return kv.Search([]byte(key))
}

func (kv KeyValue) Get(key []byte) (i int, exist bool) {
	i = kv.Search(key)
	if i < kv.Len() && cmp.Compare(kv.KeyAt(i), key) == 0 {
		exist = true
	}
	return
}

func (kv KeyValue) GetString(key string) (i int, exist bool) {
	return kv.Get([]byte(key))
}

func (kv KeyValue) Iterate(fn func(i int, key, value []byte)) {
	for i, x := range kv.entries {
		fn(i, x.key, x.value)
	}
}

func (kv KeyValue) IterateString(fn func(i int, key, value string)) {
	kv.Iterate(func(i int, key, value []byte) {
		fn(i, string(key), string(value))
	})
}

func (kv KeyValue) IterateShuffled(rnd *rand.Rand, fn func(i int, key, value []byte)) {
	ShuffledIndex(rnd, kv.Len(), 1, func(i int) {
		fn(i, kv.entries[i].key, kv.entries[i].value)
	})
}

func (kv KeyValue) IterateShuffledString(rnd *rand.Rand, fn func(i int, key, value string)) {
	kv.IterateShuffled(rnd, func(i int, key, value []byte) {
		fn(i, string(key), string(value))
	})
}

func (kv KeyValue) IterateInexact(fn func(i int, key_, key, value []byte)) {
	for i := range kv.entries {
		key_, key, value := kv.IndexInexact(i)
		fn(i, key_, key, value)
	}
}

func (kv KeyValue) IterateInexactString(fn func(i int, key_, key, value string)) {
	kv.IterateInexact(func(i int, key_, key, value []byte) {
		fn(i, string(key_), string(key), string(value))
	})
}

func (kv KeyValue) Clone() KeyValue {
	return KeyValue{append([]KeyValueEntry{}, kv.entries...), kv.nbytes}
}

func (kv KeyValue) Slice(start, limit int) KeyValue {
	if start < 0 || limit > kv.Len() {
		panic(fmt.Sprintf("Slice %d .. %d: out of range", start, limit))
	} else if limit < start {
		panic(fmt.Sprintf("Slice %d .. %d: invalid range", start, limit))
	}
	return KeyValue{append([]KeyValueEntry{}, kv.entries[start:limit]...), kv.nbytes}
}

func (kv KeyValue) SliceKey(start, limit []byte) KeyValue {
	start_ := 0
	limit_ := kv.Len()
	if start != nil {
		start_ = kv.Search(start)
	}
	if limit != nil {
		limit_ = kv.Search(limit)
	}
	return kv.Slice(start_, limit_)
}

func (kv KeyValue) SliceKeyString(start, limit string) KeyValue {
	return kv.SliceKey([]byte(start), []byte(limit))
}

func (kv KeyValue) SliceRange(r *util.Range) KeyValue {
	if r != nil {
		return kv.SliceKey(r.Start, r.Limit)
	}
	return kv.Clone()
}

func (kv KeyValue) Range(start, limit int) (r util.Range) {
	if kv.Len() > 0 {
		if start == kv.Len() {
			r.Start = BytesAfter(kv.KeyAt(start - 1))
		} else {
			r.Start = kv.KeyAt(start)
		}
	}
	if limit < kv.Len() {
		r.Limit = kv.KeyAt(limit)
	}
	return
}

func KeyValue_EmptyKey() *KeyValue {
	kv := &KeyValue{}
	kv.PutString("", "v")
	return kv
}

func KeyValue_EmptyValue() *KeyValue {
	kv := &KeyValue{}
	kv.PutString("abc", "")
	kv.PutString("abcd", "")
	return kv
}

func KeyValue_OneKeyValue() *KeyValue {
	kv := &KeyValue{}
	kv.PutString("abc", "v")
	return kv
}

func KeyValue_BigValue() *KeyValue {
	kv := &KeyValue{}
	kv.PutString("big1", strings.Repeat("1", 200000))
	return kv
}

func KeyValue_SpecialKey() *KeyValue {
	kv := &KeyValue{}
	kv.PutString("\xff\xff", "v3")
	return kv
}

func KeyValue_MultipleKeyValue() *KeyValue {
	kv := &KeyValue{}
	kv.PutString("a", "v")
	kv.PutString("aa", "v1")
	kv.PutString("aaa", "v2")
	kv.PutString("aaacccccccccc", "v2")
	kv.PutString("aaaccccccccccd", "v3")
	kv.PutString("aaaccccccccccf", "v4")
	kv.PutString("aaaccccccccccfg", "v5")
	kv.PutString("ab", "v6")
	kv.PutString("abc", "v7")
	kv.PutString("abcd", "v8")
	kv.PutString("accccccccccccccc", "v9")
	kv.PutString("b", "v10")
	kv.PutString("bb", "v11")
	kv.PutString("bc", "v12")
	kv.PutString("c", "v13")
	kv.PutString("c1", "v13")
	kv.PutString("czzzzzzzzzzzzzz", "v14")
	kv.PutString("fffffffffffffff", "v15")
	kv.PutString("g11", "v15")
	kv.PutString("g111", "v15")
	kv.PutString("g111\xff", "v15")
	kv.PutString("zz", "v16")
	kv.PutString("zzzzzzz", "v16")
	kv.PutString("zzzzzzzzzzzzzzzz", "v16")
	return kv
}

var keymap = []byte("012345678ABCDEFGHIJKLMNOPQRSTUVWXYabcdefghijklmnopqrstuvwxy")

func KeyValue_Generate(rnd *rand.Rand, n, incr, minlen, maxlen, vminlen, vmaxlen int) *KeyValue {
	if rnd == nil {
		rnd = NewRand()
	}
	if maxlen < minlen {
		panic("max len should >= min len")
	}

	rrand := func(min, max int) int {
		if min == max {
			return max
		}
		return rnd.Intn(max-min) + min
	}

	kv := &KeyValue{}
	endC := byte(len(keymap) - incr)
	gen := make([]byte, 0, maxlen)
	for i := 0; i < n; i++ {
		m := rrand(minlen, maxlen)
		last := gen
	retry:
		gen = last[:m]
		if k := len(last); m > k {
			for j := k; j < m; j++ {
				gen[j] = 0
			}
		} else {
			for j := m - 1; j >= 0; j-- {
				c := last[j]
				if c == endC {
					continue
				}
				gen[j] = c + byte(incr)
				for j++; j < m; j++ {
					gen[j] = 0
				}
				goto ok
			}
			if m < maxlen {
				m++
				goto retry
			}
			panic(fmt.Sprintf("only able to generate %d keys out of %d keys, try increasing max len", kv.Len(), n))
		ok:
		}
		key := make([]byte, m)
		for j := 0; j < m; j++ {
			key[j] = keymap[gen[j]]
		}
		value := make([]byte, rrand(vminlen, vmaxlen))
		for n := copy(value, []byte(fmt.Sprintf("v%d", i))); n < len(value); n++ {
			value[n] = 'x'
		}
		kv.Put(key, value)
	}
	return kv
}
