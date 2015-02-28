// Copyright (c) 2012, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package leveldb

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/syndtr/goleveldb/leveldb/iterator"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/storage"
)

func randomString(r *rand.Rand, n int) []byte {
	b := new(bytes.Buffer)
	for i := 0; i < n; i++ {
		b.WriteByte(' ' + byte(r.Intn(95)))
	}
	return b.Bytes()
}

func compressibleStr(r *rand.Rand, frac float32, n int) []byte {
	nn := int(float32(n) * frac)
	rb := randomString(r, nn)
	b := make([]byte, 0, n+nn)
	for len(b) < n {
		b = append(b, rb...)
	}
	return b[:n]
}

type valueGen struct {
	src []byte
	pos int
}

func newValueGen(frac float32) *valueGen {
	v := new(valueGen)
	r := rand.New(rand.NewSource(301))
	v.src = make([]byte, 0, 1048576+100)
	for len(v.src) < 1048576 {
		v.src = append(v.src, compressibleStr(r, frac, 100)...)
	}
	return v
}

func (v *valueGen) get(n int) []byte {
	if v.pos+n > len(v.src) {
		v.pos = 0
	}
	v.pos += n
	return v.src[v.pos-n : v.pos]
}

var benchDB = filepath.Join(os.TempDir(), fmt.Sprintf("goleveldbbench-%d", os.Getuid()))

type dbBench struct {
	b    *testing.B
	stor storage.Storage
	db   *DB

	o  *opt.Options
	ro *opt.ReadOptions
	wo *opt.WriteOptions

	keys, values [][]byte
}

func openDBBench(b *testing.B, noCompress bool) *dbBench {
	_, err := os.Stat(benchDB)
	if err == nil {
		err = os.RemoveAll(benchDB)
		if err != nil {
			b.Fatal("cannot remove old db: ", err)
		}
	}

	p := &dbBench{
		b:  b,
		o:  &opt.Options{},
		ro: &opt.ReadOptions{},
		wo: &opt.WriteOptions{},
	}
	p.stor, err = storage.OpenFile(benchDB)
	if err != nil {
		b.Fatal("cannot open stor: ", err)
	}
	if noCompress {
		p.o.Compression = opt.NoCompression
	}

	p.db, err = Open(p.stor, p.o)
	if err != nil {
		b.Fatal("cannot open db: ", err)
	}

	runtime.GOMAXPROCS(runtime.NumCPU())
	return p
}

func (p *dbBench) reopen() {
	p.db.Close()
	var err error
	p.db, err = Open(p.stor, p.o)
	if err != nil {
		p.b.Fatal("Reopen: got error: ", err)
	}
}

func (p *dbBench) populate(n int) {
	p.keys, p.values = make([][]byte, n), make([][]byte, n)
	v := newValueGen(0.5)
	for i := range p.keys {
		p.keys[i], p.values[i] = []byte(fmt.Sprintf("%016d", i)), v.get(100)
	}
}

func (p *dbBench) randomize() {
	m := len(p.keys)
	times := m * 2
	r1, r2 := rand.New(rand.NewSource(0xdeadbeef)), rand.New(rand.NewSource(0xbeefface))
	for n := 0; n < times; n++ {
		i, j := r1.Int()%m, r2.Int()%m
		if i == j {
			continue
		}
		p.keys[i], p.keys[j] = p.keys[j], p.keys[i]
		p.values[i], p.values[j] = p.values[j], p.values[i]
	}
}

func (p *dbBench) writes(perBatch int) {
	b := p.b
	db := p.db

	n := len(p.keys)
	m := n / perBatch
	if n%perBatch > 0 {
		m++
	}
	batches := make([]Batch, m)
	j := 0
	for i := range batches {
		first := true
		for ; j < n && ((j+1)%perBatch != 0 || first); j++ {
			first = false
			batches[i].Put(p.keys[j], p.values[j])
		}
	}
	runtime.GC()

	b.ResetTimer()
	b.StartTimer()
	for i := range batches {
		err := db.Write(&(batches[i]), p.wo)
		if err != nil {
			b.Fatal("write failed: ", err)
		}
	}
	b.StopTimer()
	b.SetBytes(116)
}

func (p *dbBench) gc() {
	p.keys, p.values = nil, nil
	runtime.GC()
}

func (p *dbBench) puts() {
	b := p.b
	db := p.db

	b.ResetTimer()
	b.StartTimer()
	for i := range p.keys {
		err := db.Put(p.keys[i], p.values[i], p.wo)
		if err != nil {
			b.Fatal("put failed: ", err)
		}
	}
	b.StopTimer()
	b.SetBytes(116)
}

func (p *dbBench) fill() {
	b := p.b
	db := p.db

	perBatch := 10000
	batch := new(Batch)
	for i, n := 0, len(p.keys); i < n; {
		first := true
		for ; i < n && ((i+1)%perBatch != 0 || first); i++ {
			first = false
			batch.Put(p.keys[i], p.values[i])
		}
		err := db.Write(batch, p.wo)
		if err != nil {
			b.Fatal("write failed: ", err)
		}
		batch.Reset()
	}
}

func (p *dbBench) gets() {
	b := p.b
	db := p.db

	b.ResetTimer()
	for i := range p.keys {
		_, err := db.Get(p.keys[i], p.ro)
		if err != nil {
			b.Error("got error: ", err)
		}
	}
	b.StopTimer()
}

func (p *dbBench) seeks() {
	b := p.b

	iter := p.newIter()
	defer iter.Release()
	b.ResetTimer()
	for i := range p.keys {
		if !iter.Seek(p.keys[i]) {
			b.Error("value not found for: ", string(p.keys[i]))
		}
	}
	b.StopTimer()
}

func (p *dbBench) newIter() iterator.Iterator {
	iter := p.db.NewIterator(nil, p.ro)
	err := iter.Error()
	if err != nil {
		p.b.Fatal("cannot create iterator: ", err)
	}
	return iter
}

func (p *dbBench) close() {
	if bp, err := p.db.GetProperty("leveldb.blockpool"); err == nil {
		p.b.Log("Block pool stats: ", bp)
	}
	p.db.Close()
	p.stor.Close()
	os.RemoveAll(benchDB)
	p.db = nil
	p.keys = nil
	p.values = nil
	runtime.GC()
	runtime.GOMAXPROCS(1)
}

func BenchmarkDBWrite(b *testing.B) {
	p := openDBBench(b, false)
	p.populate(b.N)
	p.writes(1)
	p.close()
}

func BenchmarkDBWriteBatch(b *testing.B) {
	p := openDBBench(b, false)
	p.populate(b.N)
	p.writes(1000)
	p.close()
}

func BenchmarkDBWriteUncompressed(b *testing.B) {
	p := openDBBench(b, true)
	p.populate(b.N)
	p.writes(1)
	p.close()
}

func BenchmarkDBWriteBatchUncompressed(b *testing.B) {
	p := openDBBench(b, true)
	p.populate(b.N)
	p.writes(1000)
	p.close()
}

func BenchmarkDBWriteRandom(b *testing.B) {
	p := openDBBench(b, false)
	p.populate(b.N)
	p.randomize()
	p.writes(1)
	p.close()
}

func BenchmarkDBWriteRandomSync(b *testing.B) {
	p := openDBBench(b, false)
	p.wo.Sync = true
	p.populate(b.N)
	p.writes(1)
	p.close()
}

func BenchmarkDBOverwrite(b *testing.B) {
	p := openDBBench(b, false)
	p.populate(b.N)
	p.writes(1)
	p.writes(1)
	p.close()
}

func BenchmarkDBOverwriteRandom(b *testing.B) {
	p := openDBBench(b, false)
	p.populate(b.N)
	p.writes(1)
	p.randomize()
	p.writes(1)
	p.close()
}

func BenchmarkDBPut(b *testing.B) {
	p := openDBBench(b, false)
	p.populate(b.N)
	p.puts()
	p.close()
}

func BenchmarkDBRead(b *testing.B) {
	p := openDBBench(b, false)
	p.populate(b.N)
	p.fill()
	p.gc()

	iter := p.newIter()
	b.ResetTimer()
	for iter.Next() {
	}
	iter.Release()
	b.StopTimer()
	b.SetBytes(116)
	p.close()
}

func BenchmarkDBReadGC(b *testing.B) {
	p := openDBBench(b, false)
	p.populate(b.N)
	p.fill()

	iter := p.newIter()
	b.ResetTimer()
	for iter.Next() {
	}
	iter.Release()
	b.StopTimer()
	b.SetBytes(116)
	p.close()
}

func BenchmarkDBReadUncompressed(b *testing.B) {
	p := openDBBench(b, true)
	p.populate(b.N)
	p.fill()
	p.gc()

	iter := p.newIter()
	b.ResetTimer()
	for iter.Next() {
	}
	iter.Release()
	b.StopTimer()
	b.SetBytes(116)
	p.close()
}

func BenchmarkDBReadTable(b *testing.B) {
	p := openDBBench(b, false)
	p.populate(b.N)
	p.fill()
	p.reopen()
	p.gc()

	iter := p.newIter()
	b.ResetTimer()
	for iter.Next() {
	}
	iter.Release()
	b.StopTimer()
	b.SetBytes(116)
	p.close()
}

func BenchmarkDBReadReverse(b *testing.B) {
	p := openDBBench(b, false)
	p.populate(b.N)
	p.fill()
	p.gc()

	iter := p.newIter()
	b.ResetTimer()
	iter.Last()
	for iter.Prev() {
	}
	iter.Release()
	b.StopTimer()
	b.SetBytes(116)
	p.close()
}

func BenchmarkDBReadReverseTable(b *testing.B) {
	p := openDBBench(b, false)
	p.populate(b.N)
	p.fill()
	p.reopen()
	p.gc()

	iter := p.newIter()
	b.ResetTimer()
	iter.Last()
	for iter.Prev() {
	}
	iter.Release()
	b.StopTimer()
	b.SetBytes(116)
	p.close()
}

func BenchmarkDBSeek(b *testing.B) {
	p := openDBBench(b, false)
	p.populate(b.N)
	p.fill()
	p.seeks()
	p.close()
}

func BenchmarkDBSeekRandom(b *testing.B) {
	p := openDBBench(b, false)
	p.populate(b.N)
	p.fill()
	p.randomize()
	p.seeks()
	p.close()
}

func BenchmarkDBGet(b *testing.B) {
	p := openDBBench(b, false)
	p.populate(b.N)
	p.fill()
	p.gets()
	p.close()
}

func BenchmarkDBGetRandom(b *testing.B) {
	p := openDBBench(b, false)
	p.populate(b.N)
	p.fill()
	p.randomize()
	p.gets()
	p.close()
}
