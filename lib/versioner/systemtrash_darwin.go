// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

//+build darwin

package versioner

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/sync"
)

// We trash files to the system trash can by finding the right trash can
// directory and then renaming the file to there. The "right trash can
// directory" is ~/.Trash for files on the same volume as ~ (usually the
// system volume), otherwise ${VolumeRoot}/.Trashes/${uid} for other
// volumes.
//
// We find the latter case by walking upwards towards the root, stopping
// when we find a .Trashes dir, and then double checking that we're still on
// the same volume.
//
// This fails in the presence of (for example) SMB mounts where the volume
// doesn't have a .Trashes folder. In that case we'll instead find the root
// /.Trashes, then note that it's not on the same device and fail. We then
// resort to simply deleting the file (rather, the puller overwrites it) as
// there is no system trash can there. This matches the behavior of the
// Finder as far as I can tell - except that it pops up a dialog asking
// wether it's OK to just delete the file instead of trashing it...

var (
	// Maps device identifier as returned by Lstat() to trash folder
	deviceIDtoTrashFolder    = make(map[int32]string)
	deviceIDtoTrashFolderMut = sync.NewRWMutex()
)

func init() {
	path, err := osutil.ExpandTilde("~/.Trash")
	if err != nil {
		return // We'll get errors logged from other places as this is pretty bad.
	}
	info, err := os.Lstat(path)
	if err != nil {
		return // Again, this is really weird!
	}

	dev, err := deviceID(info)
	if err != nil {
		l.Warnln("Cannot get device ID of home folder - system trash can versioning inoperable")
		return
	}

	deviceIDtoTrashFolder[dev] = path

	// Register the constructor for this type of versioner
	Factories["systemtrash"] = NewSystemTrashcan
}

type SystemTrashcan struct {
	folderPath   string
	cleanoutDays int
	stop         chan struct{}
}

func NewSystemTrashcan(folderID, folderPath string, params map[string]string) Versioner {
	return new(SystemTrashcan)
}

// Archive moves the named file to the system trashcan.
func (t *SystemTrashcan) Archive(path string) error {
	info, err := osutil.Lstat(path)
	if os.IsNotExist(err) {
		l.Debugln("not archiving nonexistent file", path)
		return nil
	} else if err != nil {
		return err
	}

	// Get the device identifier for the thing we want to move to trash
	devID, err := deviceID(info)
	if err != nil {
		return err
	}

	// Get the path to the trash folder
	trashFolder, err := getTrashFolder(path, devID)
	if err != nil {
		return err
	}

	// Move the file
	return osutil.Rename(path, filepath.Join(trashFolder, filepath.Base(path)))
}

// getTrashFolder returns the path to the correct trash folder, or an error
func getTrashFolder(path string, devID int32) (string, error) {
	// See if we already have a cached path for that device ID
	deviceIDtoTrashFolderMut.RLock()
	trashFolder, ok := deviceIDtoTrashFolder[devID]
	deviceIDtoTrashFolderMut.RUnlock()
	if ok {
		// Cache hit
		return trashFolder, nil
	}

	// Walk the directory tree to find the closest .Trashes dir.
	deviceIDtoTrashFolderMut.Lock()
	trashFolder, err := findTrashFolderForPath(path, devID)
	if err != nil {
		deviceIDtoTrashFolderMut.Unlock()
		return "", err
	}
	// Cache it.
	deviceIDtoTrashFolder[devID] = trashFolder
	deviceIDtoTrashFolderMut.Unlock()

	return trashFolder, nil
}

// findTrashFolderForPath attempts a filesystem walk to find the closest
// .Trashes folder on the same volume as the given path.
func findTrashFolderForPath(path string, devID int32) (string, error) {
	var prevPath string
	for path != prevPath {
		if info, err := os.Lstat(filepath.Join(path, ".Trashes")); err == nil {
			// We've found a .Trashes directory!
			folderDevID, err := deviceID(info)
			if err != nil {
				return "", err
			}
			if folderDevID != devID {
				// ... but it's not on the expected volume.
				return "", errors.New("traversed past volume root")
			}

			// Lets see if our uid directory exists, or try to create it.
			uid := os.Getuid()
			trashPath := filepath.Join(path, ".Trashes", strconv.Itoa(uid))
			if _, err := os.Lstat(trashPath); err != nil {
				if err := os.Mkdir(trashPath, 0700); err != nil {
					return "", err
				}
			}

			// All good.
			return trashPath, nil
		}
		prevPath = path
		path = filepath.Dir(path)
	}

	return "", errors.New("no trash folder found")
}

// The deviceID is a numeric identifier that uniquely identifies the mounted
// filesystem.
func deviceID(info os.FileInfo) (int32, error) {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return st.Dev, nil
	}

	return 0, errors.New("get device failed - not supported")
}
