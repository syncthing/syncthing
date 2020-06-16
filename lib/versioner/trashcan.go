// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package versioner

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/thejerf/suture"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/util"
)

func init() {
	// Register the constructor for this type of versioner
	factories["trashcan"] = newTrashcan
}

type trashcan struct {
	suture.Service
	folderFs        fs.Filesystem
	versionsFs      fs.Filesystem
	cleanoutDays    int
	copyRangeMethod fs.CopyRangeMethod
}

func newTrashcan(cfg config.FolderConfiguration) Versioner {
	cleanoutDays, _ := strconv.Atoi(cfg.Versioning.Params["cleanoutDays"])
	// On error we default to 0, "do not clean out the trash can"

	s := &trashcan{
		folderFs:        cfg.Filesystem(),
		versionsFs:      versionerFsFromFolderCfg(cfg),
		cleanoutDays:    cleanoutDays,
		copyRangeMethod: cfg.CopyRangeMethod,
	}
	s.Service = util.AsService(s.serve, s.String())

	l.Debugf("instantiated %#v", s)
	return s
}

// Archive moves the named file away to a version archive. If this function
// returns nil, the named file does not exist any more (has been archived).
func (t *trashcan) Archive(filePath string) error {
	return archiveFile(t.copyRangeMethod, t.folderFs, t.versionsFs, filePath, func(name, tag string) string {
		return name
	})
}

func (t *trashcan) serve(ctx context.Context) {
	l.Debugln(t, "starting")
	defer l.Debugln(t, "stopping")

	// Do the first cleanup one minute after startup.
	timer := time.NewTimer(time.Minute)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
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

func (t *trashcan) String() string {
	return fmt.Sprintf("trashcan@%p", t)
}

func (t *trashcan) cleanoutArchive() error {
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

func (t *trashcan) GetVersions() (map[string][]FileVersion, error) {
	return retrieveVersions(t.versionsFs)
}

func (t *trashcan) Restore(filepath string, versionTime time.Time) error {
	// If we have an untagged file A and want to restore it on top of existing file A, we can't first archive the
	// existing A as we'd overwrite the old A version, therefore when we archive existing file, we archive it with a
	// tag but when the restoration is finished, we rename it (untag it). This is only important if when restoring A,
	// there already exists a file at the same location

	taggedName := ""
	tagger := func(name, tag string) string {
		// We also abuse the fact that tagger gets called twice, once for tagging the restoration version, which
		// should just return the plain name, and second time by archive which archives existing file in the folder.
		// We can't use TagFilename here, as restoreFile would discover that as a valid version and restore that instead.
		if taggedName != "" {
			return taggedName
		}

		taggedName = fs.TempName(name)
		return name
	}

	err := restoreFile(t.copyRangeMethod, t.versionsFs, t.folderFs, filepath, versionTime, tagger)
	if taggedName == "" {
		return err
	}

	return t.versionsFs.Rename(taggedName, filepath)
}
