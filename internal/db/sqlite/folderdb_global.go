// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"iter"

	"github.com/syncthing/syncthing/internal/db"
	"github.com/syncthing/syncthing/internal/itererr"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/protocol"
)

func (s *folderDB) GetGlobalFile(file string) (protocol.FileInfo, bool, error) {
	file = osutil.NormalizedFilename(file)

	var ind indirectFI
	err := s.stmt(`
		SELECT fi.fiprotobuf, bl.blprotobuf FROM fileinfos fi
		INNER JOIN files f on fi.sequence = f.sequence
		LEFT JOIN blocklists bl ON bl.blocklist_hash = f.blocklist_hash
		INNER JOIN file_names n ON f.name_idx = n.idx
		WHERE n.name = ? AND f.local_flags & {{.FlagLocalGlobal}} != 0
	`).Get(&ind, file)
	if errors.Is(err, sql.ErrNoRows) {
		return protocol.FileInfo{}, false, nil
	}
	if err != nil {
		return protocol.FileInfo{}, false, wrap(err)
	}
	fi, err := ind.FileInfo()
	if err != nil {
		return protocol.FileInfo{}, false, wrap(err)
	}
	return fi, true, nil
}

func (s *folderDB) GetGlobalAvailability(file string) ([]protocol.DeviceID, error) {
	file = osutil.NormalizedFilename(file)

	var devStrs []string
	err := s.stmt(`
		SELECT d.device_id FROM files f
		INNER JOIN devices d ON d.idx = f.device_idx
		INNER JOIN files g ON g.version_idx = f.version_idx AND g.name_idx = f.name_idx
		INNER JOIN file_names n ON f.name_idx = n.idx
		WHERE n.name = ? AND g.local_flags & {{.FlagLocalGlobal}} != 0 AND f.device_idx != {{.LocalDeviceIdx}}
		ORDER BY d.device_id
	`).Select(&devStrs, file)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, wrap(err)
	}

	devs := make([]protocol.DeviceID, 0, len(devStrs))
	for _, s := range devStrs {
		d, err := protocol.DeviceIDFromString(s)
		if err != nil {
			return nil, wrap(err)
		}
		devs = append(devs, d)
	}

	return devs, nil
}

func (s *folderDB) AllGlobalFiles() (iter.Seq[db.FileMetadata], func() error) {
	it, errFn := iterStructs[db.FileMetadata](s.stmt(`
		SELECT f.sequence, n.name, f.type, f.modified as modnanos, f.size, f.deleted, f.local_flags as localflags FROM files f
		INNER JOIN file_names n ON f.name_idx = n.idx
		WHERE f.local_flags & {{.FlagLocalGlobal}} != 0
		ORDER BY n.name
	`).Queryx())
	return itererr.Map(it, errFn, func(m db.FileMetadata) (db.FileMetadata, error) {
		m.Name = osutil.NativeFilename(m.Name)
		return m, nil
	})
}

func (s *folderDB) AllGlobalFilesPrefix(prefix string) (iter.Seq[db.FileMetadata], func() error) {
	if prefix == "" {
		return s.AllGlobalFiles()
	}

	prefix = osutil.NormalizedFilename(prefix)
	end := prefixEnd(prefix)

	it, errFn := iterStructs[db.FileMetadata](s.stmt(`
		SELECT f.sequence, n.name, f.type, f.modified as modnanos, f.size, f.deleted, f.local_flags as localflags FROM files f
		INNER JOIN file_names n ON f.name_idx = n.idx
		WHERE n.name >= ? AND n.name < ? AND f.local_flags & {{.FlagLocalGlobal}} != 0
		ORDER BY n.name
	`).Queryx(prefix, end))
	return itererr.Map(it, errFn, func(m db.FileMetadata) (db.FileMetadata, error) {
		m.Name = osutil.NativeFilename(m.Name)
		return m, nil
	})
}

func (s *folderDB) AllNeededGlobalFiles(device protocol.DeviceID, order config.PullOrder, limit, offset int) (iter.Seq[protocol.FileInfo], func() error) {
	var selectOpts string
	switch order {
	case config.PullOrderRandom:
		selectOpts = "ORDER BY RANDOM()"
	case config.PullOrderAlphabetic:
		selectOpts = "ORDER BY n.name ASC"
	case config.PullOrderSmallestFirst:
		selectOpts = "ORDER BY g.size ASC"
	case config.PullOrderLargestFirst:
		selectOpts = "ORDER BY g.size DESC"
	case config.PullOrderOldestFirst:
		selectOpts = "ORDER BY g.modified ASC"
	case config.PullOrderNewestFirst:
		selectOpts = "ORDER BY g.modified DESC"
	}

	if limit > 0 {
		selectOpts += fmt.Sprintf(" LIMIT %d", limit)
	}
	if offset > 0 {
		selectOpts += fmt.Sprintf(" OFFSET %d", offset)
	}

	if device == protocol.LocalDeviceID {
		return s.neededGlobalFilesLocal(selectOpts)
	}

	return s.neededGlobalFilesRemote(device, selectOpts)
}

func (s *folderDB) neededGlobalFilesLocal(selectOpts string) (iter.Seq[protocol.FileInfo], func() error) {
	// Select all the non-ignored files with the need bit set.
	it, errFn := iterStructs[indirectFI](s.stmt(`
		SELECT fi.fiprotobuf, bl.blprotobuf, n.name, g.size, g.modified FROM fileinfos fi
		INNER JOIN files g on fi.sequence = g.sequence
		LEFT JOIN blocklists bl ON bl.blocklist_hash = g.blocklist_hash
		INNER JOIN file_names n ON g.name_idx = n.idx
		WHERE g.local_flags & {{.FlagLocalIgnored}} = 0 AND g.local_flags & {{.FlagLocalNeeded}} != 0
	` + selectOpts).Queryx())
	return itererr.Map(it, errFn, indirectFI.FileInfo)
}

func (s *folderDB) neededGlobalFilesRemote(device protocol.DeviceID, selectOpts string) (iter.Seq[protocol.FileInfo], func() error) {
	// Select:
	//
	// - all the valid, non-deleted global files that don't have a
	//   corresponding remote file with the same version.
	//
	// - all the valid, deleted global files that have a corresponding
	//   non-deleted and valid remote file (of any version)

	it, errFn := iterStructs[indirectFI](s.stmt(`
		SELECT fi.fiprotobuf, bl.blprotobuf, n.name, g.size, g.modified FROM fileinfos fi
		INNER JOIN files g on fi.sequence = g.sequence
		LEFT JOIN blocklists bl ON bl.blocklist_hash = g.blocklist_hash
		INNER JOIN file_names n ON g.name_idx = n.idx
		WHERE g.local_flags & {{.FlagLocalGlobal}} != 0 AND NOT g.deleted AND g.local_flags & {{.LocalInvalidFlags}} = 0 AND NOT EXISTS (
			SELECT 1 FROM FILES f
			INNER JOIN devices d ON d.idx = f.device_idx
			WHERE f.name_idx = g.name_idx AND f.version_idx = g.version_idx AND d.device_id = ?
		)

		UNION ALL

		SELECT fi.fiprotobuf, bl.blprotobuf, n.name, g.size, g.modified FROM fileinfos fi
		INNER JOIN files g on fi.sequence = g.sequence
		LEFT JOIN blocklists bl ON bl.blocklist_hash = g.blocklist_hash
		INNER JOIN file_names n ON g.name_idx = n.idx
		WHERE g.local_flags & {{.FlagLocalGlobal}} != 0 AND g.deleted AND g.local_flags & {{.LocalInvalidFlags}} = 0 AND EXISTS (
			SELECT 1 FROM FILES f
			INNER JOIN devices d ON d.idx = f.device_idx
			WHERE f.name_idx = g.name_idx AND d.device_id = ? AND NOT f.deleted AND f.local_flags & {{.LocalInvalidFlags}} = 0
		)
	`+selectOpts).Queryx(
		device.String(),
		device.String(),
	))
	return itererr.Map(it, errFn, indirectFI.FileInfo)
}
