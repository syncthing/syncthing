// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package db

import (
	"encoding/binary"
	"time"

	"github.com/boltdb/bolt"
)

// NamespacedKV is a simple key-value store using a specific namespace within
// a database.
type NamespacedKV struct {
	db     *BoltDB
	prefix []byte
}

// NewNamespacedKV returns a new NamespacedKV that lives in the namespace
// specified by the prefix.
func NewNamespacedKV(db *BoltDB, prefix string) *NamespacedKV {
	db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(prefix))
		if err != nil {
			panic(err)
		}
		return nil
	})
	return &NamespacedKV{
		db:     db,
		prefix: []byte(prefix),
	}
}

// Reset removes all entries in this namespace.
func (n *NamespacedKV) Reset() {
	n.db.Update(func(tx *bolt.Tx) error {
		if err := tx.DeleteBucket(n.prefix); err != nil {
			panic(err)
		}
		if _, err := tx.CreateBucket(n.prefix); err != nil {
			panic(err)
		}
		return nil
	})
}

// PutInt64 stores a new int64. Any existing value (even if of another type)
// is overwritten.
func (n *NamespacedKV) PutInt64(key string, val int64) {
	var valBs [8]byte
	binary.BigEndian.PutUint64(valBs[:], uint64(val))
	n.db.Update(func(tx *bolt.Tx) error {
		if err := tx.Bucket(n.prefix).Put([]byte(key), valBs[:]); err != nil {
			panic(err)
		}
		return nil
	})
}

// Int64 returns the stored value interpreted as an int64 and a boolean that
// is false if no value was stored at the key.
func (n *NamespacedKV) Int64(key string) (val int64, ok bool) {
	n.db.View(func(tx *bolt.Tx) error {
		valBs := tx.Bucket(n.prefix).Get([]byte(key))
		if valBs != nil {
			val = int64(binary.BigEndian.Uint64(valBs))
			ok = true
		}
		return nil
	})
	return
}

// PutTime stores a new time.Time. Any existing value (even if of another
// type) is overwritten.
func (n *NamespacedKV) PutTime(key string, val time.Time) {
	valBs, _ := val.MarshalBinary() // never returns an error
	n.db.Update(func(tx *bolt.Tx) error {
		if err := tx.Bucket(n.prefix).Put([]byte(key), valBs[:]); err != nil {
			panic(err)
		}
		return nil
	})
}

// Time returns the stored value interpreted as a time.Time and a boolean
// that is false if no value was stored at the key.
func (n NamespacedKV) Time(key string) (t time.Time, ok bool) {
	n.db.View(func(tx *bolt.Tx) error {
		valBs := tx.Bucket(n.prefix).Get([]byte(key))
		if valBs != nil {
			err := t.UnmarshalBinary(valBs)
			ok = err == nil
		}
		return nil
	})
	return
}

// PutString stores a new string. Any existing value (even if of another type)
// is overwritten.
func (n *NamespacedKV) PutString(key, val string) {
	n.db.Update(func(tx *bolt.Tx) error {
		if err := tx.Bucket(n.prefix).Put([]byte(key), []byte(val)); err != nil {
			panic(err)
		}
		return nil
	})
}

// String returns the stored value interpreted as a string and a boolean that
// is false if no value was stored at the key.
func (n NamespacedKV) String(key string) (s string, ok bool) {
	n.db.View(func(tx *bolt.Tx) error {
		valBs := tx.Bucket(n.prefix).Get([]byte(key))
		if valBs != nil {
			s = string(valBs)
			ok = true
		}
		return nil
	})
	return
}

// PutBytes stores a new byte slice. Any existing value (even if of another type)
// is overwritten.
func (n *NamespacedKV) PutBytes(key string, val []byte) {
	n.db.Update(func(tx *bolt.Tx) error {
		if err := tx.Bucket(n.prefix).Put([]byte(key), val); err != nil {
			panic(err)
		}
		return nil
	})
}

// Bytes returns the stored value as a raw byte slice and a boolean that
// is false if no value was stored at the key.
func (n NamespacedKV) Bytes(key string) (v []byte, ok bool) {
	n.db.View(func(tx *bolt.Tx) error {
		v = tx.Bucket(n.prefix).Get([]byte(key))
		if v != nil {
			ok = true
		}
		return nil
	})
	return
}

// PutBool stores a new boolean. Any existing value (even if of another type)
// is overwritten.
func (n *NamespacedKV) PutBool(key string, val bool) {
	if val {
		n.PutBytes(key, []byte{1})
	} else {
		n.PutBytes(key, []byte{0})
	}
}

// Bool returns the stored value as a boolean and a boolean that
// is false if no value was stored at the key.
func (n NamespacedKV) Bool(key string) (bool, bool) {
	bs, ok := n.Bytes(key)
	if ok && len(bs) >= 1 {
		return bs[0] != 0, true
	}
	return false, false
}

// Delete deletes the specified key. It is allowed to delete a nonexistent
// key.
func (n NamespacedKV) Delete(key string) {
	n.db.Update(func(tx *bolt.Tx) error {
		if err := tx.Bucket(n.prefix).Delete([]byte(key)); err != nil {
			panic(err)
		}
		return nil
	})
}
