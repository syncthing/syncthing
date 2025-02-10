package sqlitedb

import (
	"cmp"
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"iter"
	"slices"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3" // register sqlite3 database driver
	"github.com/syncthing/syncthing/internal/gen/bep"
	"github.com/syncthing/syncthing/lib/protocol"
	"google.golang.org/protobuf/proto"
)

var initStmts = []string{
	`CREATE TABLE IF NOT EXISTS folders (
  		idx INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
  		folder_id TEXT NOT NULL UNIQUE
 	) STRICT`,

	`CREATE TABLE IF NOT EXISTS devices (
  		idx INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
  		device_id TEXT NOT NULL UNIQUE
 	) STRICT`,

	`CREATE TABLE IF NOT EXISTS files (
		folder_idx INTEGER NOT NULL,
		device_idx INTEGER NOT NULL,
  		sequence INTEGER NOT NULL,
		name TEXT NOT NULL,
		modified INTEGER NOT NULL, -- Unix nanos
		version TEXT NOT NULL,
		deleted INTEGER NOT NULL, -- boolean
		invalid INTEGER NOT NULL, -- boolean
  		fileinfo_protobuf BLOB NOT NULL,
		PRIMARY KEY(folder_idx, device_idx, sequence),
		FOREIGN KEY(device_idx) REFERENCES devices(idx) ON DELETE CASCADE,
		FOREIGN KEY(folder_idx) REFERENCES folders(idx) ON DELETE CASCADE
 	) STRICT`,
	`CREATE UNIQUE INDEX IF NOT EXISTS files_name ON files (folder_idx, device_idx, name)`,

	`CREATE TABLE IF NOT EXISTS globals (
		folder_idx INTEGER NOT NULL,
		device_idx INTEGER NOT NULL,
  		file_sequence INTEGER NOT NULL,
		name TEXT NOT NULL,
		FOREIGN KEY(folder_idx) REFERENCES folders(idx) ON DELETE CASCADE,
		FOREIGN KEY(device_idx) REFERENCES devices(idx) ON DELETE CASCADE,
		FOREIGN KEY(folder_idx, device_idx, file_sequence) REFERENCES files(folder_idx, device_idx, sequence) ON DELETE CASCADE
 	) STRICT`,
	`CREATE UNIQUE INDEX IF NOT EXISTS globals_seq ON globals (folder_idx, device_idx, file_sequence)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS globals_name ON globals (folder_idx, name)`,

	`CREATE TABLE IF NOT EXISTS needs (
		folder_idx INTEGER NOT NULL,
		device_idx INTEGER NOT NULL,
  		file_sequence INTEGER NOT NULL,
		name TEXT NOT NULL,
		FOREIGN KEY(folder_idx) REFERENCES folders(idx) ON DELETE CASCADE,
		FOREIGN KEY(device_idx) REFERENCES devices(idx) ON DELETE CASCADE,
		FOREIGN KEY(folder_idx, device_idx, file_sequence) REFERENCES files(folder_idx, device_idx, sequence) ON DELETE CASCADE
 	) STRICT`,
	`CREATE UNIQUE INDEX IF NOT EXISTS needs_seq ON needs (folder_idx, device_idx, file_sequence)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS needs_name ON needs (folder_idx, device_idx, name)`,
}

func Open(path string) (*DB, error) {
	var err error
	sqlDB, err := sqlx.Open("sqlite3", path+"?_fk=true")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	for _, stmt := range initStmts {
		if _, err := sqlDB.Exec(stmt); err != nil {
			return nil, fmt.Errorf("init statements: %s: %w", stmt, err)
		}
	}

	db := &DB{sql: sqlDB}

	// should always exist and have a low index number, and will never
	// change
	db.localDeviceIdx, _ = db.deviceIdx(protocol.LocalDeviceID)

	return db, nil
}

type DB struct {
	sql            *sqlx.DB
	localDeviceIdx int64
}

func (db *DB) Close() error {
	return wrap("close", db.sql.Close())
}

func (db *DB) Update(folder string, device protocol.DeviceID, fs []protocol.FileInfo) error {
	tx, err := db.sql.BeginTxx(context.Background(), &sql.TxOptions{Isolation: sql.LevelReadUncommitted, ReadOnly: false})
	if err != nil {
		return wrap("update", err)
	}
	defer tx.Rollback()

	folderIdx, err := db.folderIdx(folder)
	if err != nil {
		return wrap("update", err)
	}
	deviceIdx, err := db.deviceIdx(device)
	if err != nil {
		return wrap("update", err)
	}

	var seq int64
	if device == protocol.LocalDeviceID {
		_ = db.sql.Get(&seq, `SELECT MAX(sequence) FROM files WHERE folder_idx = $1 AND device_idx = $2`, folderIdx, deviceIdx)
	}

	for _, f := range fs {
		if device == protocol.LocalDeviceID {
			seq++
			f.Sequence = seq
		}

		bs, err := proto.Marshal(f.ToWire(true))
		if err != nil {
			return wrap("update", err)
		}
		if _, err := tx.Exec(`
			INSERT OR REPLACE INTO files (folder_idx, device_idx, sequence, name, modified, version, deleted, invalid, fileinfo_protobuf)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			folderIdx, deviceIdx, f.Sequence, f.Name, f.ModTime().UnixNano(), f.Version.String(), f.IsDeleted(), f.IsInvalid(), bs); err != nil {
			return wrap("update", err)
		}
		if err := db.processNeed(tx, folder, f.Name); err != nil {
			return wrap("update", err)
		}
	}

	return wrap("update", tx.Commit())
}

func (db *DB) Drop(device protocol.DeviceID) error {
	tx, err := db.sql.BeginTx(context.Background(), &sql.TxOptions{Isolation: sql.LevelReadCommitted, ReadOnly: false})
	if err != nil {
		return wrap("drop", err)
	}
	defer tx.Rollback()

	if _, err := db.sql.Exec(`DELETE FROM devices WHERE device_id = $1`, device.String()); err != nil {
		return wrap("drop", err)
	}

	return wrap("drop", tx.Commit())
}

func (db *DB) Get(folder string, device protocol.DeviceID, file string) (*protocol.FileInfo, bool, error) {
	var pbm pbAdapter[bep.FileInfo, *bep.FileInfo]
	err := db.sql.Get(&pbm, `
		SELECT f.fileinfo_protobuf FROM files f
		INNER JOIN devices d ON f.device_idx = d.idx
		INNER JOIN folders o ON f.folder_idx = o.idx
		WHERE o.folder_id = $1 AND d.device_id = $2 AND f.name = $3`, folder, device.String(), file)
	if err != nil {
		return nil, false, wrap("get", err)
	}
	fi := protocol.FileInfoFromDB(&pbm.Message)
	return &fi, true, nil
}

func (db *DB) processNeed(tx *sqlx.Tx, folder, file string) error {
	vals := iterStructs[globalEntry](tx.Queryx(`
		SELECT f.name, f.folder_idx, f.device_idx, f.sequence, f.modified, f.version, f.deleted FROM files f
		INNER JOIN folders o ON f.folder_idx = o.idx
		WHERE f.name = $1 AND o.folder_id = $2`,
		file, folder))
	es, err := iterCollect(vals)
	if err != nil {
		return err
	}
	return db.processNeedSet(tx, es)
}

func (db *DB) processNeedSet(tx *sqlx.Tx, es []globalEntry) error {
	// Sort the entries; the global entry is at the head of the list
	slices.SortFunc(es, globalEntry.Compare)
	for i, e := range es {
		switch {
		case i == 0:
			if _, err := tx.Exec(`
			INSERT OR REPLACE INTO globals (folder_idx, device_idx, file_sequence, name)
			VALUES ($1, $2, $3, $4)`,
				e.FolderIdx, e.DeviceIdx, e.Sequence, e.Name); err != nil {
				return wrap("processNeedSet", err)
			}
			fallthrough

		case e.Version.Equal(es[0].Version.Vector):
			// The global entry is never needed, nor others that are identical to it
			if _, err := tx.Exec(`DELETE FROM needs WHERE folder_idx = $1 AND device_idx = $2 AND file_sequence = $3`, e.FolderIdx, e.DeviceIdx, e.Sequence); err != nil {
				return wrap("processNeedSet", err)
			}

		default:
			// Need it
			if _, err := tx.Exec(`INSERT OR IGNORE INTO needs (folder_idx, device_idx, file_sequence, name) VALUES ($1, $2, $3, $4)`, e.FolderIdx, e.DeviceIdx, e.Sequence, e.Name); err != nil {
				return wrap("processNeedSet", err)
			}
		}
	}
	return nil
}

type globalEntry struct {
	Name      string
	FolderIdx int64 `db:"folder_idx"`
	DeviceIdx int64 `db:"device_idx"`
	Sequence  int64
	Modified  int64
	Version   dbVector
	Deleted   bool
	Invalid   bool
}

func (e globalEntry) Compare(other globalEntry) int {
	// From FileInfo.WinsConflict
	vc := e.Version.Vector.Compare(other.Version.Vector)
	switch vc {
	case protocol.Equal:
		return 0
	case protocol.Greater: // we are newer
		return -1
	case protocol.Lesser: // we are older
		return 1
	case protocol.ConcurrentGreater, protocol.ConcurrentLesser: // there is a conflict
		if e.Invalid != other.Invalid {
			if e.Invalid { // we are invalid, we lose
				return 1
			}
			return -1 // they are invalid, we win
		}
		if e.Deleted != other.Deleted {
			if e.Deleted { // we are deleted, we lose
				return 1
			}
			return -1 // they are deleted, we win
		}
		if d := cmp.Compare(e.Modified, other.Modified); d != 0 {
			return -d // positive d means we were newer, so we win (negative return)
		}
		if vc == protocol.ConcurrentGreater {
			return -1 // we have a better device ID, we win
		}
		return 1 // they win
	default:
		return 0
	}
}

func (db *DB) GetGlobal(folder string, file string) (*protocol.FileInfo, bool, error) {
	var pbm pbAdapter[bep.FileInfo, *bep.FileInfo]
	if err := db.sql.Get(&pbm, `
		SELECT f.fileinfo_protobuf FROM files f
		INNER JOIN globals g ON f.folder_idx = g.folder_idx AND f.device_idx = g.device_idx AND f.sequence = g.file_sequence
		INNER JOIN folders o ON o.idx = g.folder_idx
		WHERE o.folder_id = $1 AND g.name = $2`, folder, file); err != nil {
		return nil, false, wrap("getGlobal", err)
	}

	fi := protocol.FileInfoFromDB(&pbm.Message)
	return &fi, true, nil
}

func (db *DB) WithNeed(folder string, device protocol.DeviceID) iter.Seq2[*protocol.FileInfo, error] {
	vals := iterValues[[]byte](db.sql.Queryx(`
		SELECT f.fileinfo_protobuf FROM files f
		INNER JOIN needs n ON f.folder_idx = n.folder_idx AND f.device_idx = n.device_idx AND f.sequence = n.file_sequence
		INNER JOIN folders o ON o.idx = n.folder_idx
		INNER JOIN devices d ON d.idx = n.device_idx
		WHERE o.folder_id = $1 AND d.device_id = $2`,
		folder, device.String()))
	beps := iterProto[bep.FileInfo](vals)
	return iterMap(beps, func(b *bep.FileInfo) *protocol.FileInfo {
		fi := protocol.FileInfoFromDB(b)
		return &fi
	})
}

func (db *DB) folderIdx(folderID string) (int64, error) {
	if _, err := db.sql.Exec(`INSERT OR IGNORE INTO folders(folder_id) VALUES($1)`, folderID); err != nil {
		return 0, wrap("folderIdx", err)
	}
	var idx int64
	if err := db.sql.Get(&idx, `SELECT idx FROM folders WHERE folder_id = $1`, folderID); err != nil {
		return 0, wrap("folderIdx", err)
	}

	return idx, nil
}

func (db *DB) deviceIdx(deviceID protocol.DeviceID) (int64, error) {
	devStr := deviceID.String()
	if _, err := db.sql.Exec(`INSERT OR IGNORE INTO devices(device_id) VALUES($1)`, devStr); err != nil {
		return 0, wrap("deviceIdx", err)
	}
	var idx int64
	if err := db.sql.Get(&idx, `SELECT idx FROM devices WHERE device_id = $1`, devStr); err != nil {
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

type dbVector struct {
	protocol.Vector
}

func (v dbVector) Value() (driver.Value, error) {
	return v.String(), nil
}

func (v *dbVector) Scan(value any) error {
	str, ok := value.(string)
	if !ok {
		return errors.New("not a string")
	}
	vec, err := protocol.VectorFromString(str)
	if err != nil {
		return err
	}
	v.Vector = vec

	return nil
}

type pbMessage[T any] interface {
	*T
	proto.Message
}

type pbAdapter[T any, PT pbMessage[T]] struct {
	Message T
}

func (v pbAdapter[T, PT]) Value() (driver.Value, error) {
	return proto.Marshal(PT(&v.Message))
}

func (v *pbAdapter[T, PT]) Scan(value any) error {
	bs, ok := value.([]byte)
	if !ok {
		return errors.New("not a byte slice")
	}
	return proto.Unmarshal(bs, PT(&v.Message))
}
