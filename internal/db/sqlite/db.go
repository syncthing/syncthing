// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package sqlite

import (
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/syncthing/syncthing/internal/db"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/thejerf/suture/v4"
)

type DB struct {
	sql            *sqlx.DB
	localDeviceIdx int64
	updateLock     sync.Mutex
	updatePoints   int

	statementsMut sync.RWMutex
	statements    map[string]*sqlx.Stmt
	tplInput      map[string]any
}

var _ db.DB = (*DB)(nil)

func (s *DB) Close() error {
	s.updateLock.Lock()
	s.statementsMut.Lock()
	defer s.updateLock.Unlock()
	defer s.statementsMut.Unlock()
	for _, stmt := range s.statements {
		stmt.Close()
	}
	return wrap(s.sql.Close())
}

func (s *DB) Service(maintenanceInterval time.Duration) suture.Service {
	return newService(s, maintenanceInterval)
}

func (s *DB) ListFolders() ([]string, error) {
	var res []string
	err := s.stmt(`
		SELECT folder_id FROM folders
		ORDER BY folder_id
	`).Select(&res)
	return res, wrap(err)
}

func (s *DB) ListDevicesForFolder(folder string) ([]protocol.DeviceID, error) {
	var res []string
	err := s.stmt(`
		SELECT d.device_id FROM counts s
		INNER JOIN folders o ON o.idx = s.folder_idx
		INNER JOIN devices d ON d.idx = s.device_idx
		WHERE o.folder_id = ? AND s.count > 0 AND s.device_idx != {{.LocalDeviceIdx}}
		GROUP BY d.device_id
		ORDER BY d.device_id
	`).Select(&res, folder)
	if err != nil {
		return nil, wrap(err)
	}

	devs := make([]protocol.DeviceID, len(res))
	for i, s := range res {
		devs[i], err = protocol.DeviceIDFromString(s)
		if err != nil {
			return nil, wrap(err)
		}
	}
	return devs, nil
}
