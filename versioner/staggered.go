// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package versioner

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/syncthing/syncthing/osutil"
)

func init() {
	// Register the constructor for this type of versioner with the name "staggered"
	Factories["staggered"] = NewStaggered
}

type Interval struct {
	step int64
	end  int64
}

// The type holds our configuration
type Staggered struct {
	versionsPath  string
	cleanInterval int64
	repoPath      string
	interval      [4]Interval
	mutex         *sync.Mutex
}

// Check if file or dir
func isFile(path string) bool {
	fileInfo, err := os.Stat(path)
	if err != nil {
		l.Infoln("versioner isFile:", err)
		return false
	}
	return fileInfo.Mode().IsRegular()
}

// The constructor function takes a map of parameters and creates the type.
func NewStaggered(repoID, repoPath string, params map[string]string) Versioner {
	maxAge, err := strconv.ParseInt(params["maxAge"], 10, 0)
	if err != nil {
		maxAge = 31536000 // Default: ~1 year
	}
	cleanInterval, err := strconv.ParseInt(params["cleanInterval"], 10, 0)
	if err != nil {
		cleanInterval = 3600 // Default: clean once per hour
	}

	// Use custom path if set, otherwise .stversions in repoPath
	var versionsDir string
	if params["versionsPath"] == "" {
		if debug {
			l.Debugln("using default dir .stversions")
		}
		versionsDir = filepath.Join(repoPath, ".stversions")
	} else {
		if debug {
			l.Debugln("using dir", params["versionsPath"])
		}
		versionsDir = params["versionsPath"]
	}

	var mutex sync.Mutex
	s := Staggered{
		versionsPath:  versionsDir,
		cleanInterval: cleanInterval,
		repoPath:      repoPath,
		interval: [4]Interval{
			Interval{30, 3600},               // first hour -> 30 sec between versions
			Interval{3600, 86400},            // next day -> 1 h between versions
			Interval{86400, 592000},          // next 30 days -> 1 day between versions
			Interval{604800, maxAge * 86400}, // next year -> 1 week between versions
		},
		mutex: &mutex,
	}

	if debug {
		l.Debugf("instantiated %#v", s)
	}

	go func() {
		s.clean()
		for _ = range time.Tick(time.Duration(cleanInterval) * time.Second) {
			s.clean()
		}
	}()

	return s
}

func (v Staggered) clean() {
	if debug {
		l.Debugln("Versioner clean: Waiting for lock on", v.versionsPath)
	}
	v.mutex.Lock()
	defer v.mutex.Unlock()
	if debug {
		l.Debugln("Versioner clean: Cleaning", v.versionsPath)
	}

	_, err := os.Stat(v.versionsPath)
	if err != nil {
		if os.IsNotExist(err) {
			if debug {
				l.Debugln("creating versions dir", v.versionsPath)
			}
			os.MkdirAll(v.versionsPath, 0755)
			osutil.HideFile(v.versionsPath)
		} else {
			l.Warnln("Versioner: can't create versions dir", err)
		}
	}

	versionsPerFile := make(map[string][]string)
	filesPerDir := make(map[string]int)

	err = filepath.Walk(v.versionsPath, func(path string, f os.FileInfo, err error) error {
		switch mode := f.Mode(); {
		case mode.IsDir():
			filesPerDir[path] = 0

		case mode.IsRegular():
			extension := filepath.Ext(path)
			dir := filepath.Dir(path)
			name := path[:len(path)-len(extension)]

			filesPerDir[dir]++
			versionsPerFile[name] = append(versionsPerFile[name], path)
		}

		return nil
	})
	if err != nil {
		l.Warnln("Versioner: error scanning versions dir", err)
		return
	}

	for _, versionList := range versionsPerFile {
		// List from filepath.Walk is sorted
		v.expire(versionList)
	}

	for path, numFiles := range filesPerDir {
		if path == v.versionsPath {
			if debug {
				l.Debugln("Cleaner: versions dir is empty, don't delete", path)
			}
			continue
		}
		if numFiles > 0 {
			continue
		}

		if debug {
			l.Debugln("Cleaner: deleting empty directory", path)
		}
		err = os.Remove(path)
		if err != nil {
			l.Warnln("Versioner: can't remove directory", path, err)
		}
	}
	if debug {
		l.Debugln("Cleaner: Finished cleaning", v.versionsPath)
	}
}

func (v Staggered) expire(versions []string) {
	if debug {
		l.Debugln("Versioner: Expiring versions", versions)
	}
	now := time.Now().Unix()
	var prevAge int64
	firstFile := true
	for _, file := range versions {
		if isFile(file) {
			versiondate, err := strconv.ParseInt(strings.Replace(filepath.Ext(file), ".v", "", 1), 10, 0)
			if err != nil {
				l.Infof("Versioner: file name %q is invalid: %v", file, err)
				continue
			}
			age := now - versiondate

			// If the file is older than the max age of the last interval, remove it
			if lastIntv := v.interval[len(v.interval)-1]; lastIntv.end > 0 && age > lastIntv.end {
				if debug {
					l.Debugln("Versioner: File over maximum age -> delete ", file)
				}
				err = os.Remove(file)
				if err != nil {
					l.Warnf("Versioner: can't remove %q: %v", file, err)
				}
				continue
			}

			// If it's the first (oldest) file in the list we can skip the interval checks
			if firstFile {
				prevAge = age
				firstFile = false
				continue
			}

			// Find the interval the file fits in
			var usedInterval Interval
			for _, usedInterval = range v.interval {
				if age < usedInterval.end {
					break
				}
			}

			if prevAge-age < usedInterval.step {
				if debug {
					l.Debugln("too many files in step -> delete", file)
				}
				err = os.Remove(file)
				if err != nil {
					l.Warnf("Versioner: can't remove %q: %v", file, err)
				}
				continue
			}

			prevAge = age
		} else {
			l.Infof("non-file %q is named like a file version", file)
		}
	}
}

// Move away the named file to a version archive. If this function returns
// nil, the named file does not exist any more (has been archived).
func (v Staggered) Archive(filePath string) error {
	if debug {
		l.Debugln("Waiting for lock on ", v.versionsPath)
	}
	v.mutex.Lock()
	defer v.mutex.Unlock()

	_, err := os.Stat(filePath)
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

	_, err = os.Stat(v.versionsPath)
	if err != nil {
		if os.IsNotExist(err) {
			if debug {
				l.Debugln("creating versions dir", v.versionsPath)
			}
			os.MkdirAll(v.versionsPath, 0755)
			osutil.HideFile(v.versionsPath)
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

	dir := filepath.Join(v.versionsPath, inRepoPath)
	err = os.MkdirAll(dir, 0755)
	if err != nil && !os.IsExist(err) {
		return err
	}

	ver := file + ".v" + fmt.Sprintf("%010d", time.Now().Unix())
	dst := filepath.Join(dir, ver)
	if debug {
		l.Debugln("moving to", dst)
	}
	err = osutil.Rename(filePath, dst)
	if err != nil {
		return err
	}

	versions, err := filepath.Glob(filepath.Join(dir, file+".v[0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9]"))
	if err != nil {
		l.Warnln("Versioner: error finding versions for", file, err)
		return nil
	}

	sort.Strings(versions)
	v.expire(versions)

	return nil
}
