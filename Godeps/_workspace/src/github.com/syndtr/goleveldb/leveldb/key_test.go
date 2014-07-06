// Copyright (c) 2012, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package leveldb

import (
	"bytes"
	"testing"

	"github.com/syndtr/goleveldb/leveldb/comparer"
)

var defaultIComparer = &iComparer{comparer.DefaultComparer}

func ikey(key string, seq uint64, t vType) iKey {
	return newIKey([]byte(key), uint64(seq), t)
}

func shortSep(a, b []byte) []byte {
	dst := make([]byte, len(a))
	dst = defaultIComparer.Separator(dst[:0], a, b)
	if dst == nil {
		return a
	}
	return dst
}

func shortSuccessor(b []byte) []byte {
	dst := make([]byte, len(b))
	dst = defaultIComparer.Successor(dst[:0], b)
	if dst == nil {
		return b
	}
	return dst
}

func testSingleKey(t *testing.T, key string, seq uint64, vt vType) {
	ik := ikey(key, seq, vt)

	if !bytes.Equal(ik.ukey(), []byte(key)) {
		t.Errorf("user key does not equal, got %v, want %v", string(ik.ukey()), key)
	}

	if rseq, rt, ok := ik.parseNum(); ok {
		if rseq != seq {
			t.Errorf("seq number does not equal, got %v, want %v", rseq, seq)
		}

		if rt != vt {
			t.Errorf("type does not equal, got %v, want %v", rt, vt)
		}
	} else {
		t.Error("cannot parse seq and type")
	}
}

func TestIKey_EncodeDecode(t *testing.T) {
	keys := []string{"", "k", "hello", "longggggggggggggggggggggg"}
	seqs := []uint64{
		1, 2, 3,
		(1 << 8) - 1, 1 << 8, (1 << 8) + 1,
		(1 << 16) - 1, 1 << 16, (1 << 16) + 1,
		(1 << 32) - 1, 1 << 32, (1 << 32) + 1,
	}
	for _, key := range keys {
		for _, seq := range seqs {
			testSingleKey(t, key, seq, tVal)
			testSingleKey(t, "hello", 1, tDel)
		}
	}
}

func assertBytes(t *testing.T, want, got []byte) {
	if !bytes.Equal(got, want) {
		t.Errorf("assert failed, got %v, want %v", got, want)
	}
}

func TestIKeyShortSeparator(t *testing.T) {
	// When user keys are same
	assertBytes(t, ikey("foo", 100, tVal),
		shortSep(ikey("foo", 100, tVal),
			ikey("foo", 99, tVal)))
	assertBytes(t, ikey("foo", 100, tVal),
		shortSep(ikey("foo", 100, tVal),
			ikey("foo", 101, tVal)))
	assertBytes(t, ikey("foo", 100, tVal),
		shortSep(ikey("foo", 100, tVal),
			ikey("foo", 100, tVal)))
	assertBytes(t, ikey("foo", 100, tVal),
		shortSep(ikey("foo", 100, tVal),
			ikey("foo", 100, tDel)))

	// When user keys are misordered
	assertBytes(t, ikey("foo", 100, tVal),
		shortSep(ikey("foo", 100, tVal),
			ikey("bar", 99, tVal)))

	// When user keys are different, but correctly ordered
	assertBytes(t, ikey("g", uint64(kMaxSeq), tSeek),
		shortSep(ikey("foo", 100, tVal),
			ikey("hello", 200, tVal)))

	// When start user key is prefix of limit user key
	assertBytes(t, ikey("foo", 100, tVal),
		shortSep(ikey("foo", 100, tVal),
			ikey("foobar", 200, tVal)))

	// When limit user key is prefix of start user key
	assertBytes(t, ikey("foobar", 100, tVal),
		shortSep(ikey("foobar", 100, tVal),
			ikey("foo", 200, tVal)))
}

func TestIKeyShortestSuccessor(t *testing.T) {
	assertBytes(t, ikey("g", uint64(kMaxSeq), tSeek),
		shortSuccessor(ikey("foo", 100, tVal)))
	assertBytes(t, ikey("\xff\xff", 100, tVal),
		shortSuccessor(ikey("\xff\xff", 100, tVal)))
}
