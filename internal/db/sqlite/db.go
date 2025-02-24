package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"iter"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/syncthing/syncthing/internal/db"
	"github.com/syncthing/syncthing/internal/gen/dbproto"
	"github.com/syncthing/syncthing/internal/itererr"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/sliceutil"
	"google.golang.org/protobuf/proto"
)

type DB struct {
	sql            *sqlx.DB
	localDeviceIdx int64
	updateLock     sync.Mutex
	prepared       map[string]*sqlx.Stmt
}

func (s *DB) Close() error {
	s.updateLock.Lock()
	defer s.updateLock.Unlock()
	return wrap("close", s.sql.Close())
}

func (s *DB) Update(folder string, device protocol.DeviceID, fs []protocol.FileInfo) error {
	t0 := time.Now()

	s.updateLock.Lock()
	defer s.updateLock.Unlock()

	t1 := time.Now()
	defer func() {
		d := time.Since(t1)
		fmt.Printf("Update(%s, %v, %d files) took %v, delayed %v, %.01f/s\n", folder, device, len(fs), d, t1.Sub(t0), float64(len(fs))/d.Seconds())
	}()

	tx, err := s.sql.BeginTxx(context.Background(), nil)
	if err != nil {
		return wrap("update", err)
	}
	defer tx.Rollback() //nolint:errcheck
	txp := &txPreparedStmts{Tx: tx}

	folderIdx, err := s.folderIdxLocked(folder)
	if err != nil {
		return wrap("update", err)
	}
	deviceIdx, err := s.deviceIdxLocked(device)
	if err != nil {
		return wrap("update", err)
	}

	insertFileStmt, err := txp.Preparex(`
		INSERT OR REPLACE INTO files (folder_idx, device_idx, remote_sequence, name, type, modified, size, version, deleted, invalid, local_flags, blocklist_hash)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		RETURNING sequence`)
	if err != nil {
		return wrap("update", err)
	}

	insertFileInfoStmt, err := txp.Preparex(`
		INSERT INTO fileinfos (sequence, fiprotobuf)
		VALUES (?, ?)`)
	if err != nil {
		return wrap("update", err)
	}

	insertBlockListStmt, err := txp.Preparex(`
		INSERT INTO blocklists (blocklist_hash, refcount, blprotobuf)
		VALUES (?, 1, ?)
		ON CONFLICT DO UPDATE SET refcount = refcount + 1`)
	if err != nil {
		return wrap("update", err)
	}

	var prevRemoteSeq int64
	for i, f := range fs {
		f.Name = osutil.NormalizedFilename(f.Name)

		var blockshash *[]byte
		if len(f.Blocks) > 0 {
			f.BlocksHash = protocol.BlocksHash(f.Blocks)
			blockshash = &f.BlocksHash
		} else {
			f.BlocksHash = nil
		}

		if f.Deleted {
			f.LocalFlags |= protocol.FlagLocalDeleted
		}

		if f.Type == protocol.FileInfoTypeDirectory {
			f.Size = 128 // synthetic directory size
		}

		// Insert the file.
		//
		// If it is a remote file, set remote_sequence otherwise leave it at
		// null. Returns the new local sequence.
		var remoteSeq *int64
		if device != protocol.LocalDeviceID {
			if i > 0 && f.Sequence == prevRemoteSeq {
				return fmt.Errorf("duplicate remote sequence number %d", prevRemoteSeq)
			}
			prevRemoteSeq = f.Sequence
			remoteSeq = &f.Sequence
		}
		var localSeq int64
		t0 := time.Now()
		if err := insertFileStmt.Get(&localSeq, folderIdx, deviceIdx, remoteSeq, f.Name, f.Type, f.ModTime().UnixNano(), f.Size, f.Version.String(), f.IsDeleted(), f.IsInvalid(), f.LocalFlags, blockshash); err != nil {
			return wrap("update (insert file)", err)
		}
		if d := time.Since(t0); d > 25*time.Millisecond {
			fmt.Println("insertFileStmt", i, d)
		}

		if len(f.Blocks) > 0 {
			// Indirect the block list
			blocks := sliceutil.Map(f.Blocks, protocol.BlockInfo.ToWire)
			bs, err := proto.Marshal(&dbproto.BlockList{Blocks: blocks})
			if err != nil {
				return wrap("update (marshal blocklist)", err)
			}
			t0 = time.Now()
			if _, err := insertBlockListStmt.Exec(f.BlocksHash, bs); err != nil {
				return wrap("update (insert blocklist)", err)
			}
			if d := time.Since(t0); d > 25*time.Millisecond {
				fmt.Println("insertBlockListStmt", i, d, len(f.Blocks))
			}

			if device == protocol.LocalDeviceID {
				// Update block lists
				t0 = time.Now()
				if err := s.insertBlocksLocked(txp, f.BlocksHash, f.Blocks); err != nil {
					return wrap("update", err)
				}
				if d := time.Since(t0); d > 25*time.Millisecond {
					fmt.Println("insertBlocksLocked", i, d, len(f.Blocks))
				}
			}

			f.Blocks = nil
		}

		// Insert the fileinfo
		if device == protocol.LocalDeviceID {
			f.Sequence = localSeq
		}
		bs, err := proto.Marshal(f.ToWire(true))
		if err != nil {
			return wrap("update", err)
		}
		t0 = time.Now()
		if _, err := insertFileInfoStmt.Exec(localSeq, bs); err != nil {
			return wrap("update (insert fileinfo)", err)
		}
		if d := time.Since(t0); d > 25*time.Millisecond {
			fmt.Println("insertFileInfoStmt", d)
		}

		// Update global and need
		t0 = time.Now()
		if err := s.recalcGlobalForFile(txp, folderIdx, f.Name); err != nil {
			return wrap("update", err)
		}
		if d := time.Since(t0); d > 25*time.Millisecond {
			fmt.Println("recalcGlobalForFile", d)
		}
	}

	return wrap("update", tx.Commit())
}

func (s *DB) DropFolder(folder string) error {
	s.updateLock.Lock()
	defer s.updateLock.Unlock()
	_, err := s.sql.Exec(`DELETE FROM folders WHERE folder_id = ?`, folder)
	return err
}

func (s *DB) DropDevice(device protocol.DeviceID) error {
	if device == protocol.LocalDeviceID {
		panic("bug: cannot drop local device")
	}

	s.updateLock.Lock()
	defer s.updateLock.Unlock()

	deviceIdx, err := s.deviceIdxLocked(device)
	if err != nil {
		return wrap("drop device", err)
	}

	tx, err := s.sql.BeginTxx(context.Background(), nil)
	if err != nil {
		return wrap("drop device", err)
	}
	defer tx.Rollback() //nolint:errcheck
	txp := &txPreparedStmts{Tx: tx}

	// Find all folders where the device is involved
	var folderIdxs []int64
	if err := tx.Select(&folderIdxs, `
		SELECT folder_idx
		FROM sizes
		WHERE device_idx = ? AND count > 0
		GROUP BY folder_idx`, deviceIdx); err != nil {
		return wrap("drop device", err)
	}

	// Drop the device, which cascades to delete all files etc for it
	if _, err := tx.Exec(`DELETE FROM devices WHERE device_id = ?`, device.String()); err != nil {
		return wrap("drop device", err)
	}

	// Recalc the globals for all affected folders
	for _, idx := range folderIdxs {
		if err := s.recalcGlobalForFolder(txp, idx); err != nil {
			return wrap("drop device", err)
		}
	}

	return wrap("drop device", tx.Commit())
}

func (s *DB) DropAllFiles(folder string, device protocol.DeviceID) error {
	s.updateLock.Lock()
	defer s.updateLock.Unlock()

	// This is a two part operation, first dropping all the files and then
	// recalculating the global state for the entire folder.

	folderIdx, err := s.folderIdxLocked(folder)
	if err != nil {
		return wrap("drop all files", err)
	}

	tx, err := s.sql.BeginTxx(context.Background(), nil)
	if err != nil {
		return wrap("drop all files", err)
	}
	defer tx.Rollback() //nolint:errcheck
	txp := &txPreparedStmts{Tx: tx}

	// Drop all the file entries

	result, err := tx.Exec(`
		DELETE FROM files WHERE ROWID in (
			SELECT f.ROWID FROM files f
			INNER JOIN devices d ON f.device_idx = d.idx
			WHERE f.folder_idx = ? AND d.device_id = ?
		)
	`, folderIdx, device.String())
	if err != nil {
		return wrap("drop all files", err)
	}
	if n, err := result.RowsAffected(); err == nil && n == 0 {
		// The delete affected no rows, so we don't need to redo the entire
		// global/need calculation.
		return wrap("drop all files", tx.Commit())
	}

	// Recalc global for the entire folder

	if err := s.recalcGlobalForFolder(txp, folderIdx); err != nil {
		return wrap("drop all files", err)
	}
	return wrap("drop all files", tx.Commit())
}

func (s *DB) DropFilesNamed(folder string, device protocol.DeviceID, names []string) error {
	for i := range names {
		names[i] = osutil.NormalizedFilename(names[i])
	}

	s.updateLock.Lock()
	defer s.updateLock.Unlock()

	folderIdx, err := s.folderIdxLocked(folder)
	if err != nil {
		return wrap("drop all files", err)
	}

	tx, err := s.sql.BeginTxx(context.Background(), nil)
	if err != nil {
		return wrap("drop all files", err)
	}
	defer tx.Rollback() //nolint:errcheck
	txp := &txPreparedStmts{Tx: tx}

	// Drop the named files

	query, args, err := sqlx.In(`
		DELETE FROM files WHERE ROWID in (
			SELECT f.ROWID FROM files f
			INNER JOIN devices d ON f.device_idx = d.idx
			WHERE f.folder_idx = ? AND device_id = ? AND f.name IN (?)
		)
	`, folderIdx, device.String(), names)
	if err != nil {
		return wrap("drop files named", err)
	}
	if _, err := tx.Exec(query, args...); err != nil {
		return wrap("drop files named", err)
	}

	// Recalc globals for the named files

	for _, name := range names {
		if err := s.recalcGlobalForFile(txp, folderIdx, name); err != nil {
			return wrap("drop files named", err)
		}
	}

	return wrap("drop files named", tx.Commit())
}

func (s *DB) Local(folder string, device protocol.DeviceID, file string) (protocol.FileInfo, bool, error) {
	file = osutil.NormalizedFilename(file)

	var ind indirectFI
	err := s.sql.Get(&ind, `
		SELECT fi.fiprotobuf, bl.blprotobuf FROM fileinfos fi
		INNER JOIN files f on fi.sequence = f.sequence
		LEFT JOIN blocklists bl ON bl.blocklist_hash = f.blocklist_hash
		INNER JOIN devices d ON f.device_idx = d.idx
		INNER JOIN folders o ON f.folder_idx = o.idx
		WHERE o.folder_id = ? AND d.device_id = ? AND f.name = ? AND f.version != ""`,
		folder, device.String(), file)
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

func (s *DB) Global(folder string, file string) (protocol.FileInfo, bool, error) {
	file = osutil.NormalizedFilename(file)

	var ind indirectFI
	err := s.sql.Get(&ind, `
		SELECT fi.fiprotobuf, bl.blprotobuf FROM fileinfos fi
		INNER JOIN files f on fi.sequence = f.sequence
		LEFT JOIN blocklists bl ON bl.blocklist_hash = f.blocklist_hash
		INNER JOIN folders o ON o.idx = f.folder_idx
		WHERE o.folder_id = ? AND f.name = ? AND f.local_flags & ? != 0`, folder, file, protocol.FlagLocalGlobal)
	if errors.Is(err, sql.ErrNoRows) {
		return protocol.FileInfo{}, false, nil
	}
	if err != nil {
		return protocol.FileInfo{}, false, wrap("global", err)
	}
	fi, err := ind.FileInfo()
	if err != nil {
		return protocol.FileInfo{}, false, wrap("local", err)
	}
	return fi, true, nil
}

func (s *DB) AllGlobal(folder string) iter.Seq2[protocol.FileInfo, error] {
	beps := iterStructs[indirectFI](s.sql.Queryx(`
		SELECT fi.fiprotobuf, bl.blprotobuf FROM fileinfos fi
		INNER JOIN files f on fi.sequence = f.sequence
		LEFT JOIN blocklists bl ON bl.blocklist_hash = f.blocklist_hash
		INNER JOIN folders o ON o.idx = f.folder_idx
		WHERE o.folder_id = ? AND f.local_flags & ? != 0`,
		folder, protocol.FlagLocalGlobal))
	return itererr.Map2(beps, indirectFI.FileInfo)
}

func (s *DB) AllGlobalPrefix(folder string, prefix string) iter.Seq2[protocol.FileInfo, error] {
	if prefix == "" {
		return s.AllGlobal(folder)
	}

	prefix = osutil.NormalizedFilename(prefix)
	pattern := prefix + "%"

	beps := iterStructs[indirectFI](s.sql.Queryx(`
		SELECT fi.fiprotobuf, bl.blprotobuf FROM fileinfos fi
		INNER JOIN files f on fi.sequence = f.sequence
		LEFT JOIN blocklists bl ON bl.blocklist_hash = f.blocklist_hash
		INNER JOIN folders o ON o.idx = f.folder_idx
		WHERE o.folder_id = ? AND (f.name = ? OR f.name LIKE ?) AND f.local_flags & ? != 0`,
		folder, prefix, pattern, protocol.FlagLocalGlobal))
	return itererr.Map2(beps, indirectFI.FileInfo)
}

func (s *DB) Sequence(folder string, device protocol.DeviceID) (int64, error) {
	field := "sequence"
	if device != protocol.LocalDeviceID {
		field = "remote_sequence"
	}

	var res sql.NullInt64
	err := s.sql.Get(&res, fmt.Sprintf(`
		SELECT MAX(f.%s) FROM files f
		INNER JOIN folders o ON o.idx = f.folder_idx
		INNER JOIN devices d ON d.idx = f.device_idx
		WHERE o.folder_id = ? AND d.device_id = ?`, field),
		folder, device.String())
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

func (s *DB) AllLocal(folder string, device protocol.DeviceID) iter.Seq2[protocol.FileInfo, error] {
	beps := iterStructs[indirectFI](s.sql.Queryx(`
		SELECT fi.fiprotobuf, bl.blprotobuf FROM fileinfos fi
		INNER JOIN files f on fi.sequence = f.sequence
		LEFT JOIN blocklists bl ON bl.blocklist_hash = f.blocklist_hash
		INNER JOIN folders o ON o.idx = f.folder_idx
		INNER JOIN devices d ON d.idx = f.device_idx
		WHERE o.folder_id = ? AND d.device_id = ?`,
		folder, device.String()))
	return itererr.Map2(beps, indirectFI.FileInfo)
}

func (s *DB) AllLocalSequenced(folder string, device protocol.DeviceID, startSeq int64) iter.Seq2[protocol.FileInfo, error] {
	beps := iterStructs[indirectFI](s.sql.Queryx(`
		SELECT fi.fiprotobuf, bl.blprotobuf FROM fileinfos fi
		INNER JOIN files f on fi.sequence = f.sequence
		LEFT JOIN blocklists bl ON bl.blocklist_hash = f.blocklist_hash
		INNER JOIN folders o ON o.idx = f.folder_idx
		INNER JOIN devices d ON d.idx = f.device_idx
		WHERE o.folder_id = ? AND d.device_id = ? AND f.sequence >= ?
		ORDER BY f.sequence`,
		folder, device.String(), startSeq))
	return itererr.Map2(beps, indirectFI.FileInfo)
}

func (s *DB) AllLocalPrefixed(folder string, device protocol.DeviceID, prefix string) iter.Seq2[protocol.FileInfo, error] {
	if prefix == "" {
		return s.AllLocal(folder, device)
	}

	prefix = osutil.NormalizedFilename(prefix)
	pattern := prefix + "%"

	beps := iterStructs[indirectFI](s.sql.Queryx(`
		SELECT fi.fiprotobuf, bl.blprotobuf FROM fileinfos fi
		INNER JOIN files f on fi.sequence = f.sequence
		LEFT JOIN blocklists bl ON bl.blocklist_hash = f.blocklist_hash
		INNER JOIN folders o ON o.idx = f.folder_idx
		INNER JOIN devices d ON d.idx = f.device_idx
		WHERE o.folder_id = ? AND d.device_id = ? AND (f.name = ? OR f.name LIKE ?)`,
		folder, device.String(), prefix, pattern))
	return itererr.Map2(beps, indirectFI.FileInfo)
}

func (s *DB) AllForBlocksHash(folder string, h []byte) iter.Seq2[protocol.FileInfo, error] {
	beps := iterStructs[indirectFI](s.sql.Queryx(`
		SELECT fi.fiprotobuf, bl.blprotobuf FROM fileinfos fi
		INNER JOIN files f on fi.sequence = f.sequence
		LEFT JOIN blocklists bl ON bl.blocklist_hash = f.blocklist_hash
		INNER JOIN folders o ON o.idx = f.folder_idx
		WHERE o.folder_id = ? AND f.blocklist_hash = ?`,
		folder, h))
	return itererr.Map2(beps, indirectFI.FileInfo)
}

func (s *DB) AllForBlocksHashAnyFolder(errptr *error, h []byte) iter.Seq2[string, protocol.FileInfo] {
	type row struct {
		FolderID   string `db:"folder_id"`
		FiProtobuf []byte
		BlProtobuf []byte
	}
	rows, err := s.sql.Queryx(`
		SELECT o.folder_id, fi.fiprotobuf, bl.blprotobuf FROM fileinfos fi
		INNER JOIN files f on fi.sequence = f.sequence
		INNER JOIN blocklists bl ON bl.blocklist_hash = f.blocklist_hash
		INNER JOIN folders o ON o.idx = f.folder_idx
		WHERE f.blocklist_hash = ?`,
		h)
	items := iterStructsErr[row](errptr, rows, err)
	return func(yield func(string, protocol.FileInfo) bool) {
		for r := range items {
			fi, err := indirectFI{FiProtobuf: r.FiProtobuf, BlProtobuf: r.BlProtobuf}.FileInfo()
			if err != nil {
				*errptr = err
				return
			}
			if !yield(r.FolderID, fi) {
				return
			}
		}
	}
}

type sizesRow struct {
	Type    protocol.FileInfoType
	Count   int
	Size    int64
	FlagBit int64 `db:"local_flags"`
}

func (s *DB) LocalSize(folder string, device protocol.DeviceID) db.Counts {
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
		return db.Counts{}
	}
	return summarizeRows(res)
}

func (s *DB) Folders() ([]string, error) {
	var res []string
	err := s.sql.Select(&res, `SELECT folder_id FROM folders ORDER BY folder_id`)
	return res, wrap("folders", err)
}

func (s *DB) DevicesForFolder(folder string) ([]protocol.DeviceID, error) {
	var res []string
	err := s.sql.Select(&res, `
		SELECT d.device_id FROM sizes s
		INNER JOIN folders o ON o.idx = s.folder_idx
		INNER JOIN devices d ON d.idx = s.device_idx
		WHERE o.folder_id = ? AND s.count > 0 AND s.device_idx != ?
		GROUP BY d.device_id
		ORDER BY d.device_id
	`, folder, s.localDeviceIdx)
	if err != nil {
		return nil, wrap("devices for folder", err)
	}

	devs := make([]protocol.DeviceID, len(res))
	for i, s := range res {
		devs[i], err = protocol.DeviceIDFromString(s)
		if err != nil {
			return nil, err
		}
	}
	return devs, nil
}

func (s *DB) NeedSize(folder string, device protocol.DeviceID) db.Counts {
	if device == protocol.LocalDeviceID {
		return s.needSizeLocal(folder)
	}
	return s.needSizeRemote(folder, device)
}

func (s *DB) needSizeLocal(folder string) db.Counts {
	// The need size for the local device is the sum of entries with both
	// the global and need bit set.
	var res []sizesRow
	err := s.sql.Select(&res, `
		SELECT s.type, s.count, s.size, s.local_flags FROM sizes s
		INNER JOIN folders o ON o.idx = s.folder_idx
		WHERE o.folder_id = ? AND s.local_flags & ? = ?
	`, folder, protocol.FlagLocalNeeded|protocol.FlagLocalGlobal, protocol.FlagLocalNeeded|protocol.FlagLocalGlobal)
	if err != nil {
		return db.Counts{}
	}
	return summarizeRows(res)
}

func (s *DB) needSizeRemote(folder string, device protocol.DeviceID) db.Counts {
	// The need size for a remote device is the global size minus the local
	// size plus the need size.
	var res []sizesRow
	err := s.sql.Select(&res, `
		SELECT type, count, size, local_flags FROM sizes s
		INNER JOIN folders o ON o.idx = s.folder_idx
		INNER JOIN devices d ON d.idx = s.device_idx
		WHERE d.device_id = ? AND local_flags & ? != 0
	`, folder, device.String(), protocol.FlagLocalNeeded)
	if err != nil {
		panic(err)
	}
	need := summarizeRows(res)
	have := s.LocalSize(folder, device)
	global := s.GlobalSize(folder)
	return global.Subtract(have).Add(need)
}

func (s *DB) GlobalSize(folder string) db.Counts {
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
		return db.Counts{}
	}
	return summarizeRows(res)
}

func (s *DB) ReceiveOnlySize(folder string) db.Counts {
	var res []sizesRow
	err := s.sql.Select(&res, `
		SELECT s.type, s.count, s.size, s.local_flags FROM sizes s
		INNER JOIN folders o ON o.idx = s.folder_idx
		WHERE o.folder_id = ? AND local_flags & ? != 0
	`, folder, protocol.FlagLocalReceiveOnly)
	if err != nil {
		return db.Counts{}
	}
	return summarizeRows(res)
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

func (s *DB) folderIdxLocked(folderID string) (int64, error) {
	if _, err := s.sql.Exec(`INSERT OR IGNORE INTO folders(folder_id) VALUES(?)`, folderID); err != nil {
		return 0, wrap("folderIdx", err)
	}
	var idx int64
	if err := s.sql.Get(&idx, `SELECT idx FROM folders WHERE folder_id = ?`, folderID); err != nil {
		return 0, wrap("folderIdx", err)
	}

	return idx, nil
}

func (s *DB) deviceIdxLocked(deviceID protocol.DeviceID) (int64, error) {
	devStr := deviceID.String()
	if _, err := s.sql.Exec(`INSERT OR IGNORE INTO devices(device_id) VALUES(?)`, devStr); err != nil {
		return 0, wrap("deviceIdx", err)
	}
	var idx int64
	if err := s.sql.Get(&idx, `SELECT idx FROM devices WHERE device_id = ?`, devStr); err != nil {
		return 0, wrap("deviceIdx", err)
	}

	return idx, nil
}

func wrap(prefix string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", prefix, err)
}
