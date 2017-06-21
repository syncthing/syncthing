// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/ignore"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/versioner"
)

type folder struct {
	stateTracker
	config.FolderConfiguration
  
	scan      folderScanner
	model     *Model
	ctx                 context.Context
	cancel              context.CancelFunc
	initialScanFinished chan struct{}
	stop      chan struct{}
	mtimeFS   *fs.MtimeFS
	versioner versioner.Versioner
	dbUpdates chan dbUpdateJob

func (f *folder) IndexUpdated() {
}

func (f *folder) DelayScan(next time.Duration) {
	f.scan.Delay(next)
}

func (f *folder) Scan(subdirs []string) error {
	<-f.initialScanFinished
	return f.scan.Scan(subdirs)
}

func (f *folder) Stop() {
	f.cancel()
}

func (f *folder) Jobs() ([]string, []string) {
	return nil, nil
}

func (f *folder) BringToFront(string) {}

func (f *folder) scanSubdirs(subDirs []string) error {
	if err := f.model.internalScanFolderSubdirs(f.ctx, f.folderID, subDirs); err != nil {
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
func (f *folder) moveForConflict(name string) error {
	if strings.Contains(filepath.Base(name), ".sync-conflict-") {
		l.Infoln("Conflict for", name, "which is already a conflict copy; not copying again.")
		if err := os.Remove(name); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}

	if f.MaxConflicts == 0 {
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
	if f.MaxConflicts > -1 {
		matches, gerr := osutil.Glob(withoutExt + ".sync-conflict-????????-??????" + ext)
		if gerr == nil && len(matches) > f.MaxConflicts {
			sort.Sort(sort.Reverse(sort.StringSlice(matches)))
			for _, match := range matches[f.MaxConflicts:] {
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
func (f *folder) deleteFile(folderPath string, file protocol.FileInfo) error {
	var err error

	realName, err := rootedJoinedPath(folderPath, file.Name)
	if err != nil {
		return err
	}

	cur, ok := f.model.CurrentFolderFile(f.folderID, file.Name)
	if ok && f.inConflict(cur.Version, file.Version) && f.MaxConflicts > 0 {
		// There is a conflict here. Move the file to a conflict copy instead
		// of deleting.
		file.Version = file.Version.Merge(cur.Version)
		err = osutil.InWritableDir(func(path string) error {
			return f.moveForConflict(realName)
		}, realName)
	} else if f.versioner != nil {
		err = osutil.InWritableDir(f.versioner.Archive, realName)
	} else {
		err = osutil.InWritableDir(os.Remove, realName)
	}

	// Update the file in the db if an update channel exists
	if f.dbUpdates != nil && f.mtimeFS != nil {
		if err == nil || os.IsNotExist(err) {
			// It was removed or it doesn't exist to start with
			f.dbUpdates <- dbUpdateJob{file, dbUpdateDeleteFile}
			err = nil
		} else if _, serr := f.mtimeFS.Lstat(realName); serr != nil && !os.IsPermission(serr) {
			// We get an error just looking at the file, and it's not a permission
			// problem. Lets assume the error is in fact some variant of "file
			// does not exist" (possibly expressed as some parent being a file and
			// not a directory etc) and that the delete is handled.
			f.dbUpdates <- dbUpdateJob{file, dbUpdateDeleteFile}
			err = nil
		}
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

// Find all invalid files and force a new evaluation
func (f *folder) resetInvalidFiles() {
	folderFiles := f.model.folderFiles[f.folderID]

	var invalidFiles []protocol.FileInfo

	folderFiles.WithHave(protocol.LocalDeviceID, func(fi db.FileIntf) bool {
		f := fi.(protocol.FileInfo)

		if f.IsInvalid() {
			invalidFiles = append(invalidFiles, f)
		}
		return true
	})

	// set all file sizes to 0, to force a new evaluation
	for i := range invalidFiles {
		invalidFiles[i].Size = 0
		if ((f.RevertLocalChanges && f.Type == config.FolderTypeReceiveOnly) || f.Type != config.FolderTypeReceiveOnly) && invalidFiles[i].Deleted {
			// restore deleted files
			invalidFiles[i].Version = protocol.Vector{}
		}
	}

	// Update the database
	folderFiles.Update(protocol.LocalDeviceID, invalidFiles)
}
