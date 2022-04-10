// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package versioner

import (
	"context"
	"strconv"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/fs"
)

func init() {
	// Register the constructor for this type of versioner with the name "simple"
	factories["simple"] = newSimple
}

type simple struct {
	keep            int
	cleanoutDays    int
	folderFs        fs.Filesystem
	versionsFs      fs.Filesystem
	copyRangeMethod fs.CopyRangeMethod
}

func newSimple(cfg config.FolderConfiguration) Versioner {
	var keep, err = strconv.Atoi(cfg.Versioning.Params["keep"])
	cleanoutDays, _ := strconv.Atoi(cfg.Versioning.Params["cleanoutDays"])
	// On error we default to 0, "do not clean out the trash can"

	if err != nil {
		keep = 5 // A reasonable default
	}

	s := simple{
		keep:            keep,
		cleanoutDays:    cleanoutDays,
		folderFs:        cfg.Filesystem(nil),
		versionsFs:      versionerFsFromFolderCfg(cfg),
		copyRangeMethod: cfg.CopyRangeMethod,
	}

	l.Debugf("instantiated %#v", s)
	return s
}

// Archive moves the named file away to a version archive. If this function
// returns nil, the named file does not exist any more (has been archived).
func (v simple) Archive(filePath string) error {
	err := archiveFile(v.copyRangeMethod, v.folderFs, v.versionsFs, filePath, TagFilename)
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

func (v simple) GetVersions() (map[string][]FileVersion, error) {
	return retrieveVersions(v.versionsFs)
}

func (v simple) Restore(filepath string, versionTime time.Time) error {
	return restoreFile(v.copyRangeMethod, v.versionsFs, v.folderFs, filepath, versionTime, TagFilename)
}

func (v simple) Clean(ctx context.Context) error {
	return cleanByDay(ctx, v.versionsFs, v.cleanoutDays)
}
