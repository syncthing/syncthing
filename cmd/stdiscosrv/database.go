// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"bufio"
	"cmp"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"log/slog"
	"os"
	"path"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/puzpuzpuz/xsync/v3"
	"google.golang.org/protobuf/proto"

	"github.com/syncthing/syncthing/internal/blob"
	"github.com/syncthing/syncthing/internal/gen/discosrv"
	"github.com/syncthing/syncthing/internal/protoutil"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/rand"
)

type clock interface {
	Now() time.Time
}

type defaultClock struct{}

func (defaultClock) Now() time.Time {
	return time.Now()
}

type database interface {
	put(key *protocol.DeviceID, rec *discosrv.DatabaseRecord) error
	merge(key *protocol.DeviceID, addrs []*discosrv.DatabaseAddress, seen int64) error
	get(key *protocol.DeviceID) (*discosrv.DatabaseRecord, error)
}

type inMemoryStore struct {
	m             *xsync.MapOf[protocol.DeviceID, *discosrv.DatabaseRecord]
	dir           string
	flushInterval time.Duration
	blobs         blob.Store
	objKey        string
	clock         clock
}

func newInMemoryStore(dir string, flushInterval time.Duration, blobs blob.Store) *inMemoryStore {
	hn, err := os.Hostname()
	if err != nil {
		hn = rand.String(8)
	}
	s := &inMemoryStore{
		m:             xsync.NewMapOf[protocol.DeviceID, *discosrv.DatabaseRecord](),
		dir:           dir,
		flushInterval: flushInterval,
		blobs:         blobs,
		objKey:        hn + ".db",
		clock:         defaultClock{},
	}
	nr, err := s.read()
	if os.IsNotExist(err) && blobs != nil {
		// Try to read from blob storage
		latestKey, cerr := blobs.LatestKey(context.Background())
		if cerr != nil {
			slog.Error("Failed to find database in blob storage", "error", cerr)
			return s
		}
		fd, cerr := os.Create(path.Join(s.dir, "records.db"))
		if cerr != nil {
			slog.Error("Failed to create database file", "error", cerr)
			return s
		}
		if cerr := blobs.Download(context.Background(), latestKey, fd); cerr != nil {
			slog.Error("Failed to download database from blob storage", "error", cerr)
		}
		_ = fd.Close()
		nr, err = s.read()
	}
	if err != nil {
		slog.Error("Failed to read database", "error", err)
	}
	slog.Info("Loaded database", "records", nr)
	s.expireAndCalculateStatistics()
	return s
}

func (s *inMemoryStore) put(key *protocol.DeviceID, rec *discosrv.DatabaseRecord) error {
	t0 := time.Now()
	s.m.Store(*key, rec)
	databaseOperations.WithLabelValues(dbOpPut, dbResSuccess).Inc()
	databaseOperationSeconds.WithLabelValues(dbOpPut).Observe(time.Since(t0).Seconds())
	return nil
}

func (s *inMemoryStore) merge(key *protocol.DeviceID, addrs []*discosrv.DatabaseAddress, seen int64) error {
	t0 := time.Now()

	newRec := &discosrv.DatabaseRecord{
		Addresses: addrs,
		Seen:      seen,
	}

	if oldRec, ok := s.m.Load(*key); ok {
		newRec = merge(newRec, oldRec)
	}
	s.m.Store(*key, newRec)

	databaseOperations.WithLabelValues(dbOpMerge, dbResSuccess).Inc()
	databaseOperationSeconds.WithLabelValues(dbOpMerge).Observe(time.Since(t0).Seconds())

	return nil
}

func (s *inMemoryStore) get(key *protocol.DeviceID) (*discosrv.DatabaseRecord, error) {
	t0 := time.Now()
	defer func() {
		databaseOperationSeconds.WithLabelValues(dbOpGet).Observe(time.Since(t0).Seconds())
	}()

	rec, ok := s.m.Load(*key)
	if !ok {
		databaseOperations.WithLabelValues(dbOpGet, dbResNotFound).Inc()
		return &discosrv.DatabaseRecord{}, nil
	}

	naddresses, changed := expire(rec.Addresses, s.clock.Now())
	if changed {
		rec = &discosrv.DatabaseRecord{
			Addresses: naddresses,
			Seen:      rec.Seen,
		}
	}
	databaseOperations.WithLabelValues(dbOpGet, dbResSuccess).Inc()
	return rec, nil
}

func (s *inMemoryStore) Serve(ctx context.Context) error {
	if s.flushInterval <= 0 {
		<-ctx.Done()
		return nil
	}

	t := time.NewTimer(s.flushInterval)
	defer t.Stop()

loop:
	for {
		select {
		case <-t.C:
			slog.InfoContext(ctx, "Calculating statistics")
			s.expireAndCalculateStatistics()
			slog.InfoContext(ctx, "Flushing database")
			if err := s.write(); err != nil {
				slog.ErrorContext(ctx, "Failed to write database", "error", err)
			}
			slog.InfoContext(ctx, "Finished flushing database")
			t.Reset(s.flushInterval)

		case <-ctx.Done():
			// We're done.
			break loop
		}
	}

	return s.write()
}

func (s *inMemoryStore) expireAndCalculateStatistics() {
	now := s.clock.Now()
	cutoff24h := now.Add(-24 * time.Hour).UnixNano()
	cutoff1w := now.Add(-7 * 24 * time.Hour).UnixNano()
	current, currentIPv4, currentIPv6, currentIPv6GUA, last24h, last1w := 0, 0, 0, 0, 0, 0

	n := 0
	s.m.Range(func(key protocol.DeviceID, rec *discosrv.DatabaseRecord) bool {
		if n%1000 == 0 {
			runtime.Gosched()
		}
		n++

		addresses, changed := expire(rec.Addresses, now)
		if changed {
			rec = &discosrv.DatabaseRecord{
				Addresses: addresses,
				Seen:      rec.Seen,
			}
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
	bw := bufio.NewWriterSize(fd, 1<<20)

	var buf []byte
	var rangeErr error
	now := s.clock.Now()
	cutoff1w := now.Add(-7 * 24 * time.Hour).UnixNano()
	n := 0
	s.m.Range(func(key protocol.DeviceID, value *discosrv.DatabaseRecord) bool {
		if n%1000 == 0 {
			runtime.Gosched()
		}
		n++

		if value.Seen < cutoff1w {
			// drop the record if it's older than a week
			return true
		}
		rec := &discosrv.ReplicationRecord{
			Key:       key[:],
			Addresses: value.Addresses,
			Seen:      value.Seen,
		}
		s := proto.Size(rec)
		if s+4 > len(buf) {
			buf = make([]byte, s+4)
		}
		n, err := protoutil.MarshalTo(buf[4:], rec)
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

	if info, err := os.Lstat(dbf); err == nil {
		slog.Info("Saved database", "name", dbf, "size", info.Size(), "modtime", info.ModTime())
	} else {
		slog.Warn("Failed to stat database after save", "error", err)
	}

	// Upload to blob storage
	if s.blobs != nil {
		fd, err = os.Open(dbf)
		if err != nil {
			slog.Error("Failed to upload database to blob storage", "error", err)
			return nil
		}
		defer fd.Close()
		if err := s.blobs.Upload(context.Background(), s.objKey, fd); err != nil {
			slog.Error("Failed to upload database to blob storage", "error", err)
		}
		slog.Info("Finished uploading database")
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
		rec := &discosrv.ReplicationRecord{}
		if err := proto.Unmarshal(buf[:n], rec); err != nil {
			return nr, err
		}
		key, err := protocol.DeviceIDFromBytes(rec.Key)
		if err != nil {
			key, err = protocol.DeviceIDFromString(string(rec.Key))
		}
		if err != nil {
			slog.Error("Got bad device ID while reading database", "error", err)
			continue
		}

		slices.SortFunc(rec.Addresses, Cmp)
		rec.Addresses, _ = expire(slices.CompactFunc(rec.Addresses, Equal), s.clock.Now())
		s.m.Store(key, &discosrv.DatabaseRecord{
			Addresses: rec.Addresses,
			Seen:      rec.Seen,
		})
		nr++
	}
	return nr, nil
}

// merge returns the merged result of the two database records a and b. The
// result is the union of the two address sets, with the newer expiry time
// chosen for any duplicates. The address list in a is overwritten and
// reused for the result; b is not modified.
func merge(a, b *discosrv.DatabaseRecord) *discosrv.DatabaseRecord {
	// Both lists must be sorted for this to work.

	a.Seen = max(a.Seen, b.Seen)

	aIdx := 0
	bIdx := 0
	for aIdx < len(a.Addresses) && bIdx < len(b.Addresses) {
		switch cmp.Compare(a.Addresses[aIdx].Address, b.Addresses[bIdx].Address) {
		case 0:
			// a == b, choose the newer expiry time
			a.Addresses[aIdx].Expires = max(a.Addresses[aIdx].Expires, b.Addresses[bIdx].Expires)
			aIdx++
			bIdx++
		case -1:
			// a < b, keep a and move on
			aIdx++
		case 1:
			// a > b, insert b before a
			a.Addresses = append(a.Addresses[:aIdx], append([]*discosrv.DatabaseAddress{b.Addresses[bIdx]}, a.Addresses[aIdx:]...)...)
			bIdx++
		}
	}
	if bIdx < len(b.Addresses) {
		a.Addresses = append(a.Addresses, b.Addresses[bIdx:]...)
	}

	return a
}

// expire returns the list of addresses after removing expired entries. A
// new slice is allocated if any changes are required, and the changed
// boolean indicates whether that happened or not.
func expire(addrs []*discosrv.DatabaseAddress, now time.Time) (result []*discosrv.DatabaseAddress, changed bool) {
	cutoff := now.UnixNano()
	remains := 0
	for _, a := range addrs {
		if a.Expires < cutoff {
			changed = true
		} else {
			remains++
		}
	}
	if !changed {
		return addrs, false
	}
	if remains == 0 {
		return nil, true
	}

	naddrs := make([]*discosrv.DatabaseAddress, 0, remains)
	for _, a := range addrs {
		if a.Expires >= cutoff {
			naddrs = append(naddrs, a)
		}
	}
	return naddrs, true
}

func Cmp(d, other *discosrv.DatabaseAddress) (n int) {
	if c := cmp.Compare(d.Address, other.Address); c != 0 {
		return c
	}
	return cmp.Compare(d.Expires, other.Expires)
}

func Equal(d, other *discosrv.DatabaseAddress) bool {
	return d.Address == other.Address
}
