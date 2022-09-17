// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

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

			d.watchInner(name, done)
		}
	}()
}

func (d *deadlockDetector) watchInner(name string, done chan struct{}) {
	warn := time.NewTimer(d.warnTimeout)
	fatal := time.NewTimer(d.fatalTimeout)
	defer func() {
		warn.Stop()
		fatal.Stop()
	}()

	select {
	case <-warn.C:
		failure := ur.FailureDataWithGoroutines(fmt.Sprintf("potential deadlock detected at %s (short timeout)", name))
		failure.Extra["timeout"] = d.warnTimeout.String()
		d.evLogger.Log(events.Failure, failure)
	case <-done:
		return
	}

	select {
	case <-fatal.C:
		err := fmt.Errorf("potential deadlock detected at %s (long timeout)", name)
		failure := ur.FailureDataWithGoroutines(err.Error())
		failure.Extra["timeout"] = d.fatalTimeout.String()
		others := d.otherHolders()
		failure.Extra["other-holders"] = others
		d.evLogger.Log(events.Failure, failure)
		d.fatal(err)
		// Give it a minute to shut down gracefully, maybe shutting down
		// can get out of the deadlock (or it's not really a deadlock).
		time.Sleep(time.Minute)
		panic(fmt.Sprintf("%v:\n%v", err, others))
	case <-done:
	}
}

func (d *deadlockDetector) otherHolders() string {
	var b strings.Builder
	for otherName, otherMut := range d.lockers {
		if otherHolder, ok := otherMut.(Holdable); ok {
			b.WriteString("===" + otherName + "===\n" + otherHolder.Holders() + "\n")
		}
	}
	return b.String()
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

	const permBits = fs.ModePerm | fs.ModeSetuid | fs.ModeSetgid | fs.ModeSticky
	var parentErr error
	if mode := info.Mode() & permBits; mode&0200 == 0 {
		// A non-writeable directory (for this user; we assume that's the
		// relevant part). Temporarily change the mode so we can delete the
		// file or directory inside it.
		parentErr = targetFs.Chmod(dir, mode|0700)
		if parentErr != nil {
			l.Debugf("Failed to make parent directory writable: %v", parentErr)
		} else {
			// Chmod succeeded, we should change the permissions back on the way
			// out. If we fail we log the error as we have irrevocably messed up
			// at this point. :( (The operation we were called to wrap has
			// succeeded or failed on its own so returning an error to the
			// caller is inappropriate.)
			defer func() {
				if err := targetFs.Chmod(dir, mode); err != nil && !fs.IsNotExist(err) {
					logFn := l.Warnln
					if ignorePerms {
						logFn = l.Debugln
					}
					logFn("Failed to restore directory permissions after gaining write access:", err)
				}
			}()
		}
	}

	err = fn(path)
	if fs.IsPermission(err) && parentErr != nil {
		err = fmt.Errorf("error after failing to make parent directory writable: %w", err)
	}
	return err
}
