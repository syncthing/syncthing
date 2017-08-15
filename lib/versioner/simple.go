// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package versioner

import (
	"path/filepath"
	"strconv"

	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/util"
)

func init() {
	// Register the constructor for this type of versioner with the name "simple"
	Factories["simple"] = NewSimple
}

type Simple struct {
	keep int
	fs   fs.Filesystem
}

func NewSimple(folderID string, fs fs.Filesystem, params map[string]string) Versioner {
	keep, err := strconv.Atoi(params["keep"])
	if err != nil {
		keep = 5 // A reasonable default
	}

	s := Simple{
		keep: keep,
		fs:   fs,
	}

	l.Debugf("instantiated %#v", s)
	return s
}

// Archive moves the named file away to a version archive. If this function
// returns nil, the named file does not exist any more (has been archived).
func (v Simple) Archive(filePath string) error {
	info, err := v.fs.Lstat(filePath)
	if fs.IsNotExist(err) {
		l.Debugln("not archiving nonexistent file", filePath)
		return nil
	} else if err != nil {
		return err
	}
	if info.IsSymlink() {
		panic("bug: attempting to version a symlink")
	}

	versionsDir := ".stversions"
	_, err = v.fs.Stat(versionsDir)
	if err != nil {
		if fs.IsNotExist(err) {
			l.Debugln("creating versions dir .stversions")
			v.fs.Mkdir(versionsDir, 0755)
			v.fs.Hide(versionsDir)
		} else {
			return err
		}
	}

	l.Debugln("archiving", filePath)

	file := filepath.Base(filePath)
	inFolderPath := filepath.Dir(filePath)

	dir := filepath.Join(versionsDir, inFolderPath)
	err = v.fs.MkdirAll(dir, 0755)
	if err != nil && !fs.IsExist(err) {
		return err
	}

	ver := taggedFilename(file, info.ModTime().Format(TimeFormat))
	dst := filepath.Join(dir, ver)
	l.Debugln("moving to", dst)
	err = osutil.Rename(v.fs, filePath, dst)
	if err != nil {
		return err
	}

	// Glob according to the new file~timestamp.ext pattern.
	pattern := filepath.Join(dir, taggedFilename(file, TimeGlob))
	newVersions, err := v.fs.Glob(pattern)
	if err != nil {
		l.Warnln("globbing:", err, "for", pattern)
		return nil
	}

	// Also according to the old file.ext~timestamp pattern.
	pattern = filepath.Join(dir, file+"~"+TimeGlob)
	oldVersions, err := v.fs.Glob(pattern)
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
			err = v.fs.Remove(toRemove)
			if err != nil {
				l.Warnln("removing old version:", err)
			}
		}
	}

	return nil
}
