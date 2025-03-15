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

func (s *DB) GetGlobalFile(folder string, file string) (protocol.FileInfo, bool, error) {
	file = osutil.NormalizedFilename(file)

	var ind indirectFI
	err := s.sql.Get(&ind, s.tpl(`
		SELECT fi.fiprotobuf, bl.blprotobuf FROM fileinfos fi
		INNER JOIN files f on fi.sequence = f.sequence
		LEFT JOIN blocklists bl ON bl.blocklist_hash = f.blocklist_hash
		INNER JOIN folders o ON o.idx = f.folder_idx
		WHERE o.folder_id = ? AND f.name = ? AND f.local_flags & {{.FlagLocalGlobal}} != 0
	`), folder, file)
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

func (s *DB) GetGlobalAvailability(folder, file string) ([]protocol.DeviceID, error) {
	file = osutil.NormalizedFilename(file)

	var devStrs []string
	err := s.sql.Select(&devStrs, s.tpl(`
		SELECT d.device_id FROM files f
		INNER JOIN devices d ON d.idx = f.device_idx
		INNER JOIN folders o ON o.idx = f.folder_idx
		INNER JOIN files g ON f.folder_idx = g.folder_idx AND g.version = f.version AND g.name = f.name
		WHERE o.folder_id = ? AND g.name = ? AND g.local_flags & {{.FlagLocalGlobal}} != 0 AND f.device_idx != {{.LocalDeviceIdx}}
		ORDER BY d.device_id
	`), folder, file)
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

// type FileMetadata struct {
// 	Sequence      int64
// 	Name          string
// 	Type          protocol.FileInfoType
// 	ModifiedNanos int64
// 	Size          int64
// 	Deleted       bool
// 	Invalid       bool
// 	LocalFlags    int
// }

func (s *DB) AllGlobalFiles(folder string) (iter.Seq[db.FileMetadata], func() error) {
	it, errFn := iterStructs[db.FileMetadata](s.sql.Queryx(s.tpl(`
		SELECT f.sequence, f.name, f.type, f.modified as modifiednanos, f.size, f.deleted, f.invalid, f.local_flags as localflags FROM files f
		INNER JOIN folders o ON o.idx = f.folder_idx
		WHERE o.folder_id = ? AND f.local_flags & {{.FlagLocalGlobal}} != 0
		ORDER BY f.name
	`), folder))
	return itererr.Map(it, errFn, func(m db.FileMetadata) (db.FileMetadata, error) {
		m.Name = osutil.NativeFilename(m.Name)
		return m, nil
	})
}

func (s *DB) AllGlobalFilesPrefix(folder string, prefix string) (iter.Seq[db.FileMetadata], func() error) {
	if prefix == "" {
		return s.AllGlobalFiles(folder)
	}

	prefix = osutil.NormalizedFilename(prefix)
	pattern := prefix + "%"

	it, errFn := iterStructs[db.FileMetadata](s.sql.Queryx(s.tpl(`
		SELECT f.sequence, f.name, f.type, f.modified as modifiednanos, f.size, f.deleted, f.invalid, f.local_flags as localflags FROM files f
		INNER JOIN folders o ON o.idx = f.folder_idx
		WHERE o.folder_id = ? AND (f.name = ? OR f.name LIKE ?) AND f.local_flags & {{.FlagLocalGlobal}} != 0
		ORDER BY f.name
	`), folder, prefix, pattern))
	return itererr.Map(it, errFn, func(m db.FileMetadata) (db.FileMetadata, error) {
		m.Name = osutil.NativeFilename(m.Name)
		return m, nil
	})
}

func (s *DB) AllNeededGlobalFiles(folder string, device protocol.DeviceID, order config.PullOrder, limit, offset int) (iter.Seq[protocol.FileInfo], func() error) {
	var selectOpts string
	switch order {
	case config.PullOrderRandom:
		selectOpts = "ORDER BY RANDOM()"
	case config.PullOrderAlphabetic:
		selectOpts = "ORDER BY g.name ASC"
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
		// Select all the non-ignored files with the need bit set.
		it, errFn := iterStructs[indirectFI](s.sql.Queryx(s.tpl(`
			SELECT fi.fiprotobuf, bl.blprotobuf, g.name, g.size, g.modified FROM fileinfos fi
			INNER JOIN files g on fi.sequence = g.sequence
			LEFT JOIN blocklists bl ON bl.blocklist_hash = g.blocklist_hash
			INNER JOIN folders o ON o.idx = g.folder_idx
			WHERE o.folder_id = ? AND g.local_flags & {{.FlagLocalIgnored}} = 0 AND g.local_flags & {{.FlagLocalNeeded}} != 0
		`)+selectOpts, folder))
		return itererr.Map(it, errFn, indirectFI.FileInfo)
	}

	// Select:
	//
	// - all the valid, non-deleted global files that don't have a corresponding
	//   remote file with the same version.
	//
	// - all the valid, deleted global files that have a corresponding non-deleted
	//   remote file (of any version)

	it, errFn := iterStructs[indirectFI](s.sql.Queryx(s.tpl(`
		SELECT fi.fiprotobuf, bl.blprotobuf, g.name, g.size, g.modified FROM fileinfos fi
		INNER JOIN files g on fi.sequence = g.sequence
		LEFT JOIN blocklists bl ON bl.blocklist_hash = g.blocklist_hash
		INNER JOIN folders o ON o.idx = g.folder_idx
		WHERE o.folder_id = ? AND g.local_flags & {{.FlagLocalGlobal}} != 0 AND NOT g.deleted AND NOT g.invalid AND NOT EXISTS (
			SELECT 1 FROM FILES f
			INNER JOIN devices d ON d.idx = f.device_idx
			WHERE f.name = g.name AND f.version = g.version AND f.folder_idx = g.folder_idx AND d.device_id = ?
		)

		UNION

		SELECT fi.fiprotobuf, bl.blprotobuf, g.name, g.size, g.modified FROM fileinfos fi
		INNER JOIN files g on fi.sequence = g.sequence
		LEFT JOIN blocklists bl ON bl.blocklist_hash = g.blocklist_hash
		INNER JOIN folders o ON o.idx = g.folder_idx
		WHERE o.folder_id = ? AND g.local_flags & {{.FlagLocalGlobal}} != 0 AND g.deleted AND NOT g.invalid AND EXISTS (
			SELECT 1 FROM FILES f
			INNER JOIN devices d ON d.idx = f.device_idx
			WHERE f.name = g.name AND f.folder_idx = g.folder_idx AND d.device_id = ? AND NOT f.deleted
		)
	`)+selectOpts,
		folder, device.String(),
		folder, device.String(),
	))
	return itererr.Map(it, errFn, indirectFI.FileInfo)
}
