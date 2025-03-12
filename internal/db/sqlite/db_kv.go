package sqlite

import (
	"iter"

	"github.com/syncthing/syncthing/internal/db"
)

func (s *DB) KVGet(key string) ([]byte, error) {
	var val []byte
	if err := s.sql.Get(&val, `SELECT value FROM kv WHERE key = ?`, key); err != nil {
		return nil, err
	}
	return val, nil
}

func (s *DB) KVPut(key string, val []byte) error {
	s.updateLock.Lock()
	defer s.updateLock.Unlock()
	_, err := s.sql.Exec(`INSERT OR REPLACE INTO kv (key, value) values (?, ?)`, key, val)
	return err
}

func (s *DB) KVDelete(key string) error {
	s.updateLock.Lock()
	defer s.updateLock.Unlock()
	_, err := s.sql.Exec(`DELETE FROM kv WHERE key = ?`, key)
	return err
}

func (s *DB) KVPrefix(prefix string) (iter.Seq[db.KeyValue], func() error) {
	prefix += "%"
	rows, err := s.sql.Queryx(`SELECT key, value FROM kv WHERE key LIKE ?`, prefix)
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
