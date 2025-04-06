// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"iter"
	"path/filepath"
	"time"

	"github.com/syncthing/syncthing/internal/db"
	"github.com/syncthing/syncthing/internal/itererr"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/protocol"
)

var errNoSuchFolder = errors.New("no such folder")

func (s *DB) getFolderDB(folder string, create bool) (*folderDB, error) {
	// Check for an already open database
	s.folderDBsMut.RLock()
	fdb, ok := s.folderDBs[folder]
	s.folderDBsMut.RUnlock()
	if ok {
		return fdb, nil
	}

	// Check for an existing folder. If we're not supposed to create the
	// folder, we don't move on if it doesn't already have an ID.
	var idx int64
	if err := s.stmt(`
		SELECT idx FROM folders
		WHERE folder_id = ?
	`).Get(&idx, folder); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, wrap(err)
	}
	if idx == 0 && !create {
		return nil, errNoSuchFolder
	}

	// Create a folder ID and database if it does not already exist
	s.folderDBsMut.Lock()
	defer s.folderDBsMut.Unlock()
	if fdb, ok := s.folderDBs[folder]; ok {
		return fdb, nil
	}

	var err error
	if idx == 0 {
		// First time we want to access this folder, need to create a new
		// folder ID
		idx, err = s.folderIdxLocked(folder)
		if err != nil {
			return nil, err
		}
	}

	name := fmt.Sprintf("folder.%04x.db", idx)
	path := filepath.Join(s.pathBase, name)
	fdb, err = s.folderDBOpener(folder, path, s.deleteRetention)
	if err != nil {
		return nil, err
	}
	s.folderDBs[folder] = fdb
	return fdb, nil
}

func (s *DB) Update(folder string, device protocol.DeviceID, fs []protocol.FileInfo) error {
	fdb, err := s.getFolderDB(folder, true)
	if err != nil {
		return err
	}
	return fdb.Update(device, fs)
}

func (s *DB) GetDeviceFile(folder string, device protocol.DeviceID, file string) (protocol.FileInfo, bool, error) {
	fdb, err := s.getFolderDB(folder, false)
	if errors.Is(err, errNoSuchFolder) {
		return protocol.FileInfo{}, false, nil
	}
	if err != nil {
		return protocol.FileInfo{}, false, err
	}
	return fdb.GetDeviceFile(device, file)
}

func (s *DB) GetGlobalAvailability(folder, file string) ([]protocol.DeviceID, error) {
	fdb, err := s.getFolderDB(folder, false)
	if errors.Is(err, errNoSuchFolder) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return fdb.GetGlobalAvailability(file)
}

func (s *DB) GetGlobalFile(folder string, file string) (protocol.FileInfo, bool, error) {
	fdb, err := s.getFolderDB(folder, false)
	if errors.Is(err, errNoSuchFolder) {
		return protocol.FileInfo{}, false, nil
	}
	if err != nil {
		return protocol.FileInfo{}, false, err
	}
	return fdb.GetGlobalFile(file)
}

func (s *DB) AllGlobalFiles(folder string) (iter.Seq[db.FileMetadata], func() error) {
	fdb, err := s.getFolderDB(folder, false)
	if errors.Is(err, errNoSuchFolder) {
		return func(yield func(db.FileMetadata) bool) {}, func() error { return nil }
	}
	if err != nil {
		return func(yield func(db.FileMetadata) bool) {}, func() error { return err }
	}
	return fdb.AllGlobalFiles()
}

func (s *DB) AllGlobalFilesPrefix(folder string, prefix string) (iter.Seq[db.FileMetadata], func() error) {
	fdb, err := s.getFolderDB(folder, false)
	if errors.Is(err, errNoSuchFolder) {
		return func(yield func(db.FileMetadata) bool) {}, func() error { return nil }
	}
	if err != nil {
		return func(yield func(db.FileMetadata) bool) {}, func() error { return err }
	}
	return fdb.AllGlobalFilesPrefix(prefix)
}

func (s *DB) AllLocalBlocksWithHash(hash []byte) ([]db.BlockMapEntry, error) {
	var entries []db.BlockMapEntry
	err := s.forEachFolder(func(fdb *folderDB) error {
		es, err := itererr.Collect(fdb.AllLocalBlocksWithHash(hash))
		entries = append(entries, es...)
		return err
	})
	return entries, err
}

func (s *DB) AllLocalFilesWithBlocksHashAnyFolder(hash []byte) (map[string][]db.FileMetadata, error) {
	res := make(map[string][]db.FileMetadata)
	err := s.forEachFolder(func(fdb *folderDB) error {
		files, err := itererr.Collect(fdb.AllLocalFilesWithBlocksHash(hash))
		res[fdb.folderID] = files
		return err
	})
	return res, err
}

func (s *DB) AllLocalFiles(folder string, device protocol.DeviceID) (iter.Seq[protocol.FileInfo], func() error) {
	fdb, err := s.getFolderDB(folder, false)
	if errors.Is(err, errNoSuchFolder) {
		return func(yield func(protocol.FileInfo) bool) {}, func() error { return nil }
	}
	if err != nil {
		return func(yield func(protocol.FileInfo) bool) {}, func() error { return err }
	}
	return fdb.AllLocalFiles(device)
}

func (s *DB) AllLocalFilesBySequence(folder string, device protocol.DeviceID, startSeq int64, limit int) (iter.Seq[protocol.FileInfo], func() error) {
	fdb, err := s.getFolderDB(folder, false)
	if errors.Is(err, errNoSuchFolder) {
		return func(yield func(protocol.FileInfo) bool) {}, func() error { return nil }
	}
	if err != nil {
		return func(yield func(protocol.FileInfo) bool) {}, func() error { return err }
	}
	return fdb.AllLocalFilesBySequence(device, startSeq, limit)
}

func (s *DB) AllLocalFilesWithPrefix(folder string, device protocol.DeviceID, prefix string) (iter.Seq[protocol.FileInfo], func() error) {
	fdb, err := s.getFolderDB(folder, false)
	if errors.Is(err, errNoSuchFolder) {
		return func(yield func(protocol.FileInfo) bool) {}, func() error { return nil }
	}
	if err != nil {
		return func(yield func(protocol.FileInfo) bool) {}, func() error { return err }
	}
	return fdb.AllLocalFilesWithPrefix(device, prefix)
}

func (s *DB) AllLocalFilesWithBlocksHash(folder string, h []byte) (iter.Seq[db.FileMetadata], func() error) {
	fdb, err := s.getFolderDB(folder, false)
	if errors.Is(err, errNoSuchFolder) {
		return func(yield func(db.FileMetadata) bool) {}, func() error { return nil }
	}
	if err != nil {
		return func(yield func(db.FileMetadata) bool) {}, func() error { return err }
	}
	return fdb.AllLocalFilesWithBlocksHash(h)
}

func (s *DB) AllNeededGlobalFiles(folder string, device protocol.DeviceID, order config.PullOrder, limit, offset int) (iter.Seq[protocol.FileInfo], func() error) {
	fdb, err := s.getFolderDB(folder, false)
	if errors.Is(err, errNoSuchFolder) {
		return func(yield func(protocol.FileInfo) bool) {}, func() error { return nil }
	}
	if err != nil {
		return func(yield func(protocol.FileInfo) bool) {}, func() error { return err }
	}
	return fdb.AllNeededGlobalFiles(device, order, limit, offset)
}

func (s *DB) DropAllFiles(folder string, device protocol.DeviceID) error {
	fdb, err := s.getFolderDB(folder, false)
	if errors.Is(err, errNoSuchFolder) {
		return nil
	}
	if err != nil {
		return err
	}
	return fdb.DropAllFiles(device)
}

func (s *DB) DropFilesNamed(folder string, device protocol.DeviceID, names []string) error {
	fdb, err := s.getFolderDB(folder, false)
	if errors.Is(err, errNoSuchFolder) {
		return nil
	}
	if err != nil {
		return err
	}
	return fdb.DropFilesNamed(device, names)
}

func (s *DB) ListDevicesForFolder(folder string) ([]protocol.DeviceID, error) {
	fdb, err := s.getFolderDB(folder, false)
	if errors.Is(err, errNoSuchFolder) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return fdb.ListDevicesForFolder()
}

func (s *DB) RemoteSequences(folder string) (map[protocol.DeviceID]int64, error) {
	fdb, err := s.getFolderDB(folder, false)
	if errors.Is(err, errNoSuchFolder) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return fdb.RemoteSequences()
}

func (s *DB) CountGlobal(folder string) (db.Counts, error) {
	fdb, err := s.getFolderDB(folder, false)
	if errors.Is(err, errNoSuchFolder) {
		return db.Counts{}, nil
	}
	if err != nil {
		return db.Counts{}, err
	}
	return fdb.CountGlobal()
}

func (s *DB) CountLocal(folder string, device protocol.DeviceID) (db.Counts, error) {
	fdb, err := s.getFolderDB(folder, false)
	if errors.Is(err, errNoSuchFolder) {
		return db.Counts{}, nil
	}
	if err != nil {
		return db.Counts{}, err
	}
	return fdb.CountLocal(device)
}

func (s *DB) CountNeed(folder string, device protocol.DeviceID) (db.Counts, error) {
	fdb, err := s.getFolderDB(folder, false)
	if errors.Is(err, errNoSuchFolder) {
		return db.Counts{}, nil
	}
	if err != nil {
		return db.Counts{}, err
	}
	return fdb.CountNeed(device)
}

func (s *DB) CountReceiveOnlyChanged(folder string) (db.Counts, error) {
	fdb, err := s.getFolderDB(folder, false)
	if errors.Is(err, errNoSuchFolder) {
		return db.Counts{}, nil
	}
	if err != nil {
		return db.Counts{}, err
	}
	return fdb.CountReceiveOnlyChanged()
}

func (s *DB) DropAllIndexIDs() error {
	return s.forEachFolder(func(fdb *folderDB) error {
		return fdb.DropAllIndexIDs()
	})
}

func (s *DB) GetIndexID(folder string, device protocol.DeviceID) (protocol.IndexID, error) {
	fdb, err := s.getFolderDB(folder, true)
	if err != nil {
		return 0, err
	}
	return fdb.GetIndexID(device)
}

func (s *DB) SetIndexID(folder string, device protocol.DeviceID, id protocol.IndexID) error {
	fdb, err := s.getFolderDB(folder, true)
	if err != nil {
		return err
	}
	return fdb.SetIndexID(device, id)
}

func (s *DB) GetDeviceSequence(folder string, device protocol.DeviceID) (int64, error) {
	fdb, err := s.getFolderDB(folder, false)
	if errors.Is(err, errNoSuchFolder) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return fdb.GetDeviceSequence(device)
}

func (s *DB) DeleteMtime(folder, name string) error {
	fdb, err := s.getFolderDB(folder, false)
	if errors.Is(err, errNoSuchFolder) {
		return nil
	}
	if err != nil {
		return err
	}
	return fdb.DeleteMtime(name)
}

func (s *DB) GetMtime(folder, name string) (ondisk, virtual time.Time) {
	fdb, err := s.getFolderDB(folder, false)
	if errors.Is(err, errNoSuchFolder) {
		return time.Time{}, time.Time{}
	}
	if err != nil {
		return time.Time{}, time.Time{}
	}
	return fdb.GetMtime(name)
}

func (s *DB) PutMtime(folder, name string, ondisk, virtual time.Time) error {
	fdb, err := s.getFolderDB(folder, true)
	if err != nil {
		return err
	}
	return fdb.PutMtime(name, ondisk, virtual)
}

func (s *DB) DropDevice(device protocol.DeviceID) error {
	return s.forEachFolder(func(fdb *folderDB) error {
		return fdb.DropDevice(device)
	})
}

// forEachFolder runs the function for each currently open folderDB,
// returning the first error that was encountered.
func (s *DB) forEachFolder(fn func(fdb *folderDB) error) error {
	folders, err := s.ListFolders()
	if err != nil {
		return err
	}

	var firstError error
	for _, folder := range folders {
		fdb, err := s.getFolderDB(folder, false)
		if err != nil {
			if firstError == nil {
				firstError = err
			}
			continue
		}
		if err := fn(fdb); err != nil && firstError == nil {
			firstError = err
		}
	}
	return firstError
}
