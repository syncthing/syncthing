// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package versioner

import (
	"context"
	"sort"
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

	cleanVersions(v.versionsFs, findAllVersions(v.versionsFs, filePath), v.toRemove)

	return nil
}

func (v simple) GetVersions() (map[string][]FileVersion, error) {
	return retrieveVersions(v.versionsFs)
}

func (v simple) Restore(filepath string, versionTime time.Time) error {
	return restoreFile(v.copyRangeMethod, v.versionsFs, v.folderFs, filepath, versionTime, TagFilename)
}

func (v simple) Clean(ctx context.Context) error {
	return clean(ctx, v.versionsFs, v.toRemove)
}

func (v simple) toRemove(versions []string, now time.Time) []string {
	var remove []string

	// The list of versions may or may not be properly sorted.
	sort.Strings(versions)

	// Too many versions: Remove the oldest ones above the treshold
	if len(versions) > v.keep {
		remove = versions[:len(versions)-v.keep]
		versions = versions[len(versions)-v.keep:] //can skip the elements we already are going to remove
	}

	// Not cleaning out based on cleanoutDays if it's set to 0 (or a negative value)
	if v.cleanoutDays <= 0 {
		return remove
	}

	// Check the rest, they can still be too old
	for _, version := range versions {
		versionTime, err := time.ParseInLocation(TimeFormat, extractTag(version), time.Local)
		if err != nil {
			l.Debugf("Versioner: file name %q is invalid: %v", version, err)
			continue
		}
		age := int64(now.Sub(versionTime).Seconds())
		maxAge := int64(v.cleanoutDays * 24 * 60 * 60)

		if age > maxAge {
			remove = append(remove, version)
		}
	}

	return remove
}
