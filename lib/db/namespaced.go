// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package db

import (
	"encoding/binary"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/util"
)

// NamespacedKV is a simple key-value store using a specific namespace within
// a leveldb.
type NamespacedKV struct {
	db     *Instance
	prefix []byte
}

// NewNamespacedKV returns a new NamespacedKV that lives in the namespace
// specified by the prefix.
func NewNamespacedKV(db *Instance, prefix string) *NamespacedKV {
	return &NamespacedKV{
		db:     db,
		prefix: []byte(prefix),
	}
}

// Reset removes all entries in this namespace.
func (n *NamespacedKV) Reset() {
	it := n.db.NewIterator(util.BytesPrefix(n.prefix), nil)
	defer it.Release()
	batch := new(leveldb.Batch)
	for it.Next() {
		batch.Delete(it.Key())
		if batch.Len() > batchFlushSize {
			if err := n.db.Write(batch, nil); err != nil {
				panic(err)
			}
			batch.Reset()
		}
	}
	if batch.Len() > 0 {
		if err := n.db.Write(batch, nil); err != nil {
			panic(err)
		}
	}
}

// PutInt64 stores a new int64. Any existing value (even if of another type)
// is overwritten.
func (n *NamespacedKV) PutInt64(key string, val int64) {
	keyBs := append(n.prefix, []byte(key)...)
	var valBs [8]byte
	binary.BigEndian.PutUint64(valBs[:], uint64(val))
	n.db.Put(keyBs, valBs[:], nil)
}

// Int64 returns the stored value interpreted as an int64 and a boolean that
// is false if no value was stored at the key.
func (n *NamespacedKV) Int64(key string) (int64, bool) {
	keyBs := append(n.prefix, []byte(key)...)
	valBs, err := n.db.Get(keyBs, nil)
	if err != nil {
		return 0, false
	}
	val := binary.BigEndian.Uint64(valBs)
	return int64(val), true
}

// PutTime stores a new time.Time. Any existing value (even if of another
// type) is overwritten.
func (n *NamespacedKV) PutTime(key string, val time.Time) {
	keyBs := append(n.prefix, []byte(key)...)
	valBs, _ := val.MarshalBinary() // never returns an error
	n.db.Put(keyBs, valBs, nil)
}

// Time returns the stored value interpreted as a time.Time and a boolean
// that is false if no value was stored at the key.
func (n NamespacedKV) Time(key string) (time.Time, bool) {
	var t time.Time
	keyBs := append(n.prefix, []byte(key)...)
	valBs, err := n.db.Get(keyBs, nil)
	if err != nil {
		return t, false
	}
	err = t.UnmarshalBinary(valBs)
	return t, err == nil
}

// PutString stores a new string. Any existing value (even if of another type)
// is overwritten.
func (n *NamespacedKV) PutString(key, val string) {
	keyBs := append(n.prefix, []byte(key)...)
	n.db.Put(keyBs, []byte(val), nil)
}

// String returns the stored value interpreted as a string and a boolean that
// is false if no value was stored at the key.
func (n NamespacedKV) String(key string) (string, bool) {
	keyBs := append(n.prefix, []byte(key)...)
	valBs, err := n.db.Get(keyBs, nil)
	if err != nil {
		return "", false
	}
	return string(valBs), true
}

// PutBytes stores a new byte slice. Any existing value (even if of another type)
// is overwritten.
func (n *NamespacedKV) PutBytes(key string, val []byte) {
	keyBs := append(n.prefix, []byte(key)...)
	n.db.Put(keyBs, val, nil)
}

// Bytes returns the stored value as a raw byte slice and a boolean that
// is false if no value was stored at the key.
func (n NamespacedKV) Bytes(key string) ([]byte, bool) {
	keyBs := append(n.prefix, []byte(key)...)
	valBs, err := n.db.Get(keyBs, nil)
	if err != nil {
		return nil, false
	}
	return valBs, true
}

// PutBool stores a new boolean. Any existing value (even if of another type)
// is overwritten.
func (n *NamespacedKV) PutBool(key string, val bool) {
	keyBs := append(n.prefix, []byte(key)...)
	if val {
		n.db.Put(keyBs, []byte{0x0}, nil)
	} else {
		n.db.Put(keyBs, []byte{0x1}, nil)
	}
}

// Bool returns the stored value as a boolean and a boolean that
// is false if no value was stored at the key.
func (n NamespacedKV) Bool(key string) (bool, bool) {
	keyBs := append(n.prefix, []byte(key)...)
	valBs, err := n.db.Get(keyBs, nil)
	if err != nil {
		return false, false
	}
	return valBs[0] == 0x0, true
}

// Delete deletes the specified key. It is allowed to delete a nonexistent
// key.
func (n NamespacedKV) Delete(key string) {
	keyBs := append(n.prefix, []byte(key)...)
	n.db.Delete(keyBs, nil)
}
