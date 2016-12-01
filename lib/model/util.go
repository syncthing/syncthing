// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package model

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/syncthing/syncthing/lib/ignore"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/versioner"
)

type Holdable interface {
	Holders() string
}

func newDeadlockDetector(timeout time.Duration) *deadlockDetector {
	return &deadlockDetector{
		timeout: timeout,
		lockers: make(map[string]sync.Locker),
	}
}

type deadlockDetector struct {
	timeout time.Duration
	lockers map[string]sync.Locker
}

func (d *deadlockDetector) Watch(name string, mut sync.Locker) {
	d.lockers[name] = mut
	go func() {
		for {
			time.Sleep(d.timeout / 4)
			ok := make(chan bool, 2)

			go func() {
				mut.Lock()
				mut.Unlock()
				ok <- true
			}()

			go func() {
				time.Sleep(d.timeout)
				ok <- false
			}()

			if r := <-ok; !r {
				msg := fmt.Sprintf("deadlock detected at %s", name)
				for otherName, otherMut := range d.lockers {
					if otherHolder, ok := otherMut.(Holdable); ok {
						msg += "\n===" + otherName + "===\n" + otherHolder.Holders()
					}
				}
				panic(msg)
			}
		}
	}()
}

// moveforconflict renames a file to deal with sync conflicts
func moveforconflict(name string, maxConflicts int) error {
	if strings.Contains(filepath.Base(name), ".sync-conflict-") {
		l.Infoln("Conflict for", name, "which is already a conflict copy; not copying again.")
		if err := os.Remove(name); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}

	if maxConflicts == 0 {
		if err := os.Remove(name); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}

	ext := filepath.Ext(name)
	withoutExt := name[:len(name)-len(ext)]
	newName := withoutExt + time.Now().Format(".sync-conflict-20060102-150405") + ext
	err := os.Rename(name, newName)
	if os.IsNotExist(err) {
		// We were supposed to move a file away but it does not exist. Either
		// the user has already moved it away, or the conflict was between a
		// remote modification and a local delete. In either way it does not
		// matter, go ahead as if the move succeeded.
		err = nil
	}
	if maxConflicts > -1 {
		matches, gerr := osutil.Glob(withoutExt + ".sync-conflict-????????-??????" + ext)
		if gerr == nil && len(matches) > maxConflicts {
			sort.Sort(sort.Reverse(sort.StringSlice(matches)))
			for _, match := range matches[maxConflicts:] {
				gerr = os.Remove(match)
				if gerr != nil {
					l.Debugln("removing extra conflict", gerr)
				}
			}
		} else if gerr != nil {
			l.Debugln("globbing for conflicts", gerr)
		}
	}
	return err
}

// deletedir attempts to delete the given directory
func deletedir(path string, file protocol.FileInfo, matcher *ignore.Matcher) error {
	realName, err := rootedJoinedPath(path, file.Name)
	if err != nil {
		return err
	}

	// Delete any temporary files lying around in the directory
	dir, _ := os.Open(realName)
	if dir != nil {
		files, _ := dir.Readdirnames(-1)
		for _, dirFile := range files {
			fullDirFile := filepath.Join(file.Name, dirFile)
			if defTempNamer.IsTemporary(dirFile) || (matcher != nil && matcher.Match(fullDirFile).IsDeletable()) {
				os.RemoveAll(filepath.Join(path, fullDirFile))
			}
		}
		dir.Close()
	}

	return osutil.InWritableDir(os.Remove, realName)
}

// deletefile attempts to delete the given file
// Takes sync conflict and versioning settings into account
func deletefile(path string, file protocol.FileInfo, ver versioner.Versioner, maxConflicts int) error {
	var err error

	realName, err := rootedJoinedPath(path, file.Name)
	if err != nil {
		return err
	}

	if maxConflicts > 0 {
		// There is a conflict here. Move the file to a conflict copy instead
		// of deleting.
		err = osutil.InWritableDir(func(path string) error {
			return moveforconflict(realName, maxConflicts)
		}, realName)
	} else if ver != nil {
		err = osutil.InWritableDir(ver.Archive, realName)
	} else {
		err = osutil.InWritableDir(os.Remove, realName)
	}
	return err
}
