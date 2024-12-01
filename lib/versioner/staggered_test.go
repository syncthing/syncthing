// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package versioner

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/d4l3k/messagediff"

	"github.com/syncthing/syncthing/lib/config"
)

func TestStaggeredVersioningVersionCount(t *testing.T) {
	/* Default settings:

	{30, 3600},       // first hour -> 30 sec between versions
	{3600, 86400},    // next day -> 1 h between versions
	{86400, 592000},  // next 30 days -> 1 day between versions
	{604800, maxAge}, // next year -> 1 week between versions
	*/

	now := parseTime("20160415-140000")
	versionsWithMtime := []string{
		// 14:00:00 is "now"
		"test~20160415-140000", // 0 seconds ago

		"test~20160415-135959", // 1 second ago
		"test~20160415-135931", // 29 seconds ago
		"test~20160415-135930", // 30 seconds ago

		"test~20160415-130059", // 59 minutes 01 seconds ago
		"test~20160415-130030", // 59 minutes 30 seconds ago

		"test~20160415-130000", // 1 hour ago
		"test~20160415-120001", // 1 hour 59:59 ago

		"test~20160414-155959", // 22 hours 1 second ago
		"test~20160414-150001", // 22 hours 59 seconds ago
		"test~20160414-150000", // 23 hours ago

		"test~20160414-140000", // 1 day ago
		"test~20160414-130001", // 1 days 59:59 second ago

		"test~20160409-135959", // 6 days 1 second ago
		"test~20160408-140001", // 6 days 23:59:59 second ago
		"test~20160408-140000", // 7 days ago

		"test~20160408-135959", // 7 days 1 second ago
		"test~20160407-140001", // 7 days 23:59:59 ago
		"test~20160407-140000", // 8 days ago

		"test~20160317-140000", // 29 days ago
		"test~20160317-135959", // 29 days 1 second ago
		"test~20160316-140000", // 30 days ago

		"test~20160308-135959", // 37 days 1 second ago
		"test~20160301-140000", // 44 days ago

		"test~20160223-140000", // 51 days ago

		"test~20150423-140000", // 358 days ago (!!! 2016 was a leap year !!!)

		"test~20150417-140000", // 364 days ago
		"test~20150416-140000", // 365 days ago

		// exceeds maxAge
		"test~20150416-135959", // 365 days 1 second ago
		"test~20150416-135958", // 365 days 2 seconds ago
		"test~20150414-140000", // 367 days ago
	}

	delete := []string{
		"test~20160415-135959", // 1 second ago
		"test~20160415-135931", // 29 seconds ago
		"test~20160415-130059", // 59 minutes 01 seconds ago
		"test~20160415-130000", // 1 hour ago
		"test~20160414-155959", // 22 hours 1 second ago
		"test~20160414-150001", // 22 hours 59 seconds ago
		"test~20160414-140000", // 1 day ago
		"test~20160409-135959", // 6 days 1 second ago
		"test~20160408-140001", // 6 days 23:59:59 second ago
		"test~20160408-135959", // 7 days 1 second ago
		"test~20160407-140001", // 7 days 23:59:59 ago
		"test~20160317-135959", // 29 days 1 second ago
		"test~20160308-135959", // 37 days 1 second ago
		"test~20150417-140000", // 364 days ago
		"test~20150416-135959", // 365 days 1 second ago
		"test~20150416-135958", // 365 days 2 seconds ago
		"test~20150414-140000", // 367 days ago
	}
	sort.Strings(delete)

	cfg := config.FolderConfiguration{
		FilesystemType: config.FilesystemTypeBasic,
		Path:           "testdata",
		Versioning: config.VersioningConfiguration{
			Params: map[string]string{
				"maxAge": strconv.Itoa(365 * 86400),
			},
		},
	}

	v := newStaggered(cfg).(*staggered)
	rem := v.toRemove(versionsWithMtime, now)
	sort.Strings(rem)

	if diff, equal := messagediff.PrettyDiff(delete, rem); !equal {
		t.Errorf("Incorrect deleted files; got %v, expected %v\n%v", rem, delete, diff)
	}
}

func parseTime(in string) time.Time {
	t, err := time.ParseInLocation(TimeFormat, in, time.Local)
	if err != nil {
		panic(err.Error())
	}
	return t
}

func TestCreateVersionPath(t *testing.T) {
	const (
		versionsDir = "some/nested/dir"
		archiveFile = "testfile"
	)

	// Create a test dir and file
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, archiveFile), []byte("sup"), 0o644); err != nil {
		t.Fatal(err)
	}

	folderCfg := config.FolderConfiguration{
		ID:   "default",
		Path: tmpDir,
		Versioning: config.VersioningConfiguration{
			Type:   "staggered",
			FSPath: versionsDir,
		},
	}

	// Archive the file
	versioner := newStaggered(folderCfg)
	if err := versioner.Archive(archiveFile); err != nil {
		t.Fatal(err)
	}

	// Look for files named like the test file, in the archive dir.
	files, err := filepath.Glob(filepath.Join(tmpDir, versionsDir, archiveFile) + "*")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Error("expected file to have been archived")
	}
}
