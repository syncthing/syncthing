package db2

import (
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

	`CREATE TABLE IF NOT EXISTS needs (
		folder_idx INTEGER NOT NULL,
		device_idx INTEGER NOT NULL,
  		file_sequence INTEGER NOT NULL,
		name TEXT NOT NULL,
		FOREIGN KEY(folder_idx) REFERENCES folders(idx) ON DELETE CASCADE,
		FOREIGN KEY(device_idx) REFERENCES devices(idx) ON DELETE CASCADE
		--FOREIGN KEY(file_sequence) REFERENCES files(sequence) ON DELETE CASCADE
 	) STRICT`,
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

	// should always exist and have a low index number
	_, _ = db.deviceIdx(protocol.LocalDeviceID)

	return db, nil
}

type DB struct {
	sql *sqlx.DB
}

func (db *DB) Close() error {
	return wrap("close", db.sql.Close())
}

func (db *DB) Update(folder string, device protocol.DeviceID, fs []protocol.FileInfo) error {
	tx, err := db.sql.BeginTx(context.Background(), &sql.TxOptions{Isolation: sql.LevelReadCommitted, ReadOnly: false})
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

func (db *DB) Get(folder string, device protocol.DeviceID, file string) (protocol.FileInfo, bool, error) {
	var pbm pbAdapter[bep.FileInfo, *bep.FileInfo]
	err := db.sql.Get(&pbm, `
		SELECT f.fileinfo_protobuf FROM files f
		INNER JOIN devices d ON f.device_idx = d.idx
		INNER JOIN folders o ON f.folder_idx = o.idx
		WHERE o.folder_id = $1 AND d.device_id = $2 AND f.name = $3`, folder, device.String(), file)
	if err != nil {
		return protocol.FileInfo{}, false, wrap("get", err)
	}
	return protocol.FileInfoFromDB(&pbm.Message), true, nil
}

func (db *DB) processNeed(folder string) error {
	rows, err := db.sql.Queryx(`
		SELECT f.name, f.folder_idx, f.device_idx, f.sequence, f.modified, f.version, f.deleted FROM files f
		INNER JOIN folders o ON f.folder_idx = o.idx
		WHERE f.invalid = FALSE AND o.folder_id = $1
		ORDER BY f.name`, folder)
	if err != nil {
		return wrap("processNeed", err)
	}

	var es []globalEntry
	for e := range allStructs[globalEntry](rows) {
		if len(es) == 0 || es[0].Name == e.Name {
			es = append(es, e)
			continue
		}
		db.processNeedSet(es)
		es = es[:0]
		es = append(es, e)
	}
	if len(es) > 0 {
		db.processNeedSet(es)
	}

	return nil
}

func (db *DB) processNeedSet(es []globalEntry) error {
	fmt.Printf("%+v\n", es)
	return nil
}

func allStructs[T any](rows *sqlx.Rows) iter.Seq[T] {
	return func(yield func(T) bool) {
		defer rows.Close()
		for rows.Next() {
			v := new(T)
			if err := rows.StructScan(v); err != nil {
				return
			}
			if !yield(*v) {
				return
			}
		}
	}
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

func (db *DB) GetGlobal(folder string, file string) (protocol.FileInfo, bool, error) {
	rows, err := db.sql.Queryx(`
		SELECT f.folder_idx, f.device_idx, f.sequence, f.modified, f.version, f.deleted FROM files f
		INNER JOIN folders o ON f.folder_idx = o.idx
		WHERE f.name = $1 AND f.invalid = FALSE AND o.folder_id = $2`, file, folder)
	if err != nil {
		return protocol.FileInfo{}, false, wrap("getGlobal", err)
	}
	es := slices.Collect(allStructs[globalEntry](rows))
	if rows.Err() != nil {
		return protocol.FileInfo{}, false, wrap("getGlobal", err)
	}

	if len(es) == 0 {
		return protocol.FileInfo{}, false, nil
	}
	newest := 0
	for i := 1; i < len(es); i++ { // XXX simplified
		if es[i].Version.GreaterEqual(es[newest].Version.Vector) {
			newest = i
		}
	}

	var pbm pbAdapter[bep.FileInfo, *bep.FileInfo]
	if err := db.sql.Get(&pbm, `
		SELECT fileinfo_protobuf FROM files
		WHERE folder_idx = $1 AND device_idx = $2 AND sequence = $3`,
		es[newest].FolderIdx, es[newest].DeviceIdx, es[newest].Sequence); err != nil {
		return protocol.FileInfo{}, false, wrap("getGlobal", err)
	}

	return protocol.FileInfoFromDB(&pbm.Message), true, nil
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
