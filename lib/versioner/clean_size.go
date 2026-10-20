// Copyright (C) 2026 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package versioner

import (
	"context"
	"sort"
	"time"

	"github.com/syncthing/syncthing/lib/fs"
)

// cleanByAgeAndSize removes expired versions, then the oldest remaining versions
// until their total size fits within maxSize. Nonpositive limits are disabled.
func cleanByAgeAndSize(ctx context.Context, versionsFs fs.Filesystem, cleanoutDays int, maxSize int64, versionTime func(string, fs.FileInfo) (time.Time, bool)) error {
	if cleanoutDays <= 0 && maxSize <= 0 {
		return nil
	}

	if _, err := versionsFs.Lstat("."); fs.IsNotExist(err) {
		return nil
	}

	cutoff := time.Now().Add(time.Duration(-24*cleanoutDays) * time.Hour)
	dirTracker := make(emptyDirTracker)
	type candidate struct {
		path        string
		size        int64
		versionTime time.Time
	}
	var files []candidate
	var totalSize int64

	walkFn := func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if info.IsDir() && !info.IsSymlink() {
			dirTracker.addDir(path)
			return nil
		}

		stamp, ok := versionTime(path, info)
		if !ok {
			dirTracker.addFile(path)
			return nil
		}
		if cleanoutDays > 0 && stamp.Before(cutoff) {
			// The file is too old; remove it.
			err = versionsFs.Remove(path)
		} else {
			// Remember candidates for size cleanup, or mark the directory
			// as nonempty when there is no size limit.
			if maxSize > 0 {
				files = append(files, candidate{path, info.Size(), stamp})
				totalSize += info.Size()
			} else {
				dirTracker.addFile(path)
			}
		}
		return err
	}

	if err := versionsFs.Walk(".", walkFn); err != nil {
		return err
	}

	if maxSize > 0 && totalSize > maxSize {
		sort.Slice(files, func(i, j int) bool {
			if files[i].versionTime.Equal(files[j].versionTime) {
				return files[i].path < files[j].path
			}
			return files[i].versionTime.Before(files[j].versionTime)
		})
	}
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		if maxSize > 0 && totalSize > maxSize {
			if err := versionsFs.Remove(file.path); err != nil {
				return err
			}
			totalSize -= file.size
		} else {
			dirTracker.addFile(file.path)
		}
	}

	dirTracker.deleteEmptyDirs(versionsFs)

	return nil
}
