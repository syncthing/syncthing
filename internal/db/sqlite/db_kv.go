package sqlite

import (
	"iter"

	"github.com/jmoiron/sqlx"
	"github.com/syncthing/syncthing/internal/db"
)

func (s *DB) KVGet(key string) ([]byte, error) {
	var val []byte
	if err := s.sql.Get(&val, `SELECT value FROM kv WHERE key = ?`, key); err != nil {
		return nil, wrap(err)
	}
	return val, nil
}

func (s *DB) KVPut(key string, val []byte) error {
	s.updateLock.Lock()
	defer s.updateLock.Unlock()
	_, err := s.sql.Exec(`INSERT OR REPLACE INTO kv (key, value) values (?, ?)`, key, val)
	return wrap(err)
}

func (s *DB) KVDelete(key string) error {
	s.updateLock.Lock()
	defer s.updateLock.Unlock()
	_, err := s.sql.Exec(`DELETE FROM kv WHERE key = ?`, key)
	return wrap(err)
}

func (s *DB) KVPrefix(prefix string) (iter.Seq[db.KeyValue], func() error) {
	var rows *sqlx.Rows
	var err error
	if prefix == "" {
		rows, err = s.sql.Queryx(`SELECT key, value FROM kv`)
	} else {
		end := prefixEnd(prefix)
		rows, err = s.sql.Queryx(`SELECT key, value FROM kv WHERE key >= ? AND key < ?`, prefix, end)
	}
	if err != nil {
		return func(_ func(db.KeyValue) bool) {}, func() error { return err }
	}

	return func(yield func(db.KeyValue) bool) {
			defer rows.Close()
			for rows.Next() {
				var key string
				var val []byte
				if err = rows.Scan(&key, &val); err != nil {
					return
				}
				if !yield(db.KeyValue{Key: key, Value: val}) {
					return
				}
			}
			err = rows.Err()
		}, func() error {
			return err
		}
}
