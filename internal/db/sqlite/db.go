package sqlite

import (
	"sync"

	"github.com/jmoiron/sqlx"
	"github.com/syncthing/syncthing/internal/db"
	"github.com/syncthing/syncthing/lib/protocol"
)

type DB struct {
	sql            *sqlx.DB
	localDeviceIdx int64
	updateLock     sync.Mutex
	prepared       map[string]*sqlx.Stmt

	statementsMut sync.RWMutex
	statements    map[string]string
	tplInput      map[string]any
}

var _ db.DB = (*DB)(nil)

func (s *DB) Close() error {
	s.updateLock.Lock()
	defer s.updateLock.Unlock()
	return wrap(s.sql.Close())
}

func (s *DB) ListFolders() ([]string, error) {
	var res []string
	err := s.sql.Select(&res, `SELECT folder_id FROM folders ORDER BY folder_id`)
	return res, wrap(err)
}

func (s *DB) ListDevicesForFolder(folder string) ([]protocol.DeviceID, error) {
	var res []string
	err := s.sql.Select(&res, s.tpl(`
		SELECT d.device_id FROM counts s
		INNER JOIN folders o ON o.idx = s.folder_idx
		INNER JOIN devices d ON d.idx = s.device_idx
		WHERE o.folder_id = ? AND s.count > 0 AND s.device_idx != {{.LocalDeviceIdx}}
		GROUP BY d.device_id
		ORDER BY d.device_id
	`), folder)
	if err != nil {
		return nil, wrap(err)
	}

	devs := make([]protocol.DeviceID, len(res))
	for i, s := range res {
		devs[i], err = protocol.DeviceIDFromString(s)
		if err != nil {
			return nil, err
		}
	}
	return devs, nil
}
