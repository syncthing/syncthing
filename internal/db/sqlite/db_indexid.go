package sqlite

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/syncthing/syncthing/lib/protocol"
)

func (s *DB) IndexIDGet(folder string, device protocol.DeviceID) (protocol.IndexID, error) {
	// Try a fast read-only query to begin with. If it does not find the ID
	// we'll do the full thing under a lock.
	var indexID string
	if err := s.sql.Get(&indexID, `
		SELECT i.index_id FROM indexids i
		INNER JOIN folders o ON o.idx  = i.folder_idx
		INNER JOIN devices d ON d.idx  = i.device_idx
		WHERE o.folder_id = ? AND d.device_id = ?`,
		folder, device.String(),
	); err == nil && indexID != "" {
		idx, err := indexIDFromHex(indexID)
		return idx, wrap(err)
	}
	if device != protocol.LocalDeviceID {
		// For non-local devices we do not create the index ID, so return
		// zero anyway if we don't have one.
		return 0, nil
	}

	s.updateLock.Lock()
	defer s.updateLock.Unlock()

	// We are now operating only for the local device ID

	folderIdx, err := s.folderIdxLocked(folder)
	if err != nil {
		return 0, wrap(err)
	}

	tx, err := s.sql.BeginTxx(context.Background(), nil)
	if err != nil {
		return 0, wrap(err)
	}
	defer tx.Rollback() //nolint:errcheck

	if err := tx.Get(&indexID, s.tpl(`
		SELECT index_id FROM indexids WHERE folder_idx = ? AND device_idx = {{.LocalDeviceIdx}}
	`), folderIdx,
	); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, wrap(err)
	}

	if indexID == "" {
		// Generate a new index ID
		id := protocol.NewIndexID()
		if _, err := tx.Exec(s.tpl(`
			INSERT INTO indexids (folder_idx, device_idx, index_id) values (?, {{.LocalDeviceIdx}}, ?)
		`), folderIdx, indexIDToHex(id),
		); err != nil {
			return 0, wrap(err)
		}
		if err := tx.Commit(); err != nil {
			return 0, wrap(err)
		}
		return id, nil
	}

	return indexIDFromHex(indexID)
}

func (s *DB) IndexIDSet(folder string, device protocol.DeviceID, id protocol.IndexID) error {
	s.updateLock.Lock()
	defer s.updateLock.Unlock()

	folderIdx, err := s.folderIdxLocked(folder)
	if err != nil {
		return wrap(err, "folder idx")
	}
	deviceIdx, err := s.deviceIdxLocked(device)
	if err != nil {
		return wrap(err, "device idx")
	}

	if _, err := s.sql.Exec(`
		INSERT OR REPLACE INTO indexids (folder_idx, device_idx, index_id) values (?, ?, ?)
	`, folderIdx, deviceIdx, indexIDToHex(id),
	); err != nil {
		return wrap(err, "insert")
	}
	return nil
}

func (s *DB) IndexIDDropAll() error {
	s.updateLock.Lock()
	defer s.updateLock.Unlock()
	_, err := s.sql.Exec(`DELETE FROM indexids`)
	return wrap(err)
}

func indexIDFromHex(s string) (protocol.IndexID, error) {
	bs, err := hex.DecodeString(s)
	if err != nil {
		return 0, fmt.Errorf("indexIDFromHex: %q: %w", s, err)
	}
	var id protocol.IndexID
	if err := id.Unmarshal(bs); err != nil {
		return 0, fmt.Errorf("indexIDFromHex: %q: %w", s, err)
	}
	return id, nil
}

func indexIDToHex(i protocol.IndexID) string {
	bs, _ := i.Marshal()
	return hex.EncodeToString(bs)
}
