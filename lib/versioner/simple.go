// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package versioner

import (
	"os"
	"path/filepath"
	"strconv"

	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/util"
)

func init() {
	// Register the constructor for this type of versioner with the name "simple"
	Factories["simple"] = NewSimple
}

type Simple struct {
	keep       int
	folderPath string
}

func NewSimple(folderID, folderPath string, params map[string]string) Versioner {
	keep, err := strconv.Atoi(params["keep"])
	if err != nil {
		keep = 5 // A reasonable default
	}

	s := Simple{
		keep:       keep,
		folderPath: folderPath,
	}

	l.Debugf("instantiated %#v", s)
	return s
}

// Archive moves the named file away to a version archive. If this function
// returns nil, the named file does not exist any more (has been archived).
func (v Simple) Archive(filePath string) error {
	fileInfo, err := osutil.Lstat(filePath)
	if os.IsNotExist(err) {
		l.Debugln("not archiving nonexistent file", filePath)
		return nil
	} else if err != nil {
		return err
	}

	versionsDir := filepath.Join(v.folderPath, ".stversions")
	_, err = os.Stat(versionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			l.Debugln("creating versions dir", versionsDir)
			osutil.MkdirAll(versionsDir, 0755)
			osutil.HideFile(versionsDir)
		} else {
			return err
		}
	}

	l.Debugln("archiving", filePath)

	file := filepath.Base(filePath)
	inFolderPath, err := filepath.Rel(v.folderPath, filepath.Dir(filePath))
	if err != nil {
		return err
	}

	dir := filepath.Join(versionsDir, inFolderPath)
	err = osutil.MkdirAll(dir, 0755)
	if err != nil && !os.IsExist(err) {
		return err
	}

	ver := taggedFilename(file, fileInfo.ModTime().Format(TimeFormat))
	dst := filepath.Join(dir, ver)
	l.Debugln("moving to", dst)
	err = osutil.Rename(filePath, dst)
	if err != nil {
		return err
	}

	// Glob according to the new file~timestamp.ext pattern.
	pattern := filepath.Join(dir, taggedFilename(file, TimeGlob))
	newVersions, err := osutil.Glob(pattern)
	if err != nil {
		l.Warnln("globbing:", err, "for", pattern)
		return nil
	}

	// Also according to the old file.ext~timestamp pattern.
	pattern = filepath.Join(dir, file+"~"+TimeGlob)
	oldVersions, err := osutil.Glob(pattern)
	if err != nil {
		l.Warnln("globbing:", err, "for", pattern)
		return nil
	}

	// Use all the found filenames. "~" sorts after "." so all old pattern
	// files will be deleted before any new, which is as it should be.
	versions := util.UniqueStrings(append(oldVersions, newVersions...))

	if len(versions) > v.keep {
		for _, toRemove := range versions[:len(versions)-v.keep] {
			l.Debugln("cleaning out", toRemove)
			err = os.Remove(toRemove)
			if err != nil {
				l.Warnln("removing old version:", err)
			}
		}
	}

	return nil
}
