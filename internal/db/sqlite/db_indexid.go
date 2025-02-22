package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/syncthing/syncthing/lib/protocol"
)

func (db *DB) IndexID(folder string, device protocol.DeviceID) (protocol.IndexID, error) {
	// Explicitly get folder and device idx because this might be our first
	// contact with the device or folder and they may need to be created.
	folderIdx, err := db.folderIdxLocked(folder)
	if err != nil {
		return 0, fmt.Errorf("indexID (folderIdx): %w", err)
	}
	deviceIdx, err := db.deviceIdxLocked(device)
	if err != nil {
		return 0, fmt.Errorf("indexID (deviceIdx): %w", err)
	}

	// Create a read-only transaction, which will be upgraded to a
	// read-write transction if required to set a new index ID.
	tx, err := db.sql.BeginTxx(context.Background(), &sql.TxOptions{Isolation: sql.LevelReadUncommitted, ReadOnly: false})
	if err != nil {
		return 0, fmt.Errorf("indexID (begin): %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	var indexID int64
	if err := tx.Get(&indexID, `
		SELECT index_id FROM index_ids WHERE folder_idx = ? AND device_idx = ?`,
		folderIdx, deviceIdx,
	); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("indexID (get): %w", err)
	}

	if indexID == 0 && device == protocol.LocalDeviceID {
		// Generate a new index ID
		indexID = int64(protocol.NewIndexID()) //nolint:gosec
		if _, err := tx.Exec(`INSERT INTO index_ids (folder_idx, device_idx, index_id) values (?, ?, ?)`,
			folderIdx, deviceIdx, indexID,
		); err != nil {
			return 0, fmt.Errorf("indexID (insert): %w", err)
		}
		if err := tx.Commit(); err != nil {
			return 0, fmt.Errorf("indexID (commit): %w", err)
		}
	}

	return protocol.IndexID(indexID), nil //nolint:gosec
}

func (db *DB) SetIndexID(folder string, device protocol.DeviceID, id protocol.IndexID) error {
	folderIdx, err := db.folderIdxLocked(folder)
	if err != nil {
		return fmt.Errorf("indexID (folderIdx): %w", err)
	}
	deviceIdx, err := db.deviceIdxLocked(device)
	if err != nil {
		return fmt.Errorf("indexID (deviceIdx): %w", err)
	}

	if _, err := db.sql.Exec(`INSERT OR REPLACE INTO index_ids (folder_idx, device_idx, index_id) values (?, ?, ?)`,
		folderIdx, deviceIdx, int64(id), //nolint:gosec
	); err != nil {
		return fmt.Errorf("indexID (insert): %w", err)
	}
	return nil
}

func (db *DB) DropIndexIDs() error {
	_, err := db.sql.Exec(`DELETE FROM index_ids`)
	return wrap("drop index IDs", err)
}
