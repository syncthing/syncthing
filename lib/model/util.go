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
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/ur"
)

type Holdable interface {
	Holders() string
}

func newDeadlockDetector(timeout time.Duration, evLogger events.Logger, fatal func(error)) *deadlockDetector {
	return &deadlockDetector{
		warnTimeout:  timeout,
		fatalTimeout: 10 * timeout,
		lockers:      make(map[string]sync.Locker),
		evLogger:     evLogger,
		fatal:        fatal,
	}
}

type deadlockDetector struct {
	warnTimeout, fatalTimeout time.Duration
	lockers                   map[string]sync.Locker
	evLogger                  events.Logger
	fatal                     func(error)
}

func (d *deadlockDetector) Watch(name string, mut sync.Locker) {
	d.lockers[name] = mut
	go func() {
		for {
			time.Sleep(d.warnTimeout / 4)
			done := make(chan struct{}, 1)

			go func() {
				mut.Lock()
				_ = 1 // empty critical section
				mut.Unlock()
				done <- struct{}{}
			}()

			warn := time.NewTimer(d.warnTimeout)
			fatal := time.NewTimer(d.fatalTimeout)

			select {
			case <-warn.C:
				failure := ur.FailureDataWithGoroutines(fmt.Sprintf("potential deadlock detected at %s (short timeout)", name))
				failure.Extra["timeout"] = d.warnTimeout.String()
				d.evLogger.Log(events.Failure, failure)
			case <-done:
				warn.Stop()
				fatal.Stop()
				continue
			}

			select {
			case <-fatal.C:
				err := fmt.Errorf("potential deadlock detected at %s (long timeout)", name)
				failure := ur.FailureDataWithGoroutines(err.Error())
				failure.Extra["timeout"] = d.fatalTimeout.String()
				var others string
				for otherName, otherHolder := range d.otherHolders() {
					others += "===" + otherName + "===\n" + otherHolder + "\n"
				}
				others = others[:len(others)-1]
				failure.Extra["other-holders"] = others
				d.otherHolders()
				d.evLogger.Log(events.Failure, failure)
				d.fatal(err)
				// Give it a minute to shut down gracefully, maybe shutting down
				// can get out of the deadlock (or it's not really a deadlock).
				time.Sleep(time.Minute)
				panic(fmt.Sprintf("%v:\n%v", err, others))
			case <-done:
				fatal.Stop()
			}
		}
	}()
}

func (d *deadlockDetector) otherHolders() map[string]string {
	m := make(map[string]string, len(d.lockers))
	for otherName, otherMut := range d.lockers {
		if otherHolder, ok := otherMut.(Holdable); ok {
			m[otherName] = otherHolder.Holders()
		}
	}
	return m
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
