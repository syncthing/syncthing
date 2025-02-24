package sqlite

import (
	"iter"

	"github.com/syncthing/syncthing/internal/db"
)

type KV struct {
	s *DB
}

func (s *DB) KV() db.KV {
	return &KV{s: s}
}

func (kv *KV) Get(key string) ([]byte, error) {
	var val []byte
	if err := kv.s.sql.Get(&val, `SELECT value FROM kv WHERE key = ?`, key); err != nil {
		return nil, err
	}
	return val, nil
}

func (kv *KV) Put(key string, val []byte) error {
	kv.s.updateLock.Lock()
	defer kv.s.updateLock.Unlock()
	_, err := kv.s.sql.Exec(`INSERT OR REPLACE INTO kv (key, value) values (?, ?)`, key, val)
	return err
}

func (kv *KV) Delete(key string) error {
	kv.s.updateLock.Lock()
	defer kv.s.updateLock.Unlock()
	_, err := kv.s.sql.Exec(`DELETE FROM kv WHERE key = ?`, key)
	return err
}

func (kv *KV) Prefix(prefix string) iter.Seq2[db.KVEntry, error] {
	prefix += "%"
	return iterStructs[db.KVEntry](kv.s.sql.Queryx(`SELECT key, value FROM kv WHERE key LIKE ?`, prefix))
}
