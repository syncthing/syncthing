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
	folderFs     fs.Filesystem
	versionsFs   fs.Filesystem
	cleanoutDays int
	stop         chan struct{}
}

func NewTrashcan(folderID string, folderFs fs.Filesystem, params map[string]string) Versioner {
	cleanoutDays, _ := strconv.Atoi(params["cleanoutDays"])
	// On error we default to 0, "do not clean out the trash can"

	s := &Trashcan{
		folderFs:     folderFs,
		versionsFs:   fsFromParams(folderFs, params),
		cleanoutDays: cleanoutDays,
		stop:         make(chan struct{}),
	}

	l.Debugf("instantiated %#v", s)
	return s
}

// Archive moves the named file away to a version archive. If this function
// returns nil, the named file does not exist any more (has been archived).
func (t *Trashcan) Archive(filePath string) error {
	return archiveFile(t.folderFs, t.versionsFs, filePath, func(name, tag string) string {
		return name
	})
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
	if _, err := t.versionsFs.Lstat("."); fs.IsNotExist(err) {
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
			err = t.versionsFs.Remove(path)
		} else {
			// Keep this file, and remember it so we don't unnecessarily try
			// to remove this directory.
			dirTracker.addFile(path)
		}
		return err
	}

	if err := t.versionsFs.Walk(".", walkFn); err != nil {
		return err
	}

	dirTracker.deleteEmptyDirs(t.versionsFs)

	return nil
}

func (t *Trashcan) GetVersions() (map[string][]FileVersion, error) {
	return retrieveVersions(t.versionsFs)
}

func (t *Trashcan) Restore(filepath string, versionTime time.Time) error {
	// If we have an untagged file A and want to restore it on top of existing file A, we can't first archive the
	// existing A as we'd overwrite the old A version, therefore when we archive existing file, we archive it with a
	// tag but when the restoration is finished, we rename it (untag it). This is only important if when restoring A,
	// there already exists a file at the same location

	taggedName := ""
	tagger := func(name, tag string) string {
		// We can't use TagFilename here, as restoreFii would discover that as a valid version and restore that instead.
		taggedName = fs.TempName(name)
		return taggedName
	}

	err := restoreFile(t.versionsFs, t.folderFs, filepath, versionTime, tagger)
	if taggedName == "" {
		return err
	}

	return t.versionsFs.Rename(taggedName, filepath)
}
