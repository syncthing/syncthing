// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package versioner

import (
	"fmt"
	"strconv"
	"time"

	"github.com/syncthing/syncthing/lib/fs"
)

func init() {
	// Register the constructor for this type of versioner
	Factories["trashcan"] = NewTrashcan
}

type Trashcan struct {
	fs           fs.Filesystem
	cleanoutDays int
	stop         chan struct{}
}

func NewTrashcan(folderID string, fs fs.Filesystem, params map[string]string) Versioner {
	cleanoutDays, _ := strconv.Atoi(params["cleanoutDays"])
	// On error we default to 0, "do not clean out the trash can"

	s := &Trashcan{
		fs:           fs,
		cleanoutDays: cleanoutDays,
		stop:         make(chan struct{}),
	}

	l.Debugf("instantiated %#v", s)
	return s
}

// Archive moves the named file away to a version archive. If this function
// returns nil, the named file does not exist any more (has been archived).
func (t *Trashcan) Archive(filePath string) error {
	return archiveFile(t.fs, t.fs, ".stversions", filePath)
}

func (t *Trashcan) Serve() {
	l.Debugln(t, "starting")
	defer l.Debugln(t, "stopping")

	// Do the first cleanup one minute after startup.
	timer := time.NewTimer(time.Minute)
	defer timer.Stop()

	for {
		select {
		case <-t.stop:
			return

		case <-timer.C:
			if t.cleanoutDays > 0 {
				if err := t.cleanoutArchive(); err != nil {
					l.Infoln("Cleaning trashcan:", err)
				}
			}

			// Cleanups once a day should be enough.
			timer.Reset(24 * time.Hour)
		}
	}
}

func (t *Trashcan) Stop() {
	close(t.stop)
}

func (t *Trashcan) String() string {
	return fmt.Sprintf("trashcan@%p", t)
}

func (t *Trashcan) cleanoutArchive() error {
	versionsDir := ".stversions"
	if _, err := t.fs.Lstat(versionsDir); fs.IsNotExist(err) {
		return nil
	}

	cutoff := time.Now().Add(time.Duration(-24*t.cleanoutDays) * time.Hour)
	dirTracker := make(emptyDirTracker)

	walkFn := func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() && !info.IsSymlink() {
			dirTracker.addDir(path)
			return nil
		}

		if info.ModTime().Before(cutoff) {
			// The file is too old; remove it.
			t.fs.Remove(path)
		} else {
			// Keep this file, and remember it so we don't unnecessarily try
			// to remove this directory.
			dirTracker.addFile(path)
		}
		return nil
	}

	if err := t.fs.Walk(versionsDir, walkFn); err != nil {
		return err
	}

	dirTracker.deleteEmptyDirs(t.fs)

	return nil
}

func (t *Trashcan) GetVersions() (map[string][]FileVersion, error) {
	return retrieveVersions(t.fs, ".stversions")
}

func (t *Trashcan) Restore(filepath string, versionTime time.Time) error {
	return restoreFile(t.fs, t.fs, ".stversions", filepath, versionTime)
}
