// Copyright (C) 2026 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package dht

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"io"
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/syncthing/syncthing/lib/protocol"
	"golang.org/x/crypto/hkdf"
)

// deriveRecordKey derives the daily discovery lookup key for id.
func deriveRecordKey(id protocol.DeviceID, date time.Time) [32]byte {
	return deriveDiscoveryKey(id, date, "syncthing-dht-record-key-v1")
}

// deriveEncryptionKey derives the daily record encryption key for id.
func deriveEncryptionKey(id protocol.DeviceID, date time.Time) [32]byte {
	return deriveDiscoveryKey(id, date, "syncthing-dht-encryption-key-v1")
}

func deriveDiscoveryKey(id protocol.DeviceID, date time.Time, salt string) [32]byte {
	kdf := hkdf.New(sha256.New, id[:], []byte(salt), []byte(discoveryDay(id, date)))
	var key [32]byte
	if _, err := io.ReadFull(kdf, key[:]); err != nil {
		panic("hkdf failure: " + err.Error())
	}
	return key
}

// discoveryKeyCache keeps keys to use for discovery
type discoveryKeyCache struct {
	mut  sync.Mutex
	keys *lru.Cache[discoveryKeyCacheKey, discoveryKeys]
}

type discoveryKeyCacheKey struct {
	id  protocol.DeviceID
	day string
}

type discoveryKeys struct {
	record     string             // lookup key for record, Ed25519 public key
	signing    ed25519.PrivateKey // Ed25519 key for record signing
	encryption [32]byte           // XChaCha20Poly1305 key for record encryption
}

// get returns the keys to use for the given device ID and timestamp
func (c *discoveryKeyCache) get(id protocol.DeviceID, date time.Time) discoveryKeys {
	c.mut.Lock()
	defer c.mut.Unlock()

	if c.keys == nil {
		c.keys, _ = lru.New[discoveryKeyCacheKey, discoveryKeys](1024)
	}

	cacheKey := discoveryKeyCacheKey{id, discoveryDay(id, date)}
	if keys, ok := c.keys.Get(cacheKey); ok {
		return keys
	}

	seed := deriveRecordKey(id, date)
	signing := ed25519.NewKeyFromSeed(seed[:])
	keys := discoveryKeys{
		record:     dhtRecordKey(signing.Public().(ed25519.PublicKey)),
		signing:    signing,
		encryption: deriveEncryptionKey(id, date),
	}
	c.keys.Add(cacheKey, keys)

	return keys
}

// discoveryDay staggers daily rotation across devices. The first eight bytes
// of the device ID select a cutoff in seconds after UTC midnight.
func discoveryDay(id protocol.DeviceID, now time.Time) string {
	offset := time.Duration(binary.BigEndian.Uint64(id[:8])%(24*60*60)) * time.Second
	return now.UTC().Add(-offset).Format(time.DateOnly)
}

// discoveryDates returns timestamps for the current key period and, when a
// record from the previous period could still be valid, the previous one.
func discoveryDates(id protocol.DeviceID, now time.Time) []time.Time {
	dates := []time.Time{now}
	oldest := now.Add(-RecordLifetime)
	if discoveryDay(id, now) != discoveryDay(id, oldest) {
		dates = append(dates, oldest)
	}
	return dates
}
