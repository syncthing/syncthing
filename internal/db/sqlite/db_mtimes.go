package sqlite

import (
	"time"
)

func (s *DB) MtimeGet(folder, name string) (ondisk, virtual time.Time) {
	var res struct {
		Ondisk  int64
		Virtual int64
	}
	if err := s.sql.Get(&res, `
		SELECT m.ondisk, m.virtual FROM mtimes m
		INNER JOIN folders o ON o.idx = m.folder_idx
		WHERE o.folder_id = ? AND m.name = ?
	`, folder, name); err != nil {
		return time.Time{}, time.Time{}
	}
	return time.Unix(0, res.Ondisk), time.Unix(0, res.Virtual)
}

func (s *DB) MtimePut(folder, name string, ondisk, virtual time.Time) error {
	s.updateLock.Lock()
	defer s.updateLock.Unlock()
	folderIdx, err := s.folderIdxLocked(folder)
	if err != nil {
		return err
	}
	_, err = s.sql.Exec(`
		INSERT OR REPLACE INTO mtimes (folder_idx, name, ondisk, virtual)
		VALUES (?, ?, ?, ?)
	`, folderIdx, name, ondisk.UnixNano(), virtual.UnixNano())
	return err
}

func (s *DB) MtimeDelete(folder, name string) error {
	s.updateLock.Lock()
	defer s.updateLock.Unlock()
	folderIdx, err := s.folderIdxLocked(folder)
	if err != nil {
		return err
	}
	_, err = s.sql.Exec(`DELETE FROM mtimes WHERE folder_idx = ? AND name = ? `, folderIdx, name)
	return err
}
