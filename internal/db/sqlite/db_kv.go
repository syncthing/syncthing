package sqlite

import (
	"iter"

	"github.com/jmoiron/sqlx"
)

type KV struct {
	sql *sqlx.DB
}

func (db *DB) KV() *KV {
	return &KV{sql: db.sql}
}

func (kv *KV) Get(key string) ([]byte, error) {
	var val []byte
	if err := kv.sql.Get(&val, `SELECT value FROM kv WHERE key = ?`, key); err != nil {
		return nil, err
	}
	return val, nil
}

func (kv *KV) Put(key string, val []byte) error {
	_, err := kv.sql.Exec(`INSERT OR REPLACE INTO kv (key, value) values (?, ?)`, key, val)
	return err
}

func (kv *KV) Delete(key string) error {
	_, err := kv.sql.Exec(`DELETE FROM kv WHERE key = ?`, key)
	return err
}

type KVEntry struct {
	Key   string
	Value []byte
}

func (kv *KV) Prefix(prefix string) iter.Seq2[string, []byte] {
	prefix += "%"
	rows, err := kv.sql.Queryx(`SELECT key, value FROM kv WHERE key LIKE ?`, prefix)
	if err != nil {
		return func(func(string, []byte) bool) {}
	}
	return func(yield func(string, []byte) bool) {
		defer rows.Close()
		for rows.Next() {
			var key string
			var val []byte
			if err := rows.Scan(&key, &val); err != nil {
				// XXX yolo
				return
			}
			if !yield(key, val) {
				return
			}
		}
	}
}
