// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package versioner

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/fs"
)

func init() {
	// Register the constructor for this type of versioner with the name "staggered"
	factories["staggered"] = newStaggered
}

type interval struct {
	step int64
	end  int64
}

type staggered struct {
	folderFs        fs.Filesystem
	versionsFs      fs.Filesystem
	interval        [5]interval
	copyRangeMethod fs.CopyRangeMethod
}

func newStaggered(cfg config.FolderConfiguration) Versioner {
	params := cfg.Versioning.Params
	interval1, err := strconv.ParseInt(params["staggeredInterval1"], 10, 0)
	if err != nil {
		interval1 = 30 // Default: 30 seconds
	}
	period1, err := strconv.ParseInt(params["staggeredPeriod1"], 10, 0)
	if err != nil {
		period1 = 3600 // Default: 1 minute
	}
	interval2, err := strconv.ParseInt(params["staggeredInterval2"], 10, 0)
	if err != nil {
		interval2 = 3600 // Default: 1 hour
	}
	period2, err := strconv.ParseInt(params["staggeredPeriod2"], 10, 0)
	if err != nil {
		period2 = 86400 // Default: 1 day
	}
	interval3, err := strconv.ParseInt(params["staggeredInterval3"], 10, 0)
	if err != nil {
		interval3 = 86400 // Default: 1 day
	}
	period3, err := strconv.ParseInt(params["staggeredPeriod3"], 10, 0)
	if err != nil {
		period3 = 2592000 // Default: 1 month
	}
	interval4, err := strconv.ParseInt(params["staggeredInterval4"], 10, 0)
	if err != nil {
		interval4 = 604800 // Default: 1 week
	}
	period4, err := strconv.ParseInt(params["staggeredPeriod4"], 10, 0)
	if err != nil {
		period4 = 31536000  // Default: 1 year
	}
	interval5, err := strconv.ParseInt(params["staggeredInterval5"], 10, 0)
	if err != nil {
		interval5 = 2592000 // Default: 1 month
	}
	maxAge, err := strconv.ParseInt(params["maxAge"], 10, 0)
	if err != nil {
		maxAge = 31536000 // Default: 1 year
	}

	versionsFs := versionerFsFromFolderCfg(cfg)

	s := &staggered{
		folderFs:   cfg.Filesystem(nil),
		versionsFs: versionsFs,
		interval: [5]interval{
			{interval1, period1},         // first hour    -> 30 sec between versions
			{interval2, period2},         // first day     -> 1 h between versions
			{interval3, period3},         // first 30 days -> 1 day between versions
			{interval4, period4},         // first year    -> 1 week between versions
			{interval5, maxAge}, // next year     -> 1 month between versions
		},
		copyRangeMethod: cfg.CopyRangeMethod,
	}

	l.Debugf("instantiated %#v", s)
	return s
}

func (v *staggered) Clean(ctx context.Context) error {
	return clean(ctx, v.versionsFs, v.toRemove)
}

func (v *staggered) toRemove(versions []string, now time.Time) []string {
	var prevAge int64
	firstFile := true
	var remove []string

	// The list of versions may or may not be properly sorted.
	sort.Strings(versions)

	for _, version := range versions {
		versionTime, err := time.ParseInLocation(TimeFormat, extractTag(version), time.Local)
		if err != nil {
			l.Debugf("Versioner: file name %q is invalid: %v", version, err)
			continue
		}
		age := int64(now.Sub(versionTime).Seconds())

		// If the file is older than the max age of the last interval, remove it
		if lastIntv := v.interval[len(v.interval)-1]; lastIntv.end > 0 && age > lastIntv.end {
			l.Debugln("Versioner: File over maximum age -> delete ", version)
			remove = append(remove, version)
			continue
		}

		// If it's the first (oldest) file in the list we can skip the interval checks
		if firstFile {
			prevAge = age
			firstFile = false
			continue
		}

		// Find the interval the file fits in
		var usedInterval interval
		for _, usedInterval = range v.interval {
			if age < usedInterval.end {
				break
			}
		}

		if prevAge-age < usedInterval.step {
			l.Debugln("too many files in step -> delete", version)
			remove = append(remove, version)
			continue
		}

		prevAge = age
	}

	return remove
}

// Archive moves the named file away to a version archive. If this function
// returns nil, the named file does not exist any more (has been archived).
func (v *staggered) Archive(filePath string) error {
	if err := archiveFile(v.copyRangeMethod, v.folderFs, v.versionsFs, filePath, TagFilename); err != nil {
		return err
	}

	cleanVersions(v.versionsFs, findAllVersions(v.versionsFs, filePath), v.toRemove)

	return nil
}

func (v *staggered) GetVersions() (map[string][]FileVersion, error) {
	return retrieveVersions(v.versionsFs)
}

func (v *staggered) Restore(filepath string, versionTime time.Time) error {
	return restoreFile(v.copyRangeMethod, v.versionsFs, v.folderFs, filepath, versionTime, TagFilename)
}

func (v *staggered) String() string {
	return fmt.Sprintf("Staggered/@%p", v)
}
