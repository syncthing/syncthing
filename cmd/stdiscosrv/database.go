// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:generate go run ../../proto/scripts/protofmt.go database.proto
//go:generate protoc -I ../../ -I . --gogofast_out=. database.proto

package main

import (
	"bufio"
	"cmp"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"log"
	"os"
	"path"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/puzpuzpuz/xsync/v3"
	"github.com/syncthing/syncthing/lib/protocol"
)

type clock interface {
	Now() time.Time
}

type defaultClock struct{}

func (defaultClock) Now() time.Time {
	return time.Now()
}

type database interface {
	put(key *protocol.DeviceID, rec DatabaseRecord) error
	merge(key *protocol.DeviceID, addrs []DatabaseAddress, seen int64) error
	get(key *protocol.DeviceID) (DatabaseRecord, error)
}

type inMemoryStore struct {
	m             *xsync.MapOf[protocol.DeviceID, DatabaseRecord]
	dir           string
	flushInterval time.Duration
	s3            *s3Copier
	clock         clock
}

func newInMemoryStore(dir string, flushInterval time.Duration, s3 *s3Copier) *inMemoryStore {
	s := &inMemoryStore{
		m:             xsync.NewMapOf[protocol.DeviceID, DatabaseRecord](),
		dir:           dir,
		flushInterval: flushInterval,
		s3:            s3,
		clock:         defaultClock{},
	}
	nr, err := s.read()
	if os.IsNotExist(err) && s3 != nil {
		// Try to read from AWS
		fd, cerr := os.Create(path.Join(s.dir, "records.db"))
		if cerr != nil {
			log.Println("Error creating database file:", err)
			return s
		}
		if err := s3.downloadLatest(fd); err != nil {
			log.Printf("Error reading database from S3: %v", err)
		}
		_ = fd.Close()
		nr, err = s.read()
	}
	if err != nil {
		log.Println("Error reading database:", err)
	}
	log.Printf("Read %d records from database", nr)
	s.calculateStatistics()
	return s
}

func (s *inMemoryStore) put(key *protocol.DeviceID, rec DatabaseRecord) error {
	t0 := time.Now()
	s.m.Store(*key, rec)
	databaseOperations.WithLabelValues(dbOpPut, dbResSuccess).Inc()
	databaseOperationSeconds.WithLabelValues(dbOpPut).Observe(time.Since(t0).Seconds())
	return nil
}

func (s *inMemoryStore) merge(key *protocol.DeviceID, addrs []DatabaseAddress, seen int64) error {
	t0 := time.Now()

	newRec := DatabaseRecord{
		Addresses: addrs,
		Seen:      seen,
	}

	oldRec, _ := s.m.Load(*key)
	newRec = merge(newRec, oldRec)
	s.m.Store(*key, newRec)

	databaseOperations.WithLabelValues(dbOpMerge, dbResSuccess).Inc()
	databaseOperationSeconds.WithLabelValues(dbOpMerge).Observe(time.Since(t0).Seconds())

	return nil
}

func (s *inMemoryStore) get(key *protocol.DeviceID) (DatabaseRecord, error) {
	t0 := time.Now()
	defer func() {
		databaseOperationSeconds.WithLabelValues(dbOpGet).Observe(time.Since(t0).Seconds())
	}()

	rec, ok := s.m.Load(*key)
	if !ok {
		databaseOperations.WithLabelValues(dbOpGet, dbResNotFound).Inc()
		return DatabaseRecord{}, nil
	}

	rec.Addresses = expire(rec.Addresses, s.clock.Now())
	databaseOperations.WithLabelValues(dbOpGet, dbResSuccess).Inc()
	return rec, nil
}

func (s *inMemoryStore) Serve(ctx context.Context) error {
	t := time.NewTimer(s.flushInterval)
	defer t.Stop()

	if s.flushInterval <= 0 {
		t.Stop()
	}

loop:
	for {
		select {
		case <-t.C:
			log.Println("Calculating statistics")
			s.calculateStatistics()
			log.Println("Flushing database")
			if err := s.write(); err != nil {
				log.Println("Error writing database:", err)
			}
			log.Println("Finished flushing database")
			t.Reset(s.flushInterval)

		case <-ctx.Done():
			// We're done.
			break loop
		}
	}

	return s.write()
}

func (s *inMemoryStore) calculateStatistics() {
	now := s.clock.Now()
	cutoff24h := now.Add(-24 * time.Hour).UnixNano()
	cutoff1w := now.Add(-7 * 24 * time.Hour).UnixNano()
	current, currentIPv4, currentIPv6, currentIPv6GUA, last24h, last1w := 0, 0, 0, 0, 0, 0

	n := 0
	s.m.Range(func(key protocol.DeviceID, rec DatabaseRecord) bool {
		if n%1000 == 0 {
			runtime.Gosched()
		}
		n++

		addresses := expire(rec.Addresses, now)
		if len(addresses) == 0 {
			rec.Addresses = nil
			s.m.Store(key, rec)
		} else if len(addresses) != len(rec.Addresses) {
			rec.Addresses = addresses
			s.m.Store(key, rec)
		}

		switch {
		case len(rec.Addresses) > 0:
			current++
			seenIPv4, seenIPv6, seenIPv6GUA := false, false, false
			for _, addr := range rec.Addresses {
				// We do fast and loose matching on strings here instead of
				// parsing the address and the IP and doing "proper" checks,
				// to keep things fast and generate less garbage.
				if strings.Contains(addr.Address, "[") {
					seenIPv6 = true
					if strings.Contains(addr.Address, "[2") {
						seenIPv6GUA = true
					}
				} else {
					seenIPv4 = true
				}
				if seenIPv4 && seenIPv6 && seenIPv6GUA {
					break
				}
			}
			if seenIPv4 {
				currentIPv4++
			}
			if seenIPv6 {
				currentIPv6++
			}
			if seenIPv6GUA {
				currentIPv6GUA++
			}
		case rec.Seen > cutoff24h:
			last24h++
		case rec.Seen > cutoff1w:
			last1w++
		default:
			// drop the record if it's older than a week
			s.m.Delete(key)
		}
		return true
	})

	databaseKeys.WithLabelValues("current").Set(float64(current))
	databaseKeys.WithLabelValues("currentIPv4").Set(float64(currentIPv4))
	databaseKeys.WithLabelValues("currentIPv6").Set(float64(currentIPv6))
	databaseKeys.WithLabelValues("currentIPv6GUA").Set(float64(currentIPv6GUA))
	databaseKeys.WithLabelValues("last24h").Set(float64(last24h))
	databaseKeys.WithLabelValues("last1w").Set(float64(last1w))
	databaseStatisticsSeconds.Set(time.Since(now).Seconds())
}

func (s *inMemoryStore) write() (err error) {
	t0 := time.Now()
	defer func() {
		if err == nil {
			databaseWriteSeconds.Set(time.Since(t0).Seconds())
			databaseLastWritten.Set(float64(t0.Unix()))
		}
	}()

	dbf := path.Join(s.dir, "records.db")
	fd, err := os.Create(dbf + ".tmp")
	if err != nil {
		return err
	}
	bw := bufio.NewWriter(fd)

	var buf []byte
	var rangeErr error
	now := s.clock.Now()
	cutoff1w := now.Add(-7 * 24 * time.Hour).UnixNano()
	n := 0
	s.m.Range(func(key protocol.DeviceID, value DatabaseRecord) bool {
		if n%1000 == 0 {
			runtime.Gosched()
		}
		n++

		if value.Seen < cutoff1w {
			// drop the record if it's older than a week
			return true
		}
		rec := ReplicationRecord{
			Key:       key[:],
			Addresses: value.Addresses,
			Seen:      value.Seen,
		}
		s := rec.Size()
		if s+4 > len(buf) {
			buf = make([]byte, s+4)
		}
		n, err := rec.MarshalTo(buf[4:])
		if err != nil {
			rangeErr = err
			return false
		}
		binary.BigEndian.PutUint32(buf, uint32(n))
		if _, err := bw.Write(buf[:n+4]); err != nil {
			rangeErr = err
			return false
		}
		return true
	})
	if rangeErr != nil {
		_ = fd.Close()
		return rangeErr
	}

	if err := bw.Flush(); err != nil {
		_ = fd.Close
		return err
	}
	if err := fd.Close(); err != nil {
		return err
	}
	if err := os.Rename(dbf+".tmp", dbf); err != nil {
		return err
	}

	// Upload to S3
	if s.s3 != nil {
		fd, err = os.Open(dbf)
		if err != nil {
			log.Printf("Error uploading database to S3: %v", err)
			return nil
		}
		defer fd.Close()
		if err := s.s3.upload(fd); err != nil {
			log.Printf("Error uploading database to S3: %v", err)
		}
		log.Println("Finished uploading database")
	}

	return nil
}

func (s *inMemoryStore) read() (int, error) {
	fd, err := os.Open(path.Join(s.dir, "records.db"))
	if err != nil {
		return 0, err
	}
	defer fd.Close()

	br := bufio.NewReader(fd)
	var buf []byte
	nr := 0
	for {
		var n uint32
		if err := binary.Read(br, binary.BigEndian, &n); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nr, err
		}
		if int(n) > len(buf) {
			buf = make([]byte, n)
		}
		if _, err := io.ReadFull(br, buf[:n]); err != nil {
			return nr, err
		}
		rec := ReplicationRecord{}
		if err := rec.Unmarshal(buf[:n]); err != nil {
			return nr, err
		}
		key, err := protocol.DeviceIDFromBytes(rec.Key)
		if err != nil {
			key, err = protocol.DeviceIDFromString(string(rec.Key))
		}
		if err != nil {
			log.Println("Bad device ID:", err)
			continue
		}

		slices.SortFunc(rec.Addresses, DatabaseAddress.Cmp)
		s.m.Store(key, DatabaseRecord{
			Addresses: expire(rec.Addresses, s.clock.Now()),
			Seen:      rec.Seen,
		})
		nr++
	}
	return nr, nil
}

// merge returns the merged result of the two database records a and b. The
// result is the union of the two address sets, with the newer expiry time
// chosen for any duplicates.
func merge(a, b DatabaseRecord) DatabaseRecord {
	// Both lists must be sorted for this to work.

	res := DatabaseRecord{
		Addresses: make([]DatabaseAddress, 0, max(len(a.Addresses), len(b.Addresses))),
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
// destroyed. Internal order is preserved.
func expire(addrs []DatabaseAddress, now time.Time) []DatabaseAddress {
	cutoff := now.UnixNano()
	naddrs := addrs[:0]
	for i := range addrs {
		if addrs[i].Expires >= cutoff {
			naddrs = append(naddrs, addrs[i])
		}
	}
	if len(naddrs) == 0 {
		return nil
	}
	return naddrs
}

func (d DatabaseAddress) Cmp(other DatabaseAddress) (n int) {
	if c := cmp.Compare(d.Address, other.Address); c != 0 {
		return c
	}
	return cmp.Compare(d.Expires, other.Expires)
}
