// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package sqlite

import (
	"iter"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/syncthing/syncthing/internal/db"
)

func (s *baseDB) GetKV(key string) ([]byte, error) {
	now := time.Now().UnixNano()
	var val []byte
	if err := s.stmt(`
		SELECT value FROM kv
		WHERE key = ? AND (expires IS null OR expires > ?)
	`).Get(&val, key, now); err != nil {
		return nil, wrap(err)
	}
	return val, nil
}

func (s *baseDB) PutKV(key string, val []byte) error {
	return wrap(s.PutKVExpires(key, val, time.Time{}))
}

func (s *baseDB) PutKVExpires(key string, val []byte, expires time.Time) error {
	var expPtr *int64
	if !expires.IsZero() {
		v := expires.UnixNano()
		expPtr = &v
	}
	s.updateLock.Lock()
	defer s.updateLock.Unlock()
	_, err := s.stmt(`
		INSERT OR REPLACE INTO kv (key, value, expires)
		VALUES (?, ?, ?)
	`).Exec(key, val, expPtr)
	return wrap(err)
}

func (s *baseDB) DeleteKV(key string) error {
	s.updateLock.Lock()
	defer s.updateLock.Unlock()
	_, err := s.stmt(`
		DELETE FROM kv WHERE key = ?
	`).Exec(key)
	return wrap(err)
}

func (s *baseDB) PrefixKV(prefix string) (iter.Seq[db.KeyValue], func() error) {
	var rows *sqlx.Rows
	var err error
	now := time.Now().UnixNano()
	if prefix == "" {
		rows, err = s.stmt(`SELECT key, value FROM kv WHERE expires IS null OR expires > ?`).Queryx(now)
	} else {
		end := prefixEnd(prefix)
		rows, err = s.stmt(`
			SELECT key, value FROM kv
			WHERE key >= ? AND key < ?
				AND (expires IS null OR expires > ?)
		`).Queryx(prefix, end, now)
	}
	if err != nil {
		return func(_ func(db.KeyValue) bool) {}, func() error { return err }
	}

	return func(yield func(db.KeyValue) bool) {
			defer rows.Close()
			for rows.Next() {
				var key string
				var val []byte
				if err = rows.Scan(&key, &val); err != nil {
					return
				}
				if !yield(db.KeyValue{Key: key, Value: val}) {
					return
				}
			}
			err = rows.Err()
		}, func() error {
			return err
		}
}

func (s *baseDB) expireKVLocked() error {
	now := time.Now().UnixNano()
	_, err := s.stmt(`
		DELETE FROM kv
		WHERE expires < ?
	`).Exec(now)
	return wrap(err)
}
