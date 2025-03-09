package sqlite

import (
	"fmt"

	"github.com/syncthing/syncthing/internal/db"
	"github.com/syncthing/syncthing/lib/protocol"
)

type sizesRow struct {
	Type    protocol.FileInfoType
	Count   int
	Size    int64
	FlagBit int64 `db:"local_flags"`
}

func (s *DB) CountLocal(folder string, device protocol.DeviceID) (db.Counts, error) {
	var res []sizesRow
	extra := ""
	if device == protocol.LocalDeviceID {
		// The size counters for the local device are special, in that we
		// synthetise entries with both the Global and Need flag for files
		// that we don't currently have. We need to exlude those from the
		// local size sum.
		extra = fmt.Sprintf(" AND local_flags & %[1]d != %[1]d", protocol.FlagLocalGlobal|protocol.FlagLocalNeeded)
	}
	if err := s.sql.Select(&res, `
		SELECT s.type, s.count, s.size, s.local_flags FROM sizes s
		INNER JOIN folders o ON o.idx = s.folder_idx
		INNER JOIN devices d ON d.idx = s.device_idx
		WHERE o.folder_id = ? AND d.device_id = ? AND s.local_flags & ? = 0`+extra,
		folder, device.String(), protocol.FlagLocalIgnored); err != nil {
		return db.Counts{}, err
	}
	return summarizeRows(res), nil
}

func (s *DB) CountNeed(folder string, device protocol.DeviceID) (db.Counts, error) {
	if device == protocol.LocalDeviceID {
		return s.needSizeLocal(folder)
	}
	return s.needSizeRemote(folder, device)
}

func (s *DB) CountGlobal(folder string) (db.Counts, error) {
	// Exclude ignored and receive-only changed files from the global count
	// (legacy expectation? it's a bit weird since those files can in fact
	// be global and you can get them with GetGlobal etc.)
	var res []sizesRow
	err := s.sql.Select(&res, `
		SELECT s.type, s.count, s.size, s.local_flags FROM sizes s
		INNER JOIN folders o ON o.idx = s.folder_idx
		WHERE o.folder_id = ? AND s.local_flags & ? != 0 AND s.local_flags & ? = 0
	`, folder, protocol.FlagLocalGlobal, protocol.FlagLocalReceiveOnly|protocol.FlagLocalIgnored)
	if err != nil {
		return db.Counts{}, err
	}
	return summarizeRows(res), nil
}

func (s *DB) CountReceiveOnlyChanged(folder string) (db.Counts, error) {
	var res []sizesRow
	err := s.sql.Select(&res, `
		SELECT s.type, s.count, s.size, s.local_flags FROM sizes s
		INNER JOIN folders o ON o.idx = s.folder_idx
		WHERE o.folder_id = ? AND local_flags & ? != 0
	`, folder, protocol.FlagLocalReceiveOnly)
	if err != nil {
		return db.Counts{}, err
	}
	return summarizeRows(res), nil
}

func (s *DB) needSizeLocal(folder string) (db.Counts, error) {
	// The need size for the local device is the sum of entries with both
	// the global and need bit set.
	var res []sizesRow
	err := s.sql.Select(&res, `
		SELECT s.type, s.count, s.size, s.local_flags FROM sizes s
		INNER JOIN folders o ON o.idx = s.folder_idx
		WHERE o.folder_id = ? AND s.local_flags & ? = ?
	`, folder, protocol.FlagLocalNeeded|protocol.FlagLocalGlobal, protocol.FlagLocalNeeded|protocol.FlagLocalGlobal)
	if err != nil {
		return db.Counts{}, err
	}
	return summarizeRows(res), nil
}

func (s *DB) needSizeRemote(folder string, device protocol.DeviceID) (db.Counts, error) {
	var res []sizesRow
	// See AllNeededNames for commentary as that is the same query without summing
	if err := s.sql.Select(&res, `
	SELECT g.type, count(*) as count, sum(g.size) as size, g.local_flags FROM files g
	INNER JOIN folders o ON o.idx = g.folder_idx
	WHERE o.folder_id = ? AND g.local_flags & ? != 0 AND NOT g.deleted AND NOT g.invalid AND NOT EXISTS (
		SELECT 1 FROM FILES f
		INNER JOIN devices d ON d.idx = f.device_idx
		WHERE f.name = g.name AND f.version = g.version AND f.folder_idx = g.folder_idx AND d.device_id = ?
	)
	GROUP BY g.type, g.local_flags

	UNION

	SELECT g.type, count(*) as count, sum(g.size) as size, g.local_flags FROM files g
	INNER JOIN folders o ON o.idx = g.folder_idx
	WHERE o.folder_id = ? AND g.local_flags & ? != 0 AND g.deleted AND NOT g.invalid AND EXISTS (
		SELECT 1 FROM FILES f
		INNER JOIN devices d ON d.idx = f.device_idx
		WHERE f.name = g.name AND f.folder_idx = g.folder_idx AND d.device_id = ? AND NOT f.deleted
	)
	GROUP BY g.type, g.local_flags`,
		folder, protocol.FlagLocalGlobal, device.String(),
		folder, protocol.FlagLocalGlobal, device.String()); err != nil {
		return db.Counts{}, err
	}

	return summarizeRows(res), nil
}

func summarizeRows(res []sizesRow) db.Counts {
	c := db.Counts{
		DeviceID: protocol.LocalDeviceID,
	}
	for _, r := range res {
		switch {
		case r.FlagBit&protocol.FlagLocalDeleted != 0:
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
