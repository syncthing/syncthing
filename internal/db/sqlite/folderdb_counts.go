// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package sqlite

import (
	"github.com/syncthing/syncthing/internal/db"
	"github.com/syncthing/syncthing/lib/protocol"
)

type countsRow struct {
	Type       protocol.FileInfoType
	Count      int
	Size       int64
	Deleted    bool
	LocalFlags protocol.FlagLocal `db:"local_flags"`
}

func (s *folderDB) CountLocal(device protocol.DeviceID) (db.Counts, error) {
	var res []countsRow
	if err := s.stmt(`
		SELECT s.type, s.count, s.size, s.local_flags, s.deleted FROM counts s
		INNER JOIN devices d ON d.idx = s.device_idx
		WHERE d.device_id = ? AND s.local_flags & {{.FlagLocalIgnored}} = 0
	`).Select(&res, device.String()); err != nil {
		return db.Counts{}, wrap(err)
	}
	return summarizeCounts(res), nil
}

func (s *folderDB) CountNeed(device protocol.DeviceID) (db.Counts, error) {
	if device == protocol.LocalDeviceID {
		return s.needSizeLocal()
	}
	return s.needSizeRemote(device)
}

func (s *folderDB) CountGlobal() (db.Counts, error) {
	var res []countsRow
	err := s.stmt(`
		SELECT s.type, s.count, s.size, s.local_flags, s.deleted FROM counts s
		WHERE s.local_flags & {{.FlagLocalGlobal}} != 0 AND s.local_flags & {{.LocalInvalidFlags}} = 0
	`).Select(&res)
	if err != nil {
		return db.Counts{}, wrap(err)
	}
	return summarizeCounts(res), nil
}

func (s *folderDB) CountReceiveOnlyChanged() (db.Counts, error) {
	var res []countsRow
	err := s.stmt(`
		SELECT s.type, s.count, s.size, s.local_flags, s.deleted FROM counts s
		WHERE local_flags & {{.FlagLocalReceiveOnly}} != 0
	`).Select(&res)
	if err != nil {
		return db.Counts{}, wrap(err)
	}
	return summarizeCounts(res), nil
}

func (s *folderDB) needSizeLocal() (db.Counts, error) {
	// The need size for the local device is the sum of entries with the
	// need bit set.
	var res []countsRow
	err := s.stmt(`
		SELECT s.type, s.count, s.size, s.local_flags, s.deleted FROM counts s
		WHERE s.local_flags & {{.FlagLocalNeeded}} != 0
	`).Select(&res)
	if err != nil {
		return db.Counts{}, wrap(err)
	}
	return summarizeCounts(res), nil
}

func (s *folderDB) needSizeRemote(device protocol.DeviceID) (db.Counts, error) {
	var res []countsRow
	// See neededGlobalFilesRemote for commentary as that is the same query without summing
	if err := s.stmt(`
		SELECT g.type, count(*) as count, sum(g.size) as size, g.local_flags, g.deleted FROM files g
		WHERE g.local_flags & {{.FlagLocalGlobal}} != 0 AND NOT g.deleted AND g.local_flags & {{.LocalInvalidFlags}} = 0 AND NOT EXISTS (
			SELECT 1 FROM FILES f
			INNER JOIN devices d ON d.idx = f.device_idx
			WHERE f.name_idx = g.name_idx AND f.version_idx = g.version_idx AND d.device_id = ?
		)
		GROUP BY g.type, g.local_flags, g.deleted

		UNION ALL

		SELECT g.type, count(*) as count, sum(g.size) as size, g.local_flags, g.deleted FROM files g
		WHERE g.local_flags & {{.FlagLocalGlobal}} != 0 AND g.deleted AND g.local_flags & {{.LocalInvalidFlags}} = 0 AND EXISTS (
			SELECT 1 FROM FILES f
			INNER JOIN devices d ON d.idx = f.device_idx
			WHERE f.name_idx = g.name_idx AND d.device_id = ? AND NOT f.deleted AND f.local_flags & {{.LocalInvalidFlags}} = 0
		)
		GROUP BY g.type, g.local_flags, g.deleted
	`).Select(&res, device.String(),
		device.String()); err != nil {
		return db.Counts{}, wrap(err)
	}

	return summarizeCounts(res), nil
}

func summarizeCounts(res []countsRow) db.Counts {
	c := db.Counts{
		DeviceID: protocol.LocalDeviceID,
	}
	for _, r := range res {
		switch {
		case r.Deleted:
			c.Deleted += r.Count
		case r.Type == protocol.FileInfoTypeFile:
			c.Files += r.Count
			c.Bytes += r.Size
		case r.Type == protocol.FileInfoTypeDirectory:
			c.Directories += r.Count
			c.Bytes += r.Size
		case r.Type == protocol.FileInfoTypeSymlink:
			c.Symlinks += r.Count
			c.Bytes += r.Size
		}
	}
	return c
}
