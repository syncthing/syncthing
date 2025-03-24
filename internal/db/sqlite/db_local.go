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
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/protocol"
)

func (s *DB) GetDeviceFile(folder string, device protocol.DeviceID, file string) (protocol.FileInfo, bool, error) {
	file = osutil.NormalizedFilename(file)

	var ind indirectFI
	err := s.stmt(`
		SELECT fi.fiprotobuf, bl.blprotobuf FROM fileinfos fi
		INNER JOIN files f on fi.sequence = f.sequence
		LEFT JOIN blocklists bl ON bl.blocklist_hash = f.blocklist_hash
		INNER JOIN devices d ON f.device_idx = d.idx
		INNER JOIN folders o ON f.folder_idx = o.idx
		WHERE o.folder_id = ? AND d.device_id = ? AND f.name = ?
	`).Get(&ind, folder, device.String(), file)
	if errors.Is(err, sql.ErrNoRows) {
		return protocol.FileInfo{}, false, nil
	}
	if err != nil {
		return protocol.FileInfo{}, false, wrap(err)
	}
	fi, err := ind.FileInfo()
	if err != nil {
		return protocol.FileInfo{}, false, wrap(err, "indirect")
	}
	return fi, true, nil
}

func (s *DB) AllLocalFiles(folder string, device protocol.DeviceID) (iter.Seq[protocol.FileInfo], func() error) {
	it, errFn := iterStructs[indirectFI](s.stmt(`
		SELECT fi.fiprotobuf, bl.blprotobuf FROM fileinfos fi
		INNER JOIN files f on fi.sequence = f.sequence
		LEFT JOIN blocklists bl ON bl.blocklist_hash = f.blocklist_hash
		INNER JOIN folders o ON o.idx = f.folder_idx
		INNER JOIN devices d ON d.idx = f.device_idx
		WHERE o.folder_id = ? AND d.device_id = ?
	`).Queryx(folder, device.String()))
	return itererr.Map(it, errFn, indirectFI.FileInfo)
}

func (s *DB) AllLocalFilesBySequence(folder string, device protocol.DeviceID, startSeq int64, limit int) (iter.Seq[protocol.FileInfo], func() error) {
	var limitStr string
	if limit > 0 {
		limitStr = fmt.Sprintf(" LIMIT %d", limit)
	}
	it, errFn := iterStructs[indirectFI](s.stmt(`
		SELECT fi.fiprotobuf, bl.blprotobuf FROM fileinfos fi
		INNER JOIN files f on fi.sequence = f.sequence
		LEFT JOIN blocklists bl ON bl.blocklist_hash = f.blocklist_hash
		INNER JOIN folders o ON o.idx = f.folder_idx
		INNER JOIN devices d ON d.idx = f.device_idx
		WHERE o.folder_id = ? AND d.device_id = ? AND f.sequence >= ?
		ORDER BY f.sequence`+limitStr).Queryx(
		folder, device.String(), startSeq))
	return itererr.Map(it, errFn, indirectFI.FileInfo)
}

func (s *DB) AllLocalFilesWithPrefix(folder string, device protocol.DeviceID, prefix string) (iter.Seq[protocol.FileInfo], func() error) {
	if prefix == "" {
		return s.AllLocalFiles(folder, device)
	}

	prefix = osutil.NormalizedFilename(prefix)
	end := prefixEnd(prefix)

	it, errFn := iterStructs[indirectFI](s.sql.Queryx(`
		SELECT fi.fiprotobuf, bl.blprotobuf FROM fileinfos fi
		INNER JOIN files f on fi.sequence = f.sequence
		LEFT JOIN blocklists bl ON bl.blocklist_hash = f.blocklist_hash
		INNER JOIN folders o ON o.idx = f.folder_idx
		INNER JOIN devices d ON d.idx = f.device_idx
		WHERE o.folder_id = ? AND d.device_id = ? AND f.name >= ? AND f.name < ?
	`, folder, device.String(), prefix, end))
	return itererr.Map(it, errFn, indirectFI.FileInfo)
}

func (s *DB) AllLocalFilesWithBlocksHash(folder string, h []byte) (iter.Seq[db.FileMetadata], func() error) {
	return iterStructs[db.FileMetadata](s.stmt(`
		SELECT f.sequence, f.name, f.type, f.modified as modnanos, f.size, f.deleted, f.invalid, f.local_flags as localflags FROM files f
		INNER JOIN folders o ON o.idx = f.folder_idx
		WHERE o.folder_id = ? AND f.device_idx = {{.LocalDeviceIdx}} AND f.blocklist_hash = ?
	`).Queryx(folder, h))
}

func (s *DB) AllLocalFilesWithBlocksHashAnyFolder(h []byte) (iter.Seq2[string, db.FileMetadata], func() error) {
	type row struct {
		FolderID string `db:"folder_id"`
		db.FileMetadata
	}
	it, errFn := iterStructs[row](s.stmt(`
		SELECT o.folder_id, f.sequence, f.name, f.type, f.modified as modnanos, f.size, f.deleted, f.invalid, f.local_flags as localflags FROM files f
		INNER JOIN folders o ON o.idx = f.folder_idx
		WHERE f.device_idx = {{.LocalDeviceIdx}} AND f.blocklist_hash = ?
	`).Queryx(h))
	return itererr.Map2(it, errFn, func(r row) (string, db.FileMetadata, error) {
		return r.FolderID, r.FileMetadata, nil
	})
}

func (s *DB) AllLocalBlocksWithHash(hash []byte) (iter.Seq[db.BlockMapEntry], func() error) {
	// We involve the files table in this select because deletion of blocks
	// & blocklists is deferred (garbage collected) while the files list is
	// not. This filters out blocks that are in fact deleted.
	return iterStructs[db.BlockMapEntry](s.stmt(`
		SELECT f.blocklist_hash as blocklisthash, b.idx as blockindex, b.offset, b.size FROM files f
		LEFT JOIN blocks b ON f.blocklist_hash = b.blocklist_hash
		WHERE f.device_idx = {{.LocalDeviceIdx}} AND b.hash = ?
	`).Queryx(hash))
}
