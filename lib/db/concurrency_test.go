// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// +build ignore // this  is a really tedious test for an old issue

package db_test

import (
	"crypto/rand"
	"log"
	"os"
	"testing"
	"time"

	"github.com/syncthing/syncthing/lib/sync"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"
)

var keys [][]byte

func init() {
	for i := 0; i < nItems; i++ {
		keys = append(keys, randomData(1))
	}
}

const nItems = 10000

func randomData(prefix byte) []byte {
	data := make([]byte, 1+32+64+32)
	_, err := rand.Reader.Read(data)
	if err != nil {
		panic(err)
	}
	return append([]byte{prefix}, data...)
}

func setItems(db *leveldb.DB) error {
	batch := new(leveldb.Batch)
	for _, k1 := range keys {
		k2 := randomData(2)
		// k2 -> data
		batch.Put(k2, randomData(42))
		// k1 -> k2
		batch.Put(k1, k2)
	}
	if testing.Verbose() {
		log.Printf("batch write (set) %p", batch)
	}
	return db.Write(batch, nil)
}

func clearItems(db *leveldb.DB) error {
	snap, err := db.GetSnapshot()
	if err != nil {
		return err
	}
	defer snap.Release()

	// Iterate over k2

	it := snap.NewIterator(util.BytesPrefix([]byte{1}), nil)
	defer it.Release()

	batch := new(leveldb.Batch)
	for it.Next() {
		k1 := it.Key()
		k2 := it.Value()

		// k2 should exist
		_, err := snap.Get(k2, nil)
		if err != nil {
			return err
		}

		// Delete the k1 => k2 mapping first
		batch.Delete(k1)
		// Then the k2 => data mapping
		batch.Delete(k2)
	}
	if testing.Verbose() {
		log.Printf("batch write (clear) %p", batch)
	}
	return db.Write(batch, nil)
}

func scanItems(db *leveldb.DB) error {
	snap, err := db.GetSnapshot()
	if testing.Verbose() {
		log.Printf("snap create %p", snap)
	}
	if err != nil {
		return err
	}
	defer func() {
		if testing.Verbose() {
			log.Printf("snap release %p", snap)
		}
		snap.Release()
	}()

	// Iterate from the start of k2 space to the end
	it := snap.NewIterator(util.BytesPrefix([]byte{1}), nil)
	defer it.Release()

	i := 0
	for it.Next() {
		// k2 => k1 => data
		k1 := it.Key()
		k2 := it.Value()
		_, err := snap.Get(k2, nil)
		if err != nil {
			log.Printf("k1: %x", k1)
			log.Printf("k2: %x (missing)", k2)
			return err
		}
		i++
	}
	if testing.Verbose() {
		log.Println("scanned", i)
	}
	return nil
}

func TestConcurrentSetClear(t *testing.T) {
	if testing.Short() {
		return
	}

	dur := 30 * time.Second
	t0 := time.Now()
	wg := sync.NewWaitGroup()

	os.RemoveAll("testdata/concurrent-set-clear.db")
	db, err := leveldb.OpenFile("testdata/concurrent-set-clear.db", &opt.Options{OpenFilesCacheCapacity: 10})
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll("testdata/concurrent-set-clear.db")

	errChan := make(chan error, 3)

	wg.Add(1)
	go func() {
		defer wg.Done()
		for time.Since(t0) < dur {
			if err := setItems(db); err != nil {
				errChan <- err
				return
			}
			if err := clearItems(db); err != nil {
				errChan <- err
				return
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for time.Since(t0) < dur {
			if err := scanItems(db); err != nil {
				errChan <- err
				return
			}
		}
	}()

	go func() {
		wg.Wait()
		errChan <- nil
	}()

	err = <-errChan
	if err != nil {
		t.Error(err)
	}
	db.Close()
}

func TestConcurrentSetOnly(t *testing.T) {
	if testing.Short() {
		return
	}

	dur := 30 * time.Second
	t0 := time.Now()
	wg := sync.NewWaitGroup()

	os.RemoveAll("testdata/concurrent-set-only.db")
	db, err := leveldb.OpenFile("testdata/concurrent-set-only.db", &opt.Options{OpenFilesCacheCapacity: 10})
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll("testdata/concurrent-set-only.db")

	errChan := make(chan error, 3)

	wg.Add(1)
	go func() {
		defer wg.Done()
		for time.Since(t0) < dur {
			if err := setItems(db); err != nil {
				errChan <- err
				return
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for time.Since(t0) < dur {
			if err := scanItems(db); err != nil {
				errChan <- err
				return
			}
		}
	}()

	go func() {
		wg.Wait()
		errChan <- nil
	}()

	err = <-errChan
	if err != nil {
		t.Error(err)
	}
}
