// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/syncthing/syncthing/lib/fs"
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
				_ = 1 // empty critical section
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

// inWritableDir calls fn(path), while making sure that the directory
// containing `path` is writable for the duration of the call.
func inWritableDir(fn func(string) error, targetFs fs.Filesystem, path string, ignorePerms bool) error {
	dir := filepath.Dir(path)
	info, err := targetFs.Stat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("Not a directory: " + path)
	}
	if info.Mode()&0200 == 0 {
		// A non-writeable directory (for this user; we assume that's the
		// relevant part). Temporarily change the mode so we can delete the
		// file or directory inside it.
		if err := targetFs.Chmod(dir, 0755); err == nil {
			// Chmod succeeded, we should change the permissions back on the way
			// out. If we fail we log the error as we have irrevocably messed up
			// at this point. :( (The operation we were called to wrap has
			// succeeded or failed on its own so returning an error to the
			// caller is inappropriate.)
			defer func() {
				if err := targetFs.Chmod(dir, info.Mode()&fs.ModePerm); err != nil && !fs.IsNotExist(err) {
					logFn := l.Warnln
					if ignorePerms {
						logFn = l.Debugln
					}
					logFn("Failed to restore directory permissions after gaining write access:", err)
				}
			}()
		}
	}

	return fn(path)
}
