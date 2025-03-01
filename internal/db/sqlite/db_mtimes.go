package sqlite

import (
	"time"
)

func (db *DB) MtimeGet(folder, name string) (ondisk, virtual time.Time) {
	var res struct {
		Ondisk  int64
		Virtual int64
	}
	if err := db.sql.Get(&res, `
		SELECT m.ondisk, m.virtual FROM mtimes m
		INNER JOIN folders o ON o.idx = m.folder_idx
		WHERE o.folder_id = ? AND m.name = ?`, folder, name); err != nil {
		return time.Time{}, time.Time{}
	}
	return time.Unix(0, res.Ondisk), time.Unix(0, res.Virtual)
}

func (db *DB) MtimePut(folder, name string, ondisk, virtual time.Time) error {
	db.updateLock.Lock()
	defer db.updateLock.Unlock()
	folderIdx, err := db.folderIdxLocked(folder)
	if err != nil {
		return err
	}
	_, err = db.sql.Exec(`INSERT OR REPLACE INTO mtimes (folder_idx, name, ondisk, virtual) values (?, ?, ?, ?)`, folderIdx, name, ondisk.UnixNano(), virtual.UnixNano())
	return err
}

func (db *DB) MtimeDelete(folder, name string) error {
	db.updateLock.Lock()
	defer db.updateLock.Unlock()
	folderIdx, err := db.folderIdxLocked(folder)
	if err != nil {
		return err
	}
	_, err = db.sql.Exec(`DELETE FROM mtimes WHERE folder_idx = ? AND name = ? `, folderIdx, name)
	return err
}
