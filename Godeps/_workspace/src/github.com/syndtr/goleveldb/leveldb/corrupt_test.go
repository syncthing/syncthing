// Copyright (c) 2013, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package leveldb

import (
	"bytes"
	"fmt"
	"github.com/syndtr/goleveldb/leveldb/filter"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/storage"
	"io"
	"math/rand"
	"testing"
)

const ctValSize = 1000

type dbCorruptHarness struct {
	dbHarness
}

func newDbCorruptHarnessWopt(t *testing.T, o *opt.Options) *dbCorruptHarness {
	h := new(dbCorruptHarness)
	h.init(t, o)
	return h
}

func newDbCorruptHarness(t *testing.T) *dbCorruptHarness {
	return newDbCorruptHarnessWopt(t, &opt.Options{
		BlockCacheCapacity: 100,
		Strict:             opt.StrictJournalChecksum,
	})
}

func (h *dbCorruptHarness) recover() {
	p := &h.dbHarness
	t := p.t

	var err error
	p.db, err = Recover(h.stor, h.o)
	if err != nil {
		t.Fatal("Repair: got error: ", err)
	}
}

func (h *dbCorruptHarness) build(n int) {
	p := &h.dbHarness
	t := p.t
	db := p.db

	batch := new(Batch)
	for i := 0; i < n; i++ {
		batch.Reset()
		batch.Put(tkey(i), tval(i, ctValSize))
		err := db.Write(batch, p.wo)
		if err != nil {
			t.Fatal("write error: ", err)
		}
	}
}

func (h *dbCorruptHarness) buildShuffled(n int, rnd *rand.Rand) {
	p := &h.dbHarness
	t := p.t
	db := p.db

	batch := new(Batch)
	for i := range rnd.Perm(n) {
		batch.Reset()
		batch.Put(tkey(i), tval(i, ctValSize))
		err := db.Write(batch, p.wo)
		if err != nil {
			t.Fatal("write error: ", err)
		}
	}
}

func (h *dbCorruptHarness) deleteRand(n, max int, rnd *rand.Rand) {
	p := &h.dbHarness
	t := p.t
	db := p.db

	batch := new(Batch)
	for i := 0; i < n; i++ {
		batch.Reset()
		batch.Delete(tkey(rnd.Intn(max)))
		err := db.Write(batch, p.wo)
		if err != nil {
			t.Fatal("write error: ", err)
		}
	}
}

func (h *dbCorruptHarness) corrupt(ft storage.FileType, fi, offset, n int) {
	p := &h.dbHarness
	t := p.t

	ff, _ := p.stor.GetFiles(ft)
	sff := files(ff)
	sff.sort()
	if fi < 0 {
		fi = len(sff) - 1
	}
	if fi >= len(sff) {
		t.Fatalf("no such file with type %q with index %d", ft, fi)
	}

	file := sff[fi]

	r, err := file.Open()
	if err != nil {
		t.Fatal("cannot open file: ", err)
	}
	x, err := r.Seek(0, 2)
	if err != nil {
		t.Fatal("cannot query file size: ", err)
	}
	m := int(x)
	if _, err := r.Seek(0, 0); err != nil {
		t.Fatal(err)
	}

	if offset < 0 {
		if -offset > m {
			offset = 0
		} else {
			offset = m + offset
		}
	}
	if offset > m {
		offset = m
	}
	if offset+n > m {
		n = m - offset
	}

	buf := make([]byte, m)
	_, err = io.ReadFull(r, buf)
	if err != nil {
		t.Fatal("cannot read file: ", err)
	}
	r.Close()

	for i := 0; i < n; i++ {
		buf[offset+i] ^= 0x80
	}

	err = file.Remove()
	if err != nil {
		t.Fatal("cannot remove old file: ", err)
	}
	w, err := file.Create()
	if err != nil {
		t.Fatal("cannot create new file: ", err)
	}
	_, err = w.Write(buf)
	if err != nil {
		t.Fatal("cannot write new file: ", err)
	}
	w.Close()
}

func (h *dbCorruptHarness) removeAll(ft storage.FileType) {
	ff, err := h.stor.GetFiles(ft)
	if err != nil {
		h.t.Fatal("get files: ", err)
	}
	for _, f := range ff {
		if err := f.Remove(); err != nil {
			h.t.Error("remove file: ", err)
		}
	}
}

func (h *dbCorruptHarness) removeOne(ft storage.FileType) {
	ff, err := h.stor.GetFiles(ft)
	if err != nil {
		h.t.Fatal("get files: ", err)
	}
	f := ff[rand.Intn(len(ff))]
	h.t.Logf("removing file @%d", f.Num())
	if err := f.Remove(); err != nil {
		h.t.Error("remove file: ", err)
	}
}

func (h *dbCorruptHarness) check(min, max int) {
	p := &h.dbHarness
	t := p.t
	db := p.db

	var n, badk, badv, missed, good int
	iter := db.NewIterator(nil, p.ro)
	for iter.Next() {
		k := 0
		fmt.Sscanf(string(iter.Key()), "%d", &k)
		if k < n {
			badk++
			continue
		}
		missed += k - n
		n = k + 1
		if !bytes.Equal(iter.Value(), tval(k, ctValSize)) {
			badv++
		} else {
			good++
		}
	}
	err := iter.Error()
	iter.Release()
	t.Logf("want=%d..%d got=%d badkeys=%d badvalues=%d missed=%d, err=%v",
		min, max, good, badk, badv, missed, err)
	if good < min || good > max {
		t.Errorf("good entries number not in range")
	}
}

func TestCorruptDB_Journal(t *testing.T) {
	h := newDbCorruptHarness(t)

	h.build(100)
	h.check(100, 100)
	h.closeDB()
	h.corrupt(storage.TypeJournal, -1, 19, 1)
	h.corrupt(storage.TypeJournal, -1, 32*1024+1000, 1)

	h.openDB()
	h.check(36, 36)

	h.close()
}

func TestCorruptDB_Table(t *testing.T) {
	h := newDbCorruptHarness(t)

	h.build(100)
	h.compactMem()
	h.compactRangeAt(0, "", "")
	h.compactRangeAt(1, "", "")
	h.closeDB()
	h.corrupt(storage.TypeTable, -1, 100, 1)

	h.openDB()
	h.check(99, 99)

	h.close()
}

func TestCorruptDB_TableIndex(t *testing.T) {
	h := newDbCorruptHarness(t)

	h.build(10000)
	h.compactMem()
	h.closeDB()
	h.corrupt(storage.TypeTable, -1, -2000, 500)

	h.openDB()
	h.check(5000, 9999)

	h.close()
}

func TestCorruptDB_MissingManifest(t *testing.T) {
	rnd := rand.New(rand.NewSource(0x0badda7a))
	h := newDbCorruptHarnessWopt(t, &opt.Options{
		BlockCacheCapacity: 100,
		Strict:             opt.StrictJournalChecksum,
		WriteBuffer:        1000 * 60,
	})

	h.build(1000)
	h.compactMem()
	h.buildShuffled(1000, rnd)
	h.compactMem()
	h.deleteRand(500, 1000, rnd)
	h.compactMem()
	h.buildShuffled(1000, rnd)
	h.compactMem()
	h.deleteRand(500, 1000, rnd)
	h.compactMem()
	h.buildShuffled(1000, rnd)
	h.compactMem()
	h.closeDB()

	h.stor.SetIgnoreOpenErr(storage.TypeManifest)
	h.removeAll(storage.TypeManifest)
	h.openAssert(false)
	h.stor.SetIgnoreOpenErr(0)

	h.recover()
	h.check(1000, 1000)
	h.build(1000)
	h.compactMem()
	h.compactRange("", "")
	h.closeDB()

	h.recover()
	h.check(1000, 1000)

	h.close()
}

func TestCorruptDB_SequenceNumberRecovery(t *testing.T) {
	h := newDbCorruptHarness(t)

	h.put("foo", "v1")
	h.put("foo", "v2")
	h.put("foo", "v3")
	h.put("foo", "v4")
	h.put("foo", "v5")
	h.closeDB()

	h.recover()
	h.getVal("foo", "v5")
	h.put("foo", "v6")
	h.getVal("foo", "v6")

	h.reopenDB()
	h.getVal("foo", "v6")

	h.close()
}

func TestCorruptDB_SequenceNumberRecoveryTable(t *testing.T) {
	h := newDbCorruptHarness(t)

	h.put("foo", "v1")
	h.put("foo", "v2")
	h.put("foo", "v3")
	h.compactMem()
	h.put("foo", "v4")
	h.put("foo", "v5")
	h.compactMem()
	h.closeDB()

	h.recover()
	h.getVal("foo", "v5")
	h.put("foo", "v6")
	h.getVal("foo", "v6")

	h.reopenDB()
	h.getVal("foo", "v6")

	h.close()
}

func TestCorruptDB_CorruptedManifest(t *testing.T) {
	h := newDbCorruptHarness(t)

	h.put("foo", "hello")
	h.compactMem()
	h.compactRange("", "")
	h.closeDB()
	h.corrupt(storage.TypeManifest, -1, 0, 1000)
	h.openAssert(false)

	h.recover()
	h.getVal("foo", "hello")

	h.close()
}

func TestCorruptDB_CompactionInputError(t *testing.T) {
	h := newDbCorruptHarness(t)

	h.build(10)
	h.compactMem()
	h.closeDB()
	h.corrupt(storage.TypeTable, -1, 100, 1)

	h.openDB()
	h.check(9, 9)

	h.build(10000)
	h.check(10000, 10000)

	h.close()
}

func TestCorruptDB_UnrelatedKeys(t *testing.T) {
	h := newDbCorruptHarness(t)

	h.build(10)
	h.compactMem()
	h.closeDB()
	h.corrupt(storage.TypeTable, -1, 100, 1)

	h.openDB()
	h.put(string(tkey(1000)), string(tval(1000, ctValSize)))
	h.getVal(string(tkey(1000)), string(tval(1000, ctValSize)))
	h.compactMem()
	h.getVal(string(tkey(1000)), string(tval(1000, ctValSize)))

	h.close()
}

func TestCorruptDB_Level0NewerFileHasOlderSeqnum(t *testing.T) {
	h := newDbCorruptHarness(t)

	h.put("a", "v1")
	h.put("b", "v1")
	h.compactMem()
	h.put("a", "v2")
	h.put("b", "v2")
	h.compactMem()
	h.put("a", "v3")
	h.put("b", "v3")
	h.compactMem()
	h.put("c", "v0")
	h.put("d", "v0")
	h.compactMem()
	h.compactRangeAt(1, "", "")
	h.closeDB()

	h.recover()
	h.getVal("a", "v3")
	h.getVal("b", "v3")
	h.getVal("c", "v0")
	h.getVal("d", "v0")

	h.close()
}

func TestCorruptDB_RecoverInvalidSeq_Issue53(t *testing.T) {
	h := newDbCorruptHarness(t)

	h.put("a", "v1")
	h.put("b", "v1")
	h.compactMem()
	h.put("a", "v2")
	h.put("b", "v2")
	h.compactMem()
	h.put("a", "v3")
	h.put("b", "v3")
	h.compactMem()
	h.put("c", "v0")
	h.put("d", "v0")
	h.compactMem()
	h.compactRangeAt(0, "", "")
	h.closeDB()

	h.recover()
	h.getVal("a", "v3")
	h.getVal("b", "v3")
	h.getVal("c", "v0")
	h.getVal("d", "v0")

	h.close()
}

func TestCorruptDB_MissingTableFiles(t *testing.T) {
	h := newDbCorruptHarness(t)

	h.put("a", "v1")
	h.put("b", "v1")
	h.compactMem()
	h.put("c", "v2")
	h.put("d", "v2")
	h.compactMem()
	h.put("e", "v3")
	h.put("f", "v3")
	h.closeDB()

	h.removeOne(storage.TypeTable)
	h.openAssert(false)

	h.close()
}

func TestCorruptDB_RecoverTable(t *testing.T) {
	h := newDbCorruptHarnessWopt(t, &opt.Options{
		WriteBuffer:         112 * opt.KiB,
		CompactionTableSize: 90 * opt.KiB,
		Filter:              filter.NewBloomFilter(10),
	})

	h.build(1000)
	h.compactMem()
	h.compactRangeAt(0, "", "")
	h.compactRangeAt(1, "", "")
	seq := h.db.seq
	h.closeDB()
	h.corrupt(storage.TypeTable, 0, 1000, 1)
	h.corrupt(storage.TypeTable, 3, 10000, 1)
	// Corrupted filter shouldn't affect recovery.
	h.corrupt(storage.TypeTable, 3, 113888, 10)
	h.corrupt(storage.TypeTable, -1, 20000, 1)

	h.recover()
	if h.db.seq != seq {
		t.Errorf("invalid seq, want=%d got=%d", seq, h.db.seq)
	}
	h.check(985, 985)

	h.close()
}
