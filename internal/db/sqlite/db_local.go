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
	err := s.sql.Get(&ind, `
		SELECT fi.fiprotobuf, bl.blprotobuf FROM fileinfos fi
		INNER JOIN files f on fi.sequence = f.sequence
		LEFT JOIN blocklists bl ON bl.blocklist_hash = f.blocklist_hash
		INNER JOIN devices d ON f.device_idx = d.idx
		INNER JOIN folders o ON f.folder_idx = o.idx
		WHERE o.folder_id = ? AND d.device_id = ? AND f.name = ?
	`, folder, device.String(), file)
	if errors.Is(err, sql.ErrNoRows) {
		return protocol.FileInfo{}, false, nil
	}
	if err != nil {
		return protocol.FileInfo{}, false, wrap("local", err)
	}
	fi, err := ind.FileInfo()
	if err != nil {
		return protocol.FileInfo{}, false, wrap("local", err)
	}
	return fi, true, nil
}

func (s *DB) GetDeviceSequence(folder string, device protocol.DeviceID) (int64, error) {
	field := "sequence"
	if device != protocol.LocalDeviceID {
		field = "remote_sequence"
	}

	var res sql.NullInt64
	err := s.sql.Get(&res, fmt.Sprintf(`
		SELECT MAX(f.%s) FROM files f
		INNER JOIN folders o ON o.idx = f.folder_idx
		INNER JOIN devices d ON d.idx = f.device_idx
		WHERE o.folder_id = ? AND d.device_id = ?
	`, field), folder, device.String())
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, wrap("sequence", err)
	}
	if !res.Valid {
		return 0, nil
	}
	return res.Int64, nil
}

func (s *DB) AllLocalFiles(folder string, device protocol.DeviceID) (iter.Seq[protocol.FileInfo], func() error) {
	it, errFn := iterStructs[indirectFI](s.sql.Queryx(`
		SELECT fi.fiprotobuf, bl.blprotobuf FROM fileinfos fi
		INNER JOIN files f on fi.sequence = f.sequence
		LEFT JOIN blocklists bl ON bl.blocklist_hash = f.blocklist_hash
		INNER JOIN folders o ON o.idx = f.folder_idx
		INNER JOIN devices d ON d.idx = f.device_idx
		WHERE o.folder_id = ? AND d.device_id = ?
	`, folder, device.String()))
	return itererr.Map(it, errFn, indirectFI.FileInfo)
}

func (s *DB) AllLocalFilesBySequence(folder string, device protocol.DeviceID, startSeq int64, limit int) (iter.Seq[protocol.FileInfo], func() error) {
	var limitStr string
	if limit > 0 {
		limitStr = fmt.Sprintf(" LIMIT %d", limit)
	}
	it, errFn := iterStructs[indirectFI](s.sql.Queryx(`
		SELECT fi.fiprotobuf, bl.blprotobuf FROM fileinfos fi
		INNER JOIN files f on fi.sequence = f.sequence
		LEFT JOIN blocklists bl ON bl.blocklist_hash = f.blocklist_hash
		INNER JOIN folders o ON o.idx = f.folder_idx
		INNER JOIN devices d ON d.idx = f.device_idx
		WHERE o.folder_id = ? AND d.device_id = ? AND f.sequence >= ?
		ORDER BY f.sequence`+limitStr,
		folder, device.String(), startSeq))
	return itererr.Map(it, errFn, indirectFI.FileInfo)
}

func (s *DB) AllLocalFilesWithPrefix(folder string, device protocol.DeviceID, prefix string) (iter.Seq[protocol.FileInfo], func() error) {
	if prefix == "" {
		return s.AllLocalFiles(folder, device)
	}

	prefix = osutil.NormalizedFilename(prefix)
	pattern := prefix + "%"

	it, errFn := iterStructs[indirectFI](s.sql.Queryx(`
		SELECT fi.fiprotobuf, bl.blprotobuf FROM fileinfos fi
		INNER JOIN files f on fi.sequence = f.sequence
		LEFT JOIN blocklists bl ON bl.blocklist_hash = f.blocklist_hash
		INNER JOIN folders o ON o.idx = f.folder_idx
		INNER JOIN devices d ON d.idx = f.device_idx
		WHERE o.folder_id = ? AND d.device_id = ? AND (f.name = ? OR f.name LIKE ?)
	`, folder, device.String(), prefix, pattern))
	return itererr.Map(it, errFn, indirectFI.FileInfo)
}

func (s *DB) AllLocalFilesWithBlocksHash(folder string, h []byte) (iter.Seq[protocol.FileInfo], func() error) {
	it, errFn := iterStructs[indirectFI](s.sql.Queryx(s.tpl(`
		SELECT fi.fiprotobuf, bl.blprotobuf FROM fileinfos fi
		INNER JOIN files f on fi.sequence = f.sequence
		LEFT JOIN blocklists bl ON bl.blocklist_hash = f.blocklist_hash
		INNER JOIN folders o ON o.idx = f.folder_idx
		WHERE o.folder_id = ? AND f.device_idx = {{.LocalDeviceIdx}} AND f.blocklist_hash = ?
	`), folder, h))
	return itererr.Map(it, errFn, indirectFI.FileInfo)
}

func (s *DB) AllLocalFilesWithBlocksHashAnyFolder(h []byte) (iter.Seq2[string, protocol.FileInfo], func() error) {
	type row struct {
		FolderID   string `db:"folder_id"`
		FiProtobuf []byte
		BlProtobuf []byte
	}
	rows, err := s.sql.Queryx(s.tpl(`
		SELECT o.folder_id, fi.fiprotobuf, bl.blprotobuf FROM fileinfos fi
		INNER JOIN files f on fi.sequence = f.sequence
		INNER JOIN blocklists bl ON bl.blocklist_hash = f.blocklist_hash
		INNER JOIN folders o ON o.idx = f.folder_idx
		WHERE f.device_idx = {{.LocalDeviceIdx}} AND f.blocklist_hash = ?
	`), h)
	items, errFn := iterStructs[row](rows, err)
	var retErr error
	errFn2 := func() error {
		if err := errFn(); err != nil {
			return err
		}
		return retErr
	}
	return func(yield func(string, protocol.FileInfo) bool) {
		for r := range items {
			fi, err := indirectFI{FiProtobuf: r.FiProtobuf, BlProtobuf: r.BlProtobuf}.FileInfo()
			if err != nil {
				retErr = err
				return
			}
			if !yield(r.FolderID, fi) {
				return
			}
		}
	}, errFn2
}

func (s *DB) AllLocalBlocksWithHash(hash []byte) (iter.Seq[db.BlockMapEntry], func() error) {
	// We involve the files table in this select because deletion of blocks
	// & blocklists is deferred (gabrage collected) while the files list is
	// not. This filters out blocks that are in fact deleted.
	return iterStructs[db.BlockMapEntry](s.sql.Queryx(s.tpl(`
		SELECT f.blocklist_hash as blocklisthash, b.idx as blockindex, b.offset, b.size FROM files f
		LEFT JOIN blocks b ON f.blocklist_hash = b.blocklist_hash
		WHERE f.device_idx = {{.LocalDeviceIdx}} AND b.hash = ?
	`), hash))
}
