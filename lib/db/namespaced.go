// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package db

import (
	"encoding/binary"
	"time"

	"github.com/syndtr/goleveldb/leveldb/util"
)

// NamespacedKV is a simple key-value store using a specific namespace within
// a leveldb.
type NamespacedKV struct {
	db     *Lowlevel
	prefix []byte
}

// NewNamespacedKV returns a new NamespacedKV that lives in the namespace
// specified by the prefix.
func NewNamespacedKV(db *Lowlevel, prefix string) *NamespacedKV {
	prefixBs := []byte(prefix)
	// After the conversion from string the cap will be larger than the len (in Go 1.11.5,
	// 32 bytes cap for small strings). We need to cut it down to ensure append() calls
	// on the prefix make a new allocation.
	prefixBs = prefixBs[:len(prefixBs):len(prefixBs)]
	return &NamespacedKV{
		db:     db,
		prefix: prefixBs,
	}
}

// Reset removes all entries in this namespace.
func (n *NamespacedKV) Reset() {
	it := n.db.NewIterator(util.BytesPrefix(n.prefix), nil)
	defer it.Release()
	batch := n.db.newBatch()
	for it.Next() {
		batch.Delete(it.Key())
		batch.checkFlush()
	}
	batch.flush()
}

// PutInt64 stores a new int64. Any existing value (even if of another type)
// is overwritten.
func (n *NamespacedKV) PutInt64(key string, val int64) {
	var valBs [8]byte
	binary.BigEndian.PutUint64(valBs[:], uint64(val))
	n.db.Put(n.prefixedKey(key), valBs[:], nil)
}

// Int64 returns the stored value interpreted as an int64 and a boolean that
// is false if no value was stored at the key.
func (n *NamespacedKV) Int64(key string) (int64, bool) {
	valBs, err := n.db.Get(n.prefixedKey(key), nil)
	if err != nil {
		return 0, false
	}
	val := binary.BigEndian.Uint64(valBs)
	return int64(val), true
}

// PutTime stores a new time.Time. Any existing value (even if of another
// type) is overwritten.
func (n *NamespacedKV) PutTime(key string, val time.Time) {
	valBs, _ := val.MarshalBinary() // never returns an error
	n.db.Put(n.prefixedKey(key), valBs, nil)
}

// Time returns the stored value interpreted as a time.Time and a boolean
// that is false if no value was stored at the key.
func (n NamespacedKV) Time(key string) (time.Time, bool) {
	var t time.Time
	valBs, err := n.db.Get(n.prefixedKey(key), nil)
	if err != nil {
		return t, false
	}
	err = t.UnmarshalBinary(valBs)
	return t, err == nil
}

// PutString stores a new string. Any existing value (even if of another type)
// is overwritten.
func (n *NamespacedKV) PutString(key, val string) {
	n.db.Put(n.prefixedKey(key), []byte(val), nil)
}

// String returns the stored value interpreted as a string and a boolean that
// is false if no value was stored at the key.
func (n NamespacedKV) String(key string) (string, bool) {
	valBs, err := n.db.Get(n.prefixedKey(key), nil)
	if err != nil {
		return "", false
	}
	return string(valBs), true
}

// PutBytes stores a new byte slice. Any existing value (even if of another type)
// is overwritten.
func (n *NamespacedKV) PutBytes(key string, val []byte) {
	n.db.Put(n.prefixedKey(key), val, nil)
}

// Bytes returns the stored value as a raw byte slice and a boolean that
// is false if no value was stored at the key.
func (n NamespacedKV) Bytes(key string) ([]byte, bool) {
	valBs, err := n.db.Get(n.prefixedKey(key), nil)
	if err != nil {
		return nil, false
	}
	return valBs, true
}

// PutBool stores a new boolean. Any existing value (even if of another type)
// is overwritten.
func (n *NamespacedKV) PutBool(key string, val bool) {
	if val {
		n.db.Put(n.prefixedKey(key), []byte{0x0}, nil)
	} else {
		n.db.Put(n.prefixedKey(key), []byte{0x1}, nil)
	}
}

// Bool returns the stored value as a boolean and a boolean that
// is false if no value was stored at the key.
func (n NamespacedKV) Bool(key string) (bool, bool) {
	valBs, err := n.db.Get(n.prefixedKey(key), nil)
	if err != nil {
		return false, false
	}
	return valBs[0] == 0x0, true
}

// Delete deletes the specified key. It is allowed to delete a nonexistent
// key.
func (n NamespacedKV) Delete(key string) {
	n.db.Delete(n.prefixedKey(key), nil)
}

func (n NamespacedKV) prefixedKey(key string) []byte {
	return append(n.prefix, []byte(key)...)
}

// Well known namespaces that can be instantiated without knowing the key
// details.

// NewDeviceStatisticsNamespace creates a KV namespace for device statistics
// for the given device.
func NewDeviceStatisticsNamespace(db *Lowlevel, device string) *NamespacedKV {
	return NewNamespacedKV(db, string(KeyTypeDeviceStatistic)+device)
}

// NewFolderStatisticsNamespace creates a KV namespace for folder statistics
// for the given folder.
func NewFolderStatisticsNamespace(db *Lowlevel, folder string) *NamespacedKV {
	return NewNamespacedKV(db, string(KeyTypeFolderStatistic)+folder)
}

// NewMiscDateNamespace creates a KV namespace for miscellaneous metadata.
func NewMiscDataNamespace(db *Lowlevel) *NamespacedKV {
	return NewNamespacedKV(db, string(KeyTypeMiscData))
}
