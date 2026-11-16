// Copyright (C) 2026 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package dht

import (
	"crypto/ed25519"
	"crypto/sha256"
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
	// Device IDs have sufficient entropy for HKDF. Use a canonical UTC date
	// so all peers derive the same keys, independent of their time zones.
	kdf := hkdf.New(sha256.New, id[:], []byte(salt), []byte(date.UTC().Format(time.DateOnly)))
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

	cacheKey := discoveryKeyCacheKey{id, date.UTC().Format(time.DateOnly)}
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

// discoveryDates return she dates for which we should attempt lookup;
// todays date, and possibly yesterdays.
func discoveryDates(now time.Time) []time.Time {
	dates := []time.Time{now}
	oldest := now.Add(-RecordLifetime)
	if now.UTC().Format(time.DateOnly) != oldest.UTC().Format(time.DateOnly) {
		dates = append(dates, oldest)
	}
	return dates
}
