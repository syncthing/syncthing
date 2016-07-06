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
	"github.com/syndtr/goleveldb/leveldb/memdb"
)

type tbRec struct {
	kt         keyType
	key, value []byte
}

type testBatch struct {
	rec []*tbRec
}

func (p *testBatch) Put(key, value []byte) {
	p.rec = append(p.rec, &tbRec{keyTypeVal, key, value})
}

func (p *testBatch) Delete(key []byte) {
	p.rec = append(p.rec, &tbRec{keyTypeDel, key, nil})
}

func compareBatch(t *testing.T, b1, b2 *Batch) {
	if b1.seq != b2.seq {
		t.Errorf("invalid seq number want %d, got %d", b1.seq, b2.seq)
	}
	if b1.Len() != b2.Len() {
		t.Fatalf("invalid record length want %d, got %d", b1.Len(), b2.Len())
	}
	p1, p2 := new(testBatch), new(testBatch)
	err := b1.Replay(p1)
	if err != nil {
		t.Fatal("error when replaying batch 1: ", err)
	}
	err = b2.Replay(p2)
	if err != nil {
		t.Fatal("error when replaying batch 2: ", err)
	}
	for i := range p1.rec {
		r1, r2 := p1.rec[i], p2.rec[i]
		if r1.kt != r2.kt {
			t.Errorf("invalid type on record '%d' want %d, got %d", i, r1.kt, r2.kt)
		}
		if !bytes.Equal(r1.key, r2.key) {
			t.Errorf("invalid key on record '%d' want %s, got %s", i, string(r1.key), string(r2.key))
		}
		if r1.kt == keyTypeVal {
			if !bytes.Equal(r1.value, r2.value) {
				t.Errorf("invalid value on record '%d' want %s, got %s", i, string(r1.value), string(r2.value))
			}
		}
	}
}

func TestBatch_EncodeDecode(t *testing.T) {
	b1 := new(Batch)
	b1.seq = 10009
	b1.Put([]byte("key1"), []byte("value1"))
	b1.Put([]byte("key2"), []byte("value2"))
	b1.Delete([]byte("key1"))
	b1.Put([]byte("k"), []byte(""))
	b1.Put([]byte("zzzzzzzzzzz"), []byte("zzzzzzzzzzzzzzzzzzzzzzzz"))
	b1.Delete([]byte("key10000"))
	b1.Delete([]byte("k"))
	buf := b1.encode()
	b2 := new(Batch)
	err := b2.decode(0, buf)
	if err != nil {
		t.Error("error when decoding batch: ", err)
	}
	compareBatch(t, b1, b2)
}

func TestBatch_Append(t *testing.T) {
	b1 := new(Batch)
	b1.seq = 10009
	b1.Put([]byte("key1"), []byte("value1"))
	b1.Put([]byte("key2"), []byte("value2"))
	b1.Delete([]byte("key1"))
	b1.Put([]byte("foo"), []byte("foovalue"))
	b1.Put([]byte("bar"), []byte("barvalue"))
	b2a := new(Batch)
	b2a.seq = 10009
	b2a.Put([]byte("key1"), []byte("value1"))
	b2a.Put([]byte("key2"), []byte("value2"))
	b2a.Delete([]byte("key1"))
	b2b := new(Batch)
	b2b.Put([]byte("foo"), []byte("foovalue"))
	b2b.Put([]byte("bar"), []byte("barvalue"))
	b2a.append(b2b)
	compareBatch(t, b1, b2a)
	if b1.size() != b2a.size() {
		t.Fatalf("invalid batch size want %d, got %d", b1.size(), b2a.size())
	}
}

func TestBatch_Size(t *testing.T) {
	b := new(Batch)
	for i := 0; i < 2; i++ {
		b.Put([]byte("key1"), []byte("value1"))
		b.Put([]byte("key2"), []byte("value2"))
		b.Delete([]byte("key1"))
		b.Put([]byte("foo"), []byte("foovalue"))
		b.Put([]byte("bar"), []byte("barvalue"))
		mem := memdb.New(&iComparer{comparer.DefaultComparer}, 0)
		b.memReplay(mem)
		if b.size() != mem.Size() {
			t.Errorf("invalid batch size calculation, want=%d got=%d", mem.Size(), b.size())
		}
		b.Reset()
	}
}
