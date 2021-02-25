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
	interval        [4]interval
	copyRangeMethod fs.CopyRangeMethod
}

func newStaggered(cfg config.FolderConfiguration) Versioner {
	params := cfg.Versioning.Params
	maxAge, err := strconv.ParseInt(params["maxAge"], 10, 0)
	if err != nil {
		maxAge = 31536000 // Default: ~1 year
	}

	versionsFs := versionerFsFromFolderCfg(cfg)

	s := &staggered{
		folderFs:   cfg.Filesystem(),
		versionsFs: versionsFs,
		interval: [4]interval{
			{30, 60 * 60},                     // first hour -> 30 sec between versions
			{60 * 60, 24 * 60 * 60},           // next day -> 1 h between versions
			{24 * 60 * 60, 30 * 24 * 60 * 60}, // next 30 days -> 1 day between versions
			{7 * 24 * 60 * 60, maxAge},        // next year -> 1 week between versions
		},
		copyRangeMethod: cfg.CopyRangeMethod,
	}

	l.Debugf("instantiated %#v", s)
	return s
}

func (v *staggered) Clean(ctx context.Context) error {
	l.Debugln("Versioner clean: Cleaning", v.versionsFs)

	if _, err := v.versionsFs.Stat("."); fs.IsNotExist(err) {
		// There is no need to clean a nonexistent dir.
		return nil
	}

	versionsPerFile := make(map[string][]string)
	dirTracker := make(emptyDirTracker)

	walkFn := func(path string, f fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if f.IsDir() && !f.IsSymlink() {
			dirTracker.addDir(path)
			return nil
		}

		// Regular file, or possibly a symlink.
		dirTracker.addFile(path)

		name, _ := UntagFilename(path)
		if name == "" {
			return nil
		}

		versionsPerFile[name] = append(versionsPerFile[name], path)

		return nil
	}

	if err := v.versionsFs.Walk(".", walkFn); err != nil {
		l.Warnln("Versioner: error scanning versions dir", err)
		return err
	}

	for _, versionList := range versionsPerFile {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		v.expire(versionList)
	}

	dirTracker.deleteEmptyDirs(v.versionsFs)

	l.Debugln("Cleaner: Finished cleaning", v.versionsFs)
	return nil
}

func (v *staggered) expire(versions []string) {
	l.Debugln("Versioner: Expiring versions", versions)
	for _, file := range v.toRemove(versions, time.Now()) {
		if fi, err := v.versionsFs.Lstat(file); err != nil {
			l.Warnln("versioner:", err)
			continue
		} else if fi.IsDir() {
			l.Infof("non-file %q is named like a file version", file)
			continue
		}

		if err := v.versionsFs.Remove(file); err != nil {
			l.Warnf("Versioner: can't remove %q: %v", file, err)
		}
	}
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

	v.expire(findAllVersions(v.versionsFs, filePath))

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
