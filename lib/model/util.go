// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/timeutil"
)

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
	if mode := info.Mode() & permBits; mode&0o200 == 0 {
		// A non-writeable directory (for this user; we assume that's the
		// relevant part). Temporarily change the mode so we can delete the
		// file or directory inside it.
		parentErr = targetFs.Chmod(dir, mode|0o700)
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

// addTimeUntilCancelled adds time to the counter for the duration of the
// Context. We do this piecemeal so that polling the counter during a long
// operation shows a relevant value, instead of the counter just increasing
// by a large amount at the end of the operation.
func addTimeUntilCancelled(ctx context.Context, counter prometheus.Counter) {
	t0 := time.Now()
	defer func() {
		if dur := time.Since(t0).Seconds(); dur > 0 {
			counter.Add(dur)
		}
	}()

	ticker := time.NewTicker(time.Second)
	defer timeutil.StopTicker(ticker)

	for {
		select {
		case t := <-ticker.C:
			if dur := t.Sub(t0).Seconds(); dur > 0 {
				counter.Add(dur)
			}
			t0 = t
		case <-ctx.Done():
			return
		}
	}
}
