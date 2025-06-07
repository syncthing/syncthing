// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package sqlite

import (
	"time"
)

func (s *folderDB) GetMtime(name string) (ondisk, virtual time.Time) {
	var res struct {
		Ondisk  int64
		Virtual int64
	}
	if err := s.stmt(`
		SELECT m.ondisk, m.virtual FROM mtimes m
		WHERE m.name = ?
	`).Get(&res, name); err != nil {
		return time.Time{}, time.Time{}
	}
	return time.Unix(0, res.Ondisk), time.Unix(0, res.Virtual)
}

func (s *folderDB) PutMtime(name string, ondisk, virtual time.Time) error {
	s.updateLock.Lock()
	defer s.updateLock.Unlock()
	_, err := s.stmt(`
		INSERT OR REPLACE INTO mtimes (name, ondisk, virtual)
		VALUES (?, ?, ?)
	`).Exec(name, ondisk.UnixNano(), virtual.UnixNano())
	return wrap(err)
}

func (s *folderDB) DeleteMtime(name string) error {
	s.updateLock.Lock()
	defer s.updateLock.Unlock()
	_, err := s.stmt(`
		DELETE FROM mtimes
		WHERE name = ?
	`).Exec(name)
	return wrap(err)
}
