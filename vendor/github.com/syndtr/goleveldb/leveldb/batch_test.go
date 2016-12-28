// Copyright (c) 2012, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package leveldb

import (
	"bytes"
	"fmt"
	"testing"
	"testing/quick"

	"github.com/syndtr/goleveldb/leveldb/testutil"
)

func TestBatchHeader(t *testing.T) {
	f := func(seq uint64, length uint32) bool {
		encoded := encodeBatchHeader(nil, seq, int(length))
		decSeq, decLength, err := decodeBatchHeader(encoded)
		return err == nil && decSeq == seq && decLength == int(length)
	}
	config := &quick.Config{
		Rand: testutil.NewRand(),
	}
	if err := quick.Check(f, config); err != nil {
		t.Error(err)
	}
}

type batchKV struct {
	kt   keyType
	k, v []byte
}

func TestBatch(t *testing.T) {
	var (
		kvs         []batchKV
		internalLen int
	)
	batch := new(Batch)
	rbatch := new(Batch)
	abatch := new(Batch)
	testBatch := func(i int, kt keyType, k, v []byte) error {
		kv := kvs[i]
		if kv.kt != kt {
			return fmt.Errorf("invalid key type, index=%d: %d vs %d", i, kv.kt, kt)
		}
		if !bytes.Equal(kv.k, k) {
			return fmt.Errorf("invalid key, index=%d", i)
		}
		if !bytes.Equal(kv.v, v) {
			return fmt.Errorf("invalid value, index=%d", i)
		}
		return nil
	}
	f := func(ktr uint8, k, v []byte) bool {
		kt := keyType(ktr % 2)
		if kt == keyTypeVal {
			batch.Put(k, v)
			rbatch.Put(k, v)
			kvs = append(kvs, batchKV{kt: kt, k: k, v: v})
			internalLen += len(k) + len(v) + 8
		} else {
			batch.Delete(k)
			rbatch.Delete(k)
			kvs = append(kvs, batchKV{kt: kt, k: k})
			internalLen += len(k) + 8
		}
		if batch.Len() != len(kvs) {
			t.Logf("batch.Len: %d vs %d", len(kvs), batch.Len())
			return false
		}
		if batch.internalLen != internalLen {
			t.Logf("abatch.internalLen: %d vs %d", internalLen, batch.internalLen)
			return false
		}
		if len(kvs)%1000 == 0 {
			if err := batch.replayInternal(testBatch); err != nil {
				t.Logf("batch.replayInternal: %v", err)
				return false
			}

			abatch.append(rbatch)
			rbatch.Reset()
			if abatch.Len() != len(kvs) {
				t.Logf("abatch.Len: %d vs %d", len(kvs), abatch.Len())
				return false
			}
			if abatch.internalLen != internalLen {
				t.Logf("abatch.internalLen: %d vs %d", internalLen, abatch.internalLen)
				return false
			}
			if err := abatch.replayInternal(testBatch); err != nil {
				t.Logf("abatch.replayInternal: %v", err)
				return false
			}

			nbatch := new(Batch)
			if err := nbatch.Load(batch.Dump()); err != nil {
				t.Logf("nbatch.Load: %v", err)
				return false
			}
			if nbatch.Len() != len(kvs) {
				t.Logf("nbatch.Len: %d vs %d", len(kvs), nbatch.Len())
				return false
			}
			if nbatch.internalLen != internalLen {
				t.Logf("nbatch.internalLen: %d vs %d", internalLen, nbatch.internalLen)
				return false
			}
			if err := nbatch.replayInternal(testBatch); err != nil {
				t.Logf("nbatch.replayInternal: %v", err)
				return false
			}
		}
		if len(kvs)%10000 == 0 {
			nbatch := new(Batch)
			if err := batch.Replay(nbatch); err != nil {
				t.Logf("batch.Replay: %v", err)
				return false
			}
			if nbatch.Len() != len(kvs) {
				t.Logf("nbatch.Len: %d vs %d", len(kvs), nbatch.Len())
				return false
			}
			if nbatch.internalLen != internalLen {
				t.Logf("nbatch.internalLen: %d vs %d", internalLen, nbatch.internalLen)
				return false
			}
			if err := nbatch.replayInternal(testBatch); err != nil {
				t.Logf("nbatch.replayInternal: %v", err)
				return false
			}
		}
		return true
	}
	config := &quick.Config{
		MaxCount: 40000,
		Rand:     testutil.NewRand(),
	}
	if err := quick.Check(f, config); err != nil {
		t.Error(err)
	}
	t.Logf("length=%d internalLen=%d", len(kvs), internalLen)
}
