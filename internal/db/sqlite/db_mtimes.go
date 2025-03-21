// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package sqlite

import (
	"time"
)

func (s *DB) GetMtime(folder, name string) (ondisk, virtual time.Time) {
	var res struct {
		Ondisk  int64
		Virtual int64
	}
	if err := s.stmt(`
		SELECT m.ondisk, m.virtual FROM mtimes m
		INNER JOIN folders o ON o.idx = m.folder_idx
		WHERE o.folder_id = ? AND m.name = ?
	`).Get(&res, folder, name); err != nil {
		return time.Time{}, time.Time{}
	}
	return time.Unix(0, res.Ondisk), time.Unix(0, res.Virtual)
}

func (s *DB) PutMtime(folder, name string, ondisk, virtual time.Time) error {
	s.updateLock.Lock()
	defer s.updateLock.Unlock()
	folderIdx, err := s.folderIdxLocked(folder)
	if err != nil {
		return wrap(err)
	}
	_, err = s.stmt(`
		INSERT OR REPLACE INTO mtimes (folder_idx, name, ondisk, virtual)
		VALUES (?, ?, ?, ?)
	`).Exec(folderIdx, name, ondisk.UnixNano(), virtual.UnixNano())
	return wrap(err)
}

func (s *DB) DeleteMtime(folder, name string) error {
	s.updateLock.Lock()
	defer s.updateLock.Unlock()
	folderIdx, err := s.folderIdxLocked(folder)
	if err != nil {
		return wrap(err)
	}
	_, err = s.stmt(`
		DELETE FROM mtimes
		WHERE folder_idx = ? AND name = ?
	`).Exec(folderIdx, name)
	return wrap(err)
}
