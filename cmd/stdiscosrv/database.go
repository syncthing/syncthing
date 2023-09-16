// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:generate go run ../../proto/scripts/protofmt.go database.proto
//go:generate protoc -I ../../ -I . --gogofast_out=. database.proto

package main

import (
	"context"
	"log"
	"sort"
	"time"

	"github.com/syncthing/syncthing/lib/sliceutil"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/storage"
	"github.com/syndtr/goleveldb/leveldb/util"
)

type clock interface {
	Now() time.Time
}

type defaultClock struct{}

func (defaultClock) Now() time.Time {
	return time.Now()
}

type database interface {
	put(key string, rec DatabaseRecord) error
	merge(key string, addrs []DatabaseAddress, seen int64) error
	get(key string) (DatabaseRecord, error)
}

type levelDBStore struct {
	db         *leveldb.DB
	inbox      chan func()
	clock      clock
	marshalBuf []byte
}

func newLevelDBStore(dir string) (*levelDBStore, error) {
	db, err := leveldb.OpenFile(dir, levelDBOptions)
	if err != nil {
		return nil, err
	}
	return &levelDBStore{
		db:    db,
		inbox: make(chan func(), 16),
		clock: defaultClock{},
	}, nil
}

func newMemoryLevelDBStore() (*levelDBStore, error) {
	db, err := leveldb.Open(storage.NewMemStorage(), nil)
	if err != nil {
		return nil, err
	}
	return &levelDBStore{
		db:    db,
		inbox: make(chan func(), 16),
		clock: defaultClock{},
	}, nil
}

func (s *levelDBStore) put(key string, rec DatabaseRecord) error {
	t0 := time.Now()
	defer func() {
		databaseOperationSeconds.WithLabelValues(dbOpPut).Observe(time.Since(t0).Seconds())
	}()

	rc := make(chan error)

	s.inbox <- func() {
		size := rec.Size()
		if len(s.marshalBuf) < size {
			s.marshalBuf = make([]byte, size)
		}
		n, _ := rec.MarshalTo(s.marshalBuf)
		rc <- s.db.Put([]byte(key), s.marshalBuf[:n], nil)
	}

	err := <-rc
	if err != nil {
		databaseOperations.WithLabelValues(dbOpPut, dbResError).Inc()
	} else {
		databaseOperations.WithLabelValues(dbOpPut, dbResSuccess).Inc()
	}

	return err
}

func (s *levelDBStore) merge(key string, addrs []DatabaseAddress, seen int64) error {
	t0 := time.Now()
	defer func() {
		databaseOperationSeconds.WithLabelValues(dbOpMerge).Observe(time.Since(t0).Seconds())
	}()

	rc := make(chan error)
	newRec := DatabaseRecord{
		Addresses: addrs,
		Seen:      seen,
	}

	s.inbox <- func() {
		// grab the existing record
		oldRec, err := s.get(key)
		if err != nil {
			// "not found" is not an error from get, so this is serious
			// stuff only
			rc <- err
			return
		}
		newRec = merge(newRec, oldRec)

		// We replicate s.put() functionality here ourselves instead of
		// calling it because we want to serialize our get above together
		// with the put in the same function.
		size := newRec.Size()
		if len(s.marshalBuf) < size {
			s.marshalBuf = make([]byte, size)
		}
		n, _ := newRec.MarshalTo(s.marshalBuf)
		rc <- s.db.Put([]byte(key), s.marshalBuf[:n], nil)
	}

	err := <-rc
	if err != nil {
		databaseOperations.WithLabelValues(dbOpMerge, dbResError).Inc()
	} else {
		databaseOperations.WithLabelValues(dbOpMerge, dbResSuccess).Inc()
	}

	return err
}

func (s *levelDBStore) get(key string) (DatabaseRecord, error) {
	t0 := time.Now()
	defer func() {
		databaseOperationSeconds.WithLabelValues(dbOpGet).Observe(time.Since(t0).Seconds())
	}()

	keyBs := []byte(key)
	val, err := s.db.Get(keyBs, nil)
	if err == leveldb.ErrNotFound {
		databaseOperations.WithLabelValues(dbOpGet, dbResNotFound).Inc()
		return DatabaseRecord{}, nil
	}
	if err != nil {
		databaseOperations.WithLabelValues(dbOpGet, dbResError).Inc()
		return DatabaseRecord{}, err
	}

	var rec DatabaseRecord

	if err := rec.Unmarshal(val); err != nil {
		databaseOperations.WithLabelValues(dbOpGet, dbResUnmarshalError).Inc()
		return DatabaseRecord{}, nil
	}

	rec.Addresses = expire(rec.Addresses, s.clock.Now().UnixNano())
	databaseOperations.WithLabelValues(dbOpGet, dbResSuccess).Inc()
	return rec, nil
}

func (s *levelDBStore) Serve(ctx context.Context) error {
	t := time.NewTimer(0)
	defer t.Stop()
	defer s.db.Close()

	// Start the statistics serve routine. It will exit with us when
	// statisticsTrigger is closed.
	statisticsTrigger := make(chan struct{})
	statisticsDone := make(chan struct{})
	go s.statisticsServe(statisticsTrigger, statisticsDone)

loop:
	for {
		select {
		case fn := <-s.inbox:
			// Run function in serialized order.
			fn()

		case <-t.C:
			// Trigger the statistics routine to do its thing in the
			// background.
			statisticsTrigger <- struct{}{}

		case <-statisticsDone:
			// The statistics routine is done with one iteratation, schedule
			// the next.
			t.Reset(databaseStatisticsInterval)

		case <-ctx.Done():
			// We're done.
			close(statisticsTrigger)
			break loop
		}
	}

	// Also wait for statisticsServe to return
	<-statisticsDone

	return nil
}

func (s *levelDBStore) statisticsServe(trigger <-chan struct{}, done chan<- struct{}) {
	defer close(done)

	for range trigger {
		t0 := time.Now()
		nowNanos := t0.UnixNano()
		cutoff24h := t0.Add(-24 * time.Hour).UnixNano()
		cutoff1w := t0.Add(-7 * 24 * time.Hour).UnixNano()
		cutoff2Mon := t0.Add(-60 * 24 * time.Hour).UnixNano()
		current, last24h, last1w, inactive, errors := 0, 0, 0, 0, 0

		iter := s.db.NewIterator(&util.Range{}, nil)
		for iter.Next() {
			// Attempt to unmarshal the record and count the
			// failure if there's something wrong with it.
			var rec DatabaseRecord
			if err := rec.Unmarshal(iter.Value()); err != nil {
				errors++
				continue
			}

			// If there are addresses that have not expired it's a current
			// record, otherwise account it based on when it was last seen
			// (last 24 hours or last week) or finally as inactice.
			switch {
			case len(expire(rec.Addresses, nowNanos)) > 0:
				current++
			case rec.Seen > cutoff24h:
				last24h++
			case rec.Seen > cutoff1w:
				last1w++
			case rec.Seen > cutoff2Mon:
				inactive++
			case rec.Missed < cutoff2Mon:
				// It hasn't been seen lately and we haven't recorded
				// someone asking for this device in a long time either;
				// delete the record.
				if err := s.db.Delete(iter.Key(), nil); err != nil {
					databaseOperations.WithLabelValues(dbOpDelete, dbResError).Inc()
				} else {
					databaseOperations.WithLabelValues(dbOpDelete, dbResSuccess).Inc()
				}
			default:
				inactive++
			}
		}

		iter.Release()

		databaseKeys.WithLabelValues("current").Set(float64(current))
		databaseKeys.WithLabelValues("last24h").Set(float64(last24h))
		databaseKeys.WithLabelValues("last1w").Set(float64(last1w))
		databaseKeys.WithLabelValues("inactive").Set(float64(inactive))
		databaseKeys.WithLabelValues("error").Set(float64(errors))
		databaseStatisticsSeconds.Set(time.Since(t0).Seconds())

		// Signal that we are done and can be scheduled again.
		done <- struct{}{}
	}
}

// merge returns the merged result of the two database records a and b. The
// result is the union of the two address sets, with the newer expiry time
// chosen for any duplicates.
func merge(a, b DatabaseRecord) DatabaseRecord {
	// Both lists must be sorted for this to work.
	if !sort.IsSorted(databaseAddressOrder(a.Addresses)) {
		log.Println("Warning: bug: addresses not correctly sorted in merge")
		a.Addresses = sortedAddressCopy(a.Addresses)
	}
	if !sort.IsSorted(databaseAddressOrder(b.Addresses)) {
		// no warning because this is the side we read from disk and it may
		// legitimately predate correct sorting.
		b.Addresses = sortedAddressCopy(b.Addresses)
	}

	res := DatabaseRecord{
		Addresses: make([]DatabaseAddress, 0, len(a.Addresses)+len(b.Addresses)),
		Seen:      a.Seen,
	}
	if b.Seen > a.Seen {
		res.Seen = b.Seen
	}

	aIdx := 0
	bIdx := 0
	aAddrs := a.Addresses
	bAddrs := b.Addresses
loop:
	for {
		switch {
		case aIdx == len(aAddrs) && bIdx == len(bAddrs):
			// both lists are exhausted, we are done
			break loop

		case aIdx == len(aAddrs):
			// a is exhausted, pick from b and continue
			res.Addresses = append(res.Addresses, bAddrs[bIdx])
			bIdx++
			continue

		case bIdx == len(bAddrs):
			// b is exhausted, pick from a and continue
			res.Addresses = append(res.Addresses, aAddrs[aIdx])
			aIdx++
			continue
		}

		// We have values left on both sides.
		aVal := aAddrs[aIdx]
		bVal := bAddrs[bIdx]

		switch {
		case aVal.Address == bVal.Address:
			// update for same address, pick newer
			if aVal.Expires > bVal.Expires {
				res.Addresses = append(res.Addresses, aVal)
			} else {
				res.Addresses = append(res.Addresses, bVal)
			}
			aIdx++
			bIdx++

		case aVal.Address < bVal.Address:
			// a is smallest, pick it and continue
			res.Addresses = append(res.Addresses, aVal)
			aIdx++

		default:
			// b is smallest, pick it and continue
			res.Addresses = append(res.Addresses, bVal)
			bIdx++
		}
	}
	return res
}

// expire returns the list of addresses after removing expired entries.
// Expiration happen in place, so the slice given as the parameter is
// destroyed. Internal order is not preserved.
func expire(addrs []DatabaseAddress, now int64) []DatabaseAddress {
	i := 0
	for i < len(addrs) {
		if addrs[i].Expires < now {
			addrs = sliceutil.RemoveAndZero(addrs, i)
			continue
		}
		i++
	}
	return addrs
}

func sortedAddressCopy(addrs []DatabaseAddress) []DatabaseAddress {
	sorted := make([]DatabaseAddress, len(addrs))
	copy(sorted, addrs)
	sort.Sort(databaseAddressOrder(sorted))
	return sorted
}

type databaseAddressOrder []DatabaseAddress

func (s databaseAddressOrder) Less(a, b int) bool {
	return s[a].Address < s[b].Address
}

func (s databaseAddressOrder) Swap(a, b int) {
	s[a], s[b] = s[b], s[a]
}

func (s databaseAddressOrder) Len() int {
	return len(s)
}
