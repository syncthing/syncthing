// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package sqlite

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/syncthing/syncthing/internal/slogutil"
)

func (s *DB) DropFolder(folder string) error {
	s.folderDBsMut.Lock()
	defer s.folderDBsMut.Unlock()
	s.updateLock.Lock()
	defer s.updateLock.Unlock()
	_, err := s.stmt(`
		DELETE FROM folders
		WHERE folder_id = ?
	`).Exec(folder)
	if fdb, ok := s.folderDBs[folder]; ok {
		fdb.Close()
		_ = os.Remove(fdb.path)
		_ = os.Remove(fdb.path + "-wal")
		_ = os.Remove(fdb.path + "-shm")
		delete(s.folderDBs, folder)
	}
	return wrap(err)
}

func (s *DB) ListFolders() ([]string, error) {
	var res []string
	err := s.stmt(`
		SELECT folder_id FROM folders
		ORDER BY folder_id
	`).Select(&res)
	return res, wrap(err)
}

// cleanDroppedFolders removes old database files for folders that no longer
// exist in the main database.
func (s *DB) cleanDroppedFolders() error {
	// All expected folder databeses.
	var names []string
	err := s.stmt(`SELECT database_name FROM folders`).Select(&names)
	if err != nil {
		return wrap(err)
	}

	// All folder database files on disk.
	files, err := filepath.Glob(filepath.Join(s.pathBase, "folder.*"))
	if err != nil {
		return wrap(err)
	}

	// Any files that don't match a name in the database are removed.
	for _, file := range files {
		base := filepath.Base(file)
		inDB := slices.ContainsFunc(names, func(name string) bool { return strings.HasPrefix(base, name) })
		if !inDB {
			if err := os.Remove(file); err != nil {
				slog.Warn("Failed to remove database file for old, dropped folder", slogutil.FilePath(base))
			} else {
				slog.Info("Cleaned out database file for old, dropped folder", slogutil.FilePath(base))
			}
		}
	}
	return nil
}

// startFolderDatabases loads all existing folder databases, thus causing
// migrations to apply.
func (s *DB) startFolderDatabases() error {
	ids, err := s.ListFolders()
	if err != nil {
		return wrap(err)
	}
	for _, id := range ids {
		_, err := s.getFolderDB(id, false)
		if err != nil && !errors.Is(err, errNoSuchFolder) {
			return wrap(err)
		}
	}
	return nil
}

// wrap returns the error wrapped with the calling function name and
// optional extra context strings as prefix. A nil error wraps to nil.
func wrap(err error, context ...string) error {
	if err == nil {
		return nil
	}

	prefix := "error"
	pc, _, _, ok := runtime.Caller(1)
	details := runtime.FuncForPC(pc)
	if ok && details != nil {
		prefix = strings.ToLower(details.Name())
		if dotIdx := strings.LastIndex(prefix, "."); dotIdx > 0 {
			prefix = prefix[dotIdx+1:]
		}
	}

	if len(context) > 0 {
		for i := range context {
			context[i] = strings.TrimSpace(context[i])
		}
		extra := strings.Join(context, ", ")
		return fmt.Errorf("%s (%s): %w", prefix, extra, err)
	}

	return fmt.Errorf("%s: %w", prefix, err)
}
