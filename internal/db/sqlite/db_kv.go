package sqlite

import (
	"iter"

	"github.com/jmoiron/sqlx"
	"github.com/syncthing/syncthing/internal/db"
)

func (s *DB) GetKV(key string) ([]byte, error) {
	var val []byte
	if err := s.stmt(`
		SELECT value FROM kv
		WHERE key = ?
	`).Get(&val, key); err != nil {
		return nil, wrap(err)
	}
	return val, nil
}

func (s *DB) PutKV(key string, val []byte) error {
	s.updateLock.Lock()
	defer s.updateLock.Unlock()
	_, err := s.stmt(`
		INSERT OR REPLACE INTO kv (key, value)
		VALUES (?, ?)
	`).Exec(key, val)
	return wrap(err)
}

func (s *DB) DeleteKV(key string) error {
	s.updateLock.Lock()
	defer s.updateLock.Unlock()
	_, err := s.stmt(`
		DELETE FROM kv WHERE key = ?
	`).Exec(key)
	return wrap(err)
}

func (s *DB) PrefixKV(prefix string) (iter.Seq[db.KeyValue], func() error) {
	var rows *sqlx.Rows
	var err error
	if prefix == "" {
		rows, err = s.stmt(`SELECT key, value FROM kv`).Queryx()
	} else {
		end := prefixEnd(prefix)
		rows, err = s.stmt(`
			SELECT key, value FROM kv
			WHERE key >= ? AND key < ?
		`).Queryx(prefix, end)
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
