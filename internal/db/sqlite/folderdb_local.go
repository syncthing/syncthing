// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package sqlite

import (
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"iter"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/syncthing/syncthing/internal/db"
	"github.com/syncthing/syncthing/internal/itererr"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/protocol"
)

func (s *folderDB) GetDeviceFile(device protocol.DeviceID, file string) (protocol.FileInfo, bool, error) {
	file = osutil.NormalizedFilename(file)

	var ind indirectFI
	err := s.stmt(`
		SELECT fi.fiprotobuf, bl.blprotobuf FROM fileinfos fi
		INNER JOIN files f on fi.sequence = f.sequence
		LEFT JOIN blocklists bl ON bl.blocklist_hash = f.blocklist_hash
		INNER JOIN devices d ON f.device_idx = d.idx
		INNER JOIN file_names n ON f.name_idx = n.idx
		WHERE d.device_id = ? AND n.name = ?
	`).Get(&ind, device.String(), file)
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

func (s *folderDB) AllLocalFiles(device protocol.DeviceID) (iter.Seq[protocol.FileInfo], func() error) {
	it, errFn := iterStructs[indirectFI](s.stmt(`
		SELECT fi.fiprotobuf, bl.blprotobuf FROM fileinfos fi
		INNER JOIN files f on fi.sequence = f.sequence
		LEFT JOIN blocklists bl ON bl.blocklist_hash = f.blocklist_hash
		INNER JOIN devices d ON d.idx = f.device_idx
		WHERE d.device_id = ?
	`).Queryx(device.String()))
	return itererr.Map(it, errFn, indirectFI.FileInfo)
}

func (s *folderDB) AllLocalFilesBySequence(device protocol.DeviceID, startSeq int64, limit int) (iter.Seq[protocol.FileInfo], func() error) {
	var limitStr string
	if limit > 0 {
		limitStr = fmt.Sprintf(" LIMIT %d", limit)
	}
	it, errFn := iterStructs[indirectFI](s.stmt(`
		SELECT fi.fiprotobuf, bl.blprotobuf FROM fileinfos fi
		INNER JOIN files f on fi.sequence = f.sequence
		LEFT JOIN blocklists bl ON bl.blocklist_hash = f.blocklist_hash
		INNER JOIN devices d ON d.idx = f.device_idx
		WHERE d.device_id = ? AND f.sequence >= ?
		ORDER BY f.sequence`+limitStr).Queryx(
		device.String(), startSeq))
	return itererr.Map(it, errFn, indirectFI.FileInfo)
}

func (s *folderDB) AllLocalFilesWithPrefix(device protocol.DeviceID, prefix string) (iter.Seq[protocol.FileInfo], func() error) {
	if prefix == "" {
		return s.AllLocalFiles(device)
	}

	prefix = osutil.NormalizedFilename(prefix)
	end := prefixEnd(prefix)

	it, errFn := iterStructs[indirectFI](s.sql.Queryx(`
		SELECT fi.fiprotobuf, bl.blprotobuf FROM fileinfos fi
		INNER JOIN files f on fi.sequence = f.sequence
		LEFT JOIN blocklists bl ON bl.blocklist_hash = f.blocklist_hash
		INNER JOIN devices d ON d.idx = f.device_idx
		INNER JOIN file_names n ON f.name_idx = n.idx
		WHERE d.device_id = ? AND n.name >= ? AND n.name < ?
	`, device.String(), prefix, end))
	return itererr.Map(it, errFn, indirectFI.FileInfo)
}

func (s *folderDB) AllLocalFilesWithBlocksHash(h []byte) (iter.Seq[db.FileMetadata], func() error) {
	return iterStructs[db.FileMetadata](s.stmt(`
		SELECT f.sequence, n.name, f.type, f.modified as modnanos, f.size, f.deleted, f.local_flags as localflags FROM files f
		INNER JOIN file_names n ON f.name_idx = n.idx
		WHERE f.device_idx = {{.LocalDeviceIdx}} AND f.blocklist_hash = ?
	`).Queryx(h))
}

func (s *folderDB) AllLocalBlocksWithHash(hash []byte) (iter.Seq[db.BlockMapEntry], func() error) {
	// We involve the files table in this select because deletion of blocks
	// & blocklists is deferred (garbage collected) while the files list is
	// not. This filters out blocks that are in fact deleted.
	return iterStructs[db.BlockMapEntry](s.stmt(`
		SELECT f.blocklist_hash as blocklisthash, b.idx as blockindex, b.offset, b.size, n.name as filename FROM files f
		INNER JOIN file_names n ON f.name_idx = n.idx
		LEFT JOIN blocks b ON f.blocklist_hash = b.blocklist_hash
		WHERE f.device_idx = {{.LocalDeviceIdx}} AND b.hash = ?
	`).Queryx(hash))
}

func (s *folderDB) ListDevicesForFolder() ([]protocol.DeviceID, error) {
	var res []string
	err := s.stmt(`
		SELECT DISTINCT d.device_id FROM counts s
		INNER JOIN devices d ON d.idx = s.device_idx
		WHERE s.count > 0 AND s.device_idx != {{.LocalDeviceIdx}}
		ORDER BY d.device_id
	`).Select(&res)
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

func (s *folderDB) DebugCounts(out io.Writer) error {
	type deviceCountsRow struct {
		countsRow

		DeviceID string
	}

	delMap := map[bool]string{
		true:  "del",
		false: "---",
	}

	var res []deviceCountsRow
	if err := s.stmt(`
		SELECT d.device_id as deviceid, s.type, s.count, s.size, s.local_flags, s.deleted FROM counts s
		INNER JOIN devices d ON d.idx = s.device_idx
	`).Select(&res); err != nil {
		return wrap(err)
	}

	tw := tabwriter.NewWriter(out, 2, 2, 2, ' ', 0)
	fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", "DEVICE", "TYPE", "FLAGS", "DELETED", "COUNT", "SIZE")
	for _, row := range res {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\t%d\n", shortDevice(row.DeviceID), shortType(row.Type), row.LocalFlags.HumanString(), delMap[row.Deleted], row.Count, row.Size)
	}
	return tw.Flush()
}

func (s *folderDB) DebugFilePattern(out io.Writer, name string) error {
	type hashFileMetadata struct {
		db.FileMetadata

		Version       dbVector
		BlocklistHash []byte
		DeviceID      string
	}
	name = "%" + name + "%"
	res := itererr.Zip(iterStructs[hashFileMetadata](s.stmt(`
		SELECT f.sequence, n.name, f.type, f.modified as modnanos, f.size, f.deleted, f.local_flags as localflags, v.version, f.blocklist_hash as blocklisthash, d.device_id as deviceid FROM files f
		INNER JOIN devices d ON d.idx = f.device_idx
		INNER JOIN file_names n ON n.idx = f.name_idx
		INNER JOIN file_versions v ON v.idx = f.version_idx
		WHERE n.name LIKE ?
		ORDER BY n.name, f.device_idx
	`).Queryx(name)))

	delMap := map[bool]string{
		true:  "del",
		false: "---",
	}

	tw := tabwriter.NewWriter(out, 2, 2, 2, ' ', 0)
	fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", "DEVICE", "TYPE", "NAME", "SEQUENCE", "DELETED", "MODIFIED", "SIZE", "FLAGS", "VERSION", "BLOCKLIST")
	for row, err := range res {
		if err != nil {
			return err
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\t%s\t%d\t%s\t%s\t%s\n", shortDevice(row.DeviceID), shortType(row.Type), row.Name, row.Sequence, delMap[row.Deleted], row.ModTime().UTC().Format(time.RFC3339Nano), row.Size, row.LocalFlags.HumanString(), row.Version.HumanString(), shortHash(row.BlocklistHash))
	}
	return tw.Flush()
}

func shortDevice(s string) string {
	if dev, err := protocol.DeviceIDFromString(s); err == nil && dev == protocol.LocalDeviceID {
		return "-local-"
	}
	short, _, _ := strings.Cut(s, "-")
	return short
}

func shortType(t protocol.FileInfoType) string {
	return strings.TrimPrefix(t.String(), "FILE_INFO_TYPE_")
}

func shortHash(bs []byte) string {
	if len(bs) == 0 {
		return "-nil-"
	}
	return base64.RawStdEncoding.EncodeToString(bs)[:8]
}
