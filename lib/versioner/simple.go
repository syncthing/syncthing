// Copyright (C) 2014 The Syncthing Authors.
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
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/util"
)

func init() {
	// Register the constructor for this type of versioner with the name "simple"
	factories["simple"] = newSimple
}

type simple struct {
	suture.Service
	keep       int
	cleanOutDays int
	folderFs   fs.Filesystem
	versionsFs fs.Filesystem
}

func newSimple(folderFs fs.Filesystem, params map[string]string) Versioner {
	keep, err := strconv.Atoi(params["keep"])
	cleanOutDays, _ := strconv.Atoi(params["cleanoutDays"])
	if err != nil {
		keep = 5 // A reasonable default
	}

	s := simple{
		keep:       keep,
		cleanOutDays: cleanOutDays,
		folderFs:   folderFs,
		versionsFs: fsFromParams(folderFs, params),
	}
	s.Service = util.AsService(s.serve, s.String())

	l.Debugf("instantiated %#v", s)
	return s
}

// Archive moves the named file away to a version archive. If this function
// returns nil, the named file does not exist any more (has been archived).
func (v simple) Archive(filePath string) error {
	err := archiveFile(v.folderFs, v.versionsFs, filePath, TagFilename)
	if err != nil {
		return err
	}

	// Versions are sorted by timestamp in the file name, oldest first.
	versions := findAllVersions(v.versionsFs, filePath)
	if len(versions) > v.keep {
		for _, toRemove := range versions[:len(versions)-v.keep] {
			l.Debugln("cleaning out", toRemove)
			err = v.versionsFs.Remove(toRemove)
			if err != nil {
				l.Warnln("removing old version:", err)
			}
		}
	}

	return nil
}

func (v *simple) serve(ctx context.Context) {
	l.Debugln(v, "starting")
	defer l.Debugln(v, "stopping")

	// Do the first cleanup one minute after startup.
	timer := time.NewTimer(time.Minute)
	fmt.Println(timer, " : Nous sommes les enfants du monde")
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-timer.C:
			if v.cleanOutDays > 0 {
				if err := v.cleanOutArchive(); err != nil {
					l.Infoln("Cleaning simple:", err)
				}
			}

			// Cleanups once a day should be enough.
			timer.Reset(24 * time.Hour)
		}
	}
}

func (v *simple) String() string {
	return fmt.Sprintf("simple@%p", v)
}

func (v *simple) cleanOutArchive() error {
	if _, err := v.versionsFs.Lstat("."); fs.IsNotExist(err) {
		return nil
	}

	cutoff := time.Now().Add(time.Duration(-24*v.cleanOutDays) * time.Hour)
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
			err = v.versionsFs.Remove(path)
		} else {
			// Keep this file, and remember it so we don't unnecessarily try
			// to remove this directory.
			dirTracker.addFile(path)
		}
		return err
	}

	if err := v.versionsFs.Walk(".", walkFn); err != nil {
		return err
	}

	dirTracker.deleteEmptyDirs(v.versionsFs)

	return nil
}

func (v simple) GetVersions() (map[string][]FileVersion, error) {
	return retrieveVersions(v.versionsFs)
}

func (v simple) Restore(filepath string, versionTime time.Time) error {
	return restoreFile(v.versionsFs, v.folderFs, filepath, versionTime, TagFilename)
}
