package sqlite

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/syncthing/syncthing/lib/protocol"
)

func (s *DB) IndexID(folder string, device protocol.DeviceID) (protocol.IndexID, error) {
	// Try a fast read-only query to begin with. If it does not find the ID
	// we'll do the full thing under a lock.
	var indexID string
	if err := s.sql.Get(&indexID, `
		SELECT i.index_id FROM index_ids i
		INNER JOIN folders o ON o.idx  = i.folder_idx
		INNER JOIN devices d ON d.idx  = i.device_idx
		WHERE o.folder_id = ? AND d.device_id = ?`,
		folder, device.String(),
	); err == nil && indexID != "" {
		return indexIDFromHex(indexID)
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
		return 0, fmt.Errorf("indexID (folderIdx): %w", err)
	}

	tx, err := s.sql.BeginTxx(context.Background(), nil)
	if err != nil {
		return 0, fmt.Errorf("indexID (begin): %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if err := tx.Get(&indexID, `
		SELECT index_id FROM index_ids WHERE folder_idx = ? AND device_idx = ?`,
		folderIdx, s.localDeviceIdx,
	); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("indexID (get): %w", err)
	}

	if indexID == "" {
		// Generate a new index ID
		id := protocol.NewIndexID()
		if _, err := tx.Exec(`INSERT INTO index_ids (folder_idx, device_idx, index_id) values (?, ?, ?)`,
			folderIdx, s.localDeviceIdx, indexIDToHex(id),
		); err != nil {
			return 0, fmt.Errorf("indexID (insert): %w", err)
		}
		if err := tx.Commit(); err != nil {
			return 0, fmt.Errorf("indexID (commit): %w", err)
		}
		return id, nil
	}

	return indexIDFromHex(indexID)
}

func (s *DB) SetIndexID(folder string, device protocol.DeviceID, id protocol.IndexID) error {
	s.updateLock.Lock()
	defer s.updateLock.Unlock()

	folderIdx, err := s.folderIdxLocked(folder)
	if err != nil {
		return fmt.Errorf("indexID (folderIdx): %w", err)
	}
	deviceIdx, err := s.deviceIdxLocked(device)
	if err != nil {
		return fmt.Errorf("indexID (deviceIdx): %w", err)
	}

	if _, err := s.sql.Exec(`INSERT OR REPLACE INTO index_ids (folder_idx, device_idx, index_id) values (?, ?, ?)`,
		folderIdx, deviceIdx, indexIDToHex(id),
	); err != nil {
		return fmt.Errorf("indexID (insert): %w", err)
	}
	return nil
}

func (s *DB) DropIndexIDs() error {
	s.updateLock.Lock()
	defer s.updateLock.Unlock()
	_, err := s.sql.Exec(`DELETE FROM index_ids`)
	return wrap("drop index IDs", err)
}

func indexIDFromHex(s string) (protocol.IndexID, error) {
	bs, err := hex.DecodeString(s)
	if err != nil {
		return 0, fmt.Errorf("%q: %w", s, err)
	}
	var id protocol.IndexID
	if err := id.Unmarshal(bs); err != nil {
		return 0, fmt.Errorf("%q: %w", s, err)
	}
	return id, nil
}

func indexIDToHex(i protocol.IndexID) string {
	bs, _ := i.Marshal()
	return hex.EncodeToString(bs)
}
