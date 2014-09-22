// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package versioner

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/syncthing/syncthing/internal/osutil"
)

func init() {
	// Register the constructor for this type of versioner with the name "simple"
	Factories["simple"] = NewSimple
}

// The type holds our configuration
type Simple struct {
	keep     int
	repoPath string
}

// The constructor function takes a map of parameters and creates the type.
func NewSimple(repoID, repoPath string, params map[string]string) Versioner {
	keep, err := strconv.Atoi(params["keep"])
	if err != nil {
		keep = 5 // A reasonable default
	}

	s := Simple{
		keep:     keep,
		repoPath: repoPath,
	}

	if debug {
		l.Debugf("instantiated %#v", s)
	}
	return s
}

// Move away the named file to a version archive. If this function returns
// nil, the named file does not exist any more (has been archived).
func (v Simple) Archive(filePath string) error {
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			if debug {
				l.Debugln("not archiving nonexistent file", filePath)
			}
			return nil
		} else {
			return err
		}
	}

	versionsDir := filepath.Join(v.repoPath, ".stversions")
	_, err = os.Stat(versionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			if debug {
				l.Debugln("creating versions dir", versionsDir)
			}
			os.MkdirAll(versionsDir, 0755)
			osutil.HideFile(versionsDir)
		} else {
			return err
		}
	}

	if debug {
		l.Debugln("archiving", filePath)
	}

	file := filepath.Base(filePath)
	inRepoPath, err := filepath.Rel(v.repoPath, filepath.Dir(filePath))
	if err != nil {
		return err
	}

	dir := filepath.Join(versionsDir, inRepoPath)
	err = os.MkdirAll(dir, 0755)
	if err != nil && !os.IsExist(err) {
		return err
	}

	ver := file + "~" + fileInfo.ModTime().Format("20060102-150405")
	dst := filepath.Join(dir, ver)
	if debug {
		l.Debugln("moving to", dst)
	}
	err = osutil.Rename(filePath, dst)
	if err != nil {
		return err
	}

	versions, err := filepath.Glob(filepath.Join(dir, file+"~[0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9]-[0-9][0-9][0-9][0-9][0-9][0-9]"))
	if err != nil {
		l.Warnln("globbing:", err)
		return nil
	}

	if len(versions) > v.keep {
		sort.Strings(versions)
		for _, toRemove := range versions[:len(versions)-v.keep] {
			if debug {
				l.Debugln("cleaning out", toRemove)
			}
			err = os.Remove(toRemove)
			if err != nil {
				l.Warnln("removing old version:", err)
			}
		}
	}

	return nil
}
