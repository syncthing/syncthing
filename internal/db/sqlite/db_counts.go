package sqlite

import (
	"github.com/syncthing/syncthing/internal/db"
	"github.com/syncthing/syncthing/lib/protocol"
)

type countsRow struct {
	Type    protocol.FileInfoType
	Count   int
	Size    int64
	FlagBit int64 `db:"local_flags"`
}

func (s *DB) CountLocal(folder string, device protocol.DeviceID) (db.Counts, error) {
	var res []countsRow
	if err := s.sql.Select(&res, s.tpl(`
		SELECT s.type, s.count, s.size, s.local_flags FROM counts s
		INNER JOIN folders o ON o.idx = s.folder_idx
		INNER JOIN devices d ON d.idx = s.device_idx
		WHERE o.folder_id = ? AND d.device_id = ? AND s.local_flags & {{.FlagLocalIgnored}} = 0
	`), folder, device.String()); err != nil {
		return db.Counts{}, err
	}
	return summarizeCounts(res), nil
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
	var res []countsRow
	err := s.sql.Select(&res, s.tpl(`
		SELECT s.type, s.count, s.size, s.local_flags FROM counts s
		INNER JOIN folders o ON o.idx = s.folder_idx
		WHERE o.folder_id = ? AND s.local_flags & {{.FlagLocalGlobal}} != 0 AND s.local_flags & {{or .FlagLocalReceiveOnly .FlagLocalIgnored}} = 0
	`), folder)
	if err != nil {
		return db.Counts{}, err
	}
	return summarizeCounts(res), nil
}

func (s *DB) CountReceiveOnlyChanged(folder string) (db.Counts, error) {
	var res []countsRow
	err := s.sql.Select(&res, s.tpl(`
		SELECT s.type, s.count, s.size, s.local_flags FROM counts s
		INNER JOIN folders o ON o.idx = s.folder_idx
		WHERE o.folder_id = ? AND local_flags & {{.FlagLocalReceiveOnly}} != 0
	`), folder)
	if err != nil {
		return db.Counts{}, err
	}
	return summarizeCounts(res), nil
}

func (s *DB) needSizeLocal(folder string) (db.Counts, error) {
	// The need size for the local device is the sum of entries with the
	// need bit set.
	var res []countsRow
	err := s.sql.Select(&res, s.tpl(`
		SELECT s.type, s.count, s.size, s.local_flags FROM counts s
		INNER JOIN folders o ON o.idx = s.folder_idx
		WHERE o.folder_id = ? AND s.local_flags & {{.FlagLocalNeeded}} != 0
	`), folder)
	if err != nil {
		return db.Counts{}, err
	}
	return summarizeCounts(res), nil
}

func (s *DB) needSizeRemote(folder string, device protocol.DeviceID) (db.Counts, error) {
	var res []countsRow
	// See AllNeededNames for commentary as that is the same query without summing
	if err := s.sql.Select(&res, s.tpl(`
		SELECT g.type, count(*) as count, sum(g.size) as size, g.local_flags FROM files g
		INNER JOIN folders o ON o.idx = g.folder_idx
		WHERE o.folder_id = ? AND g.local_flags & {{.FlagLocalGlobal}} != 0 AND NOT g.deleted AND NOT g.invalid AND NOT EXISTS (
			SELECT 1 FROM FILES f
			INNER JOIN devices d ON d.idx = f.device_idx
			WHERE f.name = g.name AND f.version = g.version AND f.folder_idx = g.folder_idx AND d.device_id = ?
		)
		GROUP BY g.type, g.local_flags

		UNION

		SELECT g.type, count(*) as count, sum(g.size) as size, g.local_flags FROM files g
		INNER JOIN folders o ON o.idx = g.folder_idx
		WHERE o.folder_id = ? AND g.local_flags & {{.FlagLocalGlobal}} != 0 AND g.deleted AND NOT g.invalid AND EXISTS (
			SELECT 1 FROM FILES f
			INNER JOIN devices d ON d.idx = f.device_idx
			WHERE f.name = g.name AND f.folder_idx = g.folder_idx AND d.device_id = ? AND NOT f.deleted
		)
		GROUP BY g.type, g.local_flags
	`), folder, device.String(),
		folder, device.String()); err != nil {
		return db.Counts{}, err
	}

	return summarizeCounts(res), nil
}

func summarizeCounts(res []countsRow) db.Counts {
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
