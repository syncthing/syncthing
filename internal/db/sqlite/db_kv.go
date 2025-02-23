package sqlite

import (
	"iter"
)

type KV struct {
	db *DB
}

func (db *DB) KV() *KV {
	return &KV{db: db}
}

func (kv *KV) Get(key string) ([]byte, error) {
	var val []byte
	if err := kv.db.sql.Get(&val, `SELECT value FROM kv WHERE key = ?`, key); err != nil {
		return nil, err
	}
	return val, nil
}

func (kv *KV) Put(key string, val []byte) error {
	kv.db.updateLock.Lock()
	defer kv.db.updateLock.Unlock()
	_, err := kv.db.sql.Exec(`INSERT OR REPLACE INTO kv (key, value) values (?, ?)`, key, val)
	return err
}

func (kv *KV) Delete(key string) error {
	kv.db.updateLock.Lock()
	defer kv.db.updateLock.Unlock()
	_, err := kv.db.sql.Exec(`DELETE FROM kv WHERE key = ?`, key)
	return err
}

type KVEntry struct {
	Key   string
	Value []byte
}

func (kv *KV) Prefix(prefix string) iter.Seq2[KVEntry, error] {
	prefix += "%"
	return iterStructs[KVEntry](kv.db.sql.Queryx(`SELECT key, value FROM kv WHERE key LIKE ?`, prefix))
}
