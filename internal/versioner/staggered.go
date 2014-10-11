// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

package versioner

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/syncthing/syncthing/internal/osutil"
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
	folderPath    string
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

const TimeLayout = "20060102-150405"

func versionExt(path string) string {
	pathSplit := strings.Split(path, "~")
	if len(pathSplit) > 1 {
		return pathSplit[len(pathSplit)-1]
	} else {
		return ""
	}
}

// Rename versions with old version format
func (v Staggered) renameOld() {
	err := filepath.Walk(v.versionsPath, func(path string, f os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if f.Mode().IsRegular() {
			versionUnix, err := strconv.ParseInt(strings.Replace(filepath.Ext(path), ".v", "", 1), 10, 0)
			if err == nil {
				l.Infoln("Renaming file", path, "from old to new version format")
				versiondate := time.Unix(versionUnix, 0)
				name := path[:len(path)-len(filepath.Ext(path))]
				err = osutil.Rename(path, name+"~"+versiondate.Format(TimeLayout))
				if err != nil {
					l.Infoln("Error renaming to new format", err)
				}
			}
		}
		return nil
	})
	if err != nil {
		l.Infoln("Versioner: error scanning versions dir", err)
		return
	}
}

// The constructor function takes a map of parameters and creates the type.
func NewStaggered(folderID, folderPath string, params map[string]string) Versioner {
	maxAge, err := strconv.ParseInt(params["maxAge"], 10, 0)
	if err != nil {
		maxAge = 31536000 // Default: ~1 year
	}
	cleanInterval, err := strconv.ParseInt(params["cleanInterval"], 10, 0)
	if err != nil {
		cleanInterval = 3600 // Default: clean once per hour
	}

	// Use custom path if set, otherwise .stversions in folderPath
	var versionsDir string
	if params["versionsPath"] == "" {
		if debug {
			l.Debugln("using default dir .stversions")
		}
		versionsDir = filepath.Join(folderPath, ".stversions")
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
		folderPath:    folderPath,
		interval: [4]Interval{
			{30, 3600},               // first hour -> 30 sec between versions
			{3600, 86400},            // next day -> 1 h between versions
			{86400, 592000},          // next 30 days -> 1 day between versions
			{604800, maxAge},         // next year -> 1 week between versions
		},
		mutex: &mutex,
	}

	if debug {
		l.Debugf("instantiated %#v", s)
	}

	// Rename version with old version format
	s.renameOld()

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
		if err != nil {
			return err
		}
		switch mode := f.Mode(); {
		case mode.IsDir():
			filesPerDir[path] = 0
			if path != v.versionsPath {
				dir := filepath.Dir(path)
				filesPerDir[dir]++
			}
		case mode.IsRegular():
			extension := versionExt(path)
			dir := filepath.Dir(path)
			name := path[:len(path)-len(extension)-1]

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
		if numFiles > 0 {
			continue
		}

		if path == v.versionsPath {
			if debug {
				l.Debugln("Cleaner: versions dir is empty, don't delete", path)
			}
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
	var prevAge int64
	firstFile := true
	for _, file := range versions {
		if isFile(file) {
			versionTime, err := time.Parse(TimeLayout, versionExt(file))
			if err != nil {
				l.Infof("Versioner: file name %q is invalid: %v", file, err)
				continue
			}
			age := int64(time.Since(versionTime).Seconds())

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
	inFolderPath, err := filepath.Rel(v.folderPath, filepath.Dir(filePath))
	if err != nil {
		return err
	}

	dir := filepath.Join(v.versionsPath, inFolderPath)
	err = os.MkdirAll(dir, 0755)
	if err != nil && !os.IsExist(err) {
		return err
	}

	ver := file + "~" + fileInfo.ModTime().Format(TimeLayout)
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
		l.Warnln("Versioner: error finding versions for", file, err)
		return nil
	}

	sort.Strings(versions)
	v.expire(versions)

	return nil
}
