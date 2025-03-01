package sqlite

import (
	"iter"
)

func (db *DB) KVGet(key string) ([]byte, error) {
	var val []byte
	if err := db.sql.Get(&val, `SELECT value FROM kv WHERE key = ?`, key); err != nil {
		return nil, err
	}
	return val, nil
}

func (db *DB) KVPut(key string, val []byte) error {
	db.updateLock.Lock()
	defer db.updateLock.Unlock()
	_, err := db.sql.Exec(`INSERT OR REPLACE INTO kv (key, value) values (?, ?)`, key, val)
	return err
}

func (db *DB) KVDelete(key string) error {
	db.updateLock.Lock()
	defer db.updateLock.Unlock()
	_, err := db.sql.Exec(`DELETE FROM kv WHERE key = ?`, key)
	return err
}

func (db *DB) KVPrefix(prefix string) (iter.Seq2[string, []byte], func() error) {
	prefix += "%"
	rows, err := db.sql.Queryx(`SELECT key, value FROM kv WHERE key LIKE ?`, prefix)
	if err != nil {
		return func(yield func(string, []byte) bool) {}, func() error { return err }
	}

	return func(yield func(string, []byte) bool) {
			defer rows.Close()
			for rows.Next() {
				var key string
				var val []byte
				if err = rows.Scan(&key, &val); err != nil {
					return
				}
				if !yield(key, val) {
					return
				}
			}
			err = rows.Err()
		}, func() error {
			return err
		}
}
