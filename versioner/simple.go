// Copyright (C) 2014 Jakob Borg and other contributors. All rights reserved.
// Use of this source code is governed by an MIT-style license that can be
// found in the LICENSE file.

package versioner

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/calmh/syncthing/osutil"
)

func init() {
	// Register the constructor for this type of versioner with the name "simple"
	Factories["simple"] = NewSimple
}

// The type holds our configuration
type Simple struct {
	keep int
}

// The constructor function takes a map of parameters and creates the type.
func NewSimple(params map[string]string) Versioner {
	keep, err := strconv.Atoi(params["keep"])
	if err != nil {
		keep = 5 // A reasonable default
	}

	s := Simple{
		keep: keep,
	}

	if debug {
		l.Debugf("instantiated %#v", s)
	}
	return s
}

// Move away the named file to a version archive. If this function returns
// nil, the named file does not exist any more (has been archived).
func (v Simple) Archive(path string) error {
	_, err := os.Stat(path)
	if err != nil && os.IsNotExist(err) {
		return nil
	}

	if debug {
		l.Debugln("archiving", path)
	}

	file := filepath.Base(path)
	dir := filepath.Join(filepath.Dir(path), ".stversions")
	err = os.MkdirAll(dir, 0755)
	if err != nil && !os.IsExist(err) {
		return err
	} else {
		osutil.HideFile(dir)
	}

	ver := file + "~" + time.Now().Format("20060102-150405")
	err = osutil.Rename(path, filepath.Join(dir, ver))
	if err != nil {
		return err
	}

	versions, err := filepath.Glob(filepath.Join(dir, file+"~*"))
	if err != nil {
		l.Warnln(err)
		return nil
	}

	if len(versions) > v.keep {
		sort.Strings(versions)
		for _, toRemove := range versions[:len(versions)-v.keep] {
			err = os.Remove(toRemove)
			if err != nil {
				l.Warnln(err)
			}
		}
	}

	return nil
}
