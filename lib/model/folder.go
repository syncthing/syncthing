// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package model

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/syncthing/syncthing/lib/ignore"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/versioner"
)

type folder struct {
	stateTracker
	scan  folderScanner
	model *Model
	stop  chan struct{}
}

func (f *folder) IndexUpdated() {
}

func (f *folder) DelayScan(next time.Duration) {
	f.scan.Delay(next)
}

func (f *folder) Scan(subdirs []string) error {
	return f.scan.Scan(subdirs)
}
func (f *folder) Stop() {
	close(f.stop)
}

func (f *folder) Jobs() ([]string, []string) {
	return nil, nil
}

func (f *folder) BringToFront(string) {}

func (f *folder) scanSubdirsIfHealthy(subDirs []string) error {
	if err := f.model.CheckFolderHealth(f.folderID); err != nil {
		l.Infoln("Skipping folder", f.folderID, "scan due to folder error:", err)
		return err
	}
	l.Debugln(f, "Scanning subdirectories")
	if err := f.model.internalScanFolderSubdirs(f.folderID, subDirs); err != nil {
		// Potentially sets the error twice, once in the scanner just
		// by doing a check, and once here, if the error returned is
		// the same one as returned by CheckFolderHealth, though
		// duplicate set is handled by setError.
		f.setError(err)
		return err
	}
	return nil
}

// A function to provide the ability to validate and modify local changes,
// before they are committed to the database
// Default behavior is to apply the changes as-is, overwrite function as needed
func (f *folder) validateAndUpdateLocalChanges(fs []protocol.FileInfo) []protocol.FileInfo {
	// update the database
	f.model.updateLocals(f.folderID, fs)

	return fs
}

// moveforconflict renames a file to deal with sync conflicts
func (f *folder) moveForConflict(name string, maxConflicts int) error {
	if strings.Contains(filepath.Base(name), ".sync-conflict-") {
		l.Infoln("Conflict for", name, "which is already a conflict copy; not copying again.")
		if err := os.Remove(name); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}

	if maxConflicts == 0 {
		if err := os.Remove(name); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}

	ext := filepath.Ext(name)
	withoutExt := name[:len(name)-len(ext)]
	newName := withoutExt + time.Now().Format(".sync-conflict-20060102-150405") + ext
	err := os.Rename(name, newName)
	if os.IsNotExist(err) {
		// We were supposed to move a file away but it does not exist. Either
		// the user has already moved it away, or the conflict was between a
		// remote modification and a local delete. In either way it does not
		// matter, go ahead as if the move succeeded.
		err = nil
	}
	if maxConflicts > -1 {
		matches, gerr := osutil.Glob(withoutExt + ".sync-conflict-????????-??????" + ext)
		if gerr == nil && len(matches) > maxConflicts {
			sort.Sort(sort.Reverse(sort.StringSlice(matches)))
			for _, match := range matches[maxConflicts:] {
				gerr = os.Remove(match)
				if gerr != nil {
					l.Debugln("removing extra conflict", gerr)
				}
			}
		} else if gerr != nil {
			l.Debugln("globbing for conflicts", gerr)
		}
	}
	return err
}

// deleteDir attempts to delete the given directory
func (f *folder) deleteDir(folderPath string, file protocol.FileInfo, matcher *ignore.Matcher) error {
	realName, err := rootedJoinedPath(folderPath, file.Name)
	if err != nil {
		return err
	}

	// Delete any temporary files lying around in the directory
	dir, _ := os.Open(realName)
	if dir != nil {
		files, _ := dir.Readdirnames(-1)
		for _, dirFile := range files {
			fullDirFile := filepath.Join(file.Name, dirFile)
			if ignore.IsTemporary(dirFile) || (matcher != nil &&
				matcher.Match(fullDirFile).IsDeletable()) {
				os.RemoveAll(filepath.Join(folderPath, fullDirFile))
			}
		}
		dir.Close()
	}
	return osutil.InWritableDir(os.Remove, realName)
}

// deleteFile attempts to delete the given file
// Takes sync conflict and versioning settings into account
func (f *folder) deleteFile(folderPath string, file protocol.FileInfo, ver versioner.Versioner, maxConflicts int) error {
	var err error

	realName, err := rootedJoinedPath(folderPath, file.Name)
	if err != nil {
		return err
	}

	cur, ok := f.model.CurrentFolderFile(f.folderID, file.Name)
	if ok && f.inConflict(cur.Version, file.Version) && maxConflicts > 0 {
		// There is a conflict here. Move the file to a conflict copy instead
		// of deleting.
		file.Version = file.Version.Merge(cur.Version)
		err = osutil.InWritableDir(func(path string) error {
			return f.moveForConflict(realName, maxConflicts)
		}, realName)
	} else if ver != nil {
		err = osutil.InWritableDir(ver.Archive, realName)
	} else {
		err = osutil.InWritableDir(os.Remove, realName)
	}
	return err
}

func (f *folder) inConflict(current, replacement protocol.Vector) bool {
	if current.Concurrent(replacement) {
		// Obvious case
		return true
	}
	if replacement.Counter(f.model.shortID) > current.Counter(f.model.shortID) {
		// The replacement file contains a higher version for ourselves than
		// what we have. This isn't supposed to be possible, since it's only
		// we who can increment that counter. We take it as a sign that
		// something is wrong (our index may have been corrupted or removed)
		// and flag it as a conflict.
		return true
	}
	return false
}
