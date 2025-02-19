package sqlite

func (db *DB) Get(key string) ([]byte, error) {
	var val []byte
	if err := db.sql.Get(&val, `SELECT value FROM kv WHERE key = ?`, key); err != nil {
		return nil, err
	}
	return val, nil
}

func (db *DB) Put(key string, val []byte) error {
	_, err := db.sql.Exec(`INSERT OR REPLACE INTO kv (key, value) values (?, ?)`, key, val)
	return err
}

func (db *DB) Delete(key string) error {
	_, err := db.sql.Exec(`DELETE FROM kv WHERE key = ?`, key)
	return err
}
