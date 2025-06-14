// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package sqlite

import (
	"fmt"
	"os"
	"runtime"
	"strings"
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
