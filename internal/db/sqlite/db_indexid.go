// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package sqlite

import (
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/syncthing/syncthing/internal/itererr"
	"github.com/syncthing/syncthing/lib/protocol"
)

func (s *DB) GetIndexID(folder string, device protocol.DeviceID) (protocol.IndexID, error) {
	// Try a fast read-only query to begin with. If it does not find the ID
	// we'll do the full thing under a lock.
	var indexID string
	if err := s.stmt(`
		SELECT i.index_id FROM indexids i
		INNER JOIN folders o ON o.idx  = i.folder_idx
		INNER JOIN devices d ON d.idx  = i.device_idx
		WHERE o.folder_id = ? AND d.device_id = ?
	`).Get(&indexID, folder, device.String()); err == nil && indexID != "" {
		idx, err := indexIDFromHex(indexID)
		return idx, wrap(err)
	}
	if device != protocol.LocalDeviceID {
		// For non-local devices we do not create the index ID, so return
		// zero anyway if we don't have one.
		return 0, nil
	}

	s.updateLock.Lock()
	defer s.updateLock.Unlock()

	// We are now operating only for the local device ID

	folderIdx, err := s.folderIdxLocked(folder)
	if err != nil {
		return 0, wrap(err)
	}

	if err := s.stmt(`
		SELECT index_id FROM indexids WHERE folder_idx = ? AND device_idx = {{.LocalDeviceIdx}}
	`).Get(&indexID, folderIdx); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, wrap(err)
	}

	if indexID == "" {
		// Generate a new index ID
		id := protocol.NewIndexID()
		if _, err := s.stmt(`
			INSERT INTO indexids (folder_idx, device_idx, index_id) values (?, {{.LocalDeviceIdx}}, ?)
		`).Exec(folderIdx, indexIDToHex(id)); err != nil {
			return 0, wrap(err)
		}
		return id, nil
	}

	return indexIDFromHex(indexID)
}

func (s *DB) SetIndexID(folder string, device protocol.DeviceID, id protocol.IndexID) error {
	s.updateLock.Lock()
	defer s.updateLock.Unlock()

	folderIdx, err := s.folderIdxLocked(folder)
	if err != nil {
		return wrap(err, "folder idx")
	}
	deviceIdx, err := s.deviceIdxLocked(device)
	if err != nil {
		return wrap(err, "device idx")
	}

	if _, err := s.stmt(`
		INSERT OR REPLACE INTO indexids (folder_idx, device_idx, index_id) values (?, ?, ?)
	`).Exec(folderIdx, deviceIdx, indexIDToHex(id)); err != nil {
		return wrap(err, "insert")
	}
	return nil
}

func (s *DB) DropAllIndexIDs() error {
	s.updateLock.Lock()
	defer s.updateLock.Unlock()
	_, err := s.stmt(`DELETE FROM indexids`).Exec()
	return wrap(err)
}

func (s *DB) GetDeviceSequence(folder string, device protocol.DeviceID) (int64, error) {
	var res sql.NullInt64
	err := s.stmt(`
		SELECT sequence FROM indexids i
		INNER JOIN folders o ON o.idx = i.folder_idx
		INNER JOIN devices d ON d.idx = i.device_idx
		WHERE o.folder_id = ? AND d.device_id = ?
	`).Get(&res, folder, device.String())
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, wrap(err)
	}
	if !res.Valid {
		return 0, nil
	}
	return res.Int64, nil
}

func (s *DB) RemoteSequences(folder string) (map[protocol.DeviceID]int64, error) {
	type row struct {
		Device string
		Seq    int64
	}

	it, errFn := iterStructs[row](s.stmt(`
		SELECT d.device_id AS device, i.sequence AS seq FROM indexids i
		INNER JOIN folders o ON o.idx = i.folder_idx
		INNER JOIN devices d ON d.idx = i.device_idx
		WHERE o.folder_id = ? AND i.device_idx != {{.LocalDeviceIdx}}
	`).Queryx(folder))

	res := make(map[protocol.DeviceID]int64)
	for row, err := range itererr.Zip(it, errFn) {
		if err != nil {
			return nil, wrap(err)
		}
		dev, err := protocol.DeviceIDFromString(row.Device)
		if err != nil {
			return nil, wrap(err, "device ID")
		}
		res[dev] = row.Seq
	}
	return res, nil
}

func indexIDFromHex(s string) (protocol.IndexID, error) {
	bs, err := hex.DecodeString(s)
	if err != nil {
		return 0, fmt.Errorf("indexIDFromHex: %q: %w", s, err)
	}
	var id protocol.IndexID
	if err := id.Unmarshal(bs); err != nil {
		return 0, fmt.Errorf("indexIDFromHex: %q: %w", s, err)
	}
	return id, nil
}

func indexIDToHex(i protocol.IndexID) string {
	bs, _ := i.Marshal()
	return hex.EncodeToString(bs)
}
