// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// +build !solaris,!darwin solaris,cgo darwin,cgo

package fswatcher

import (
	"errors"
	"syscall"
	"testing"
)

func TestErrorInotifyInterpretation(t *testing.T) {
	// Exchange link for own documentation when available
	msg := "failed to install inotify handler for folder test-folder. Please increase inotify limits, see https://github.com/syncthing/syncthing-inotify#troubleshooting-for-folders-with-many-files-on-linux for more information"
	var errTooManyFiles syscall.Errno = 24
	var errNoSpace syscall.Errno = 28

	if !reachedMaxUserWatches(errTooManyFiles) {
		t.Errorf("Errno %v should be recognised to be about inotify limits.", errTooManyFiles)
	}
	if !reachedMaxUserWatches(errNoSpace) {
		t.Errorf("Errno %v should be recognised to be about inotify limits.", errNoSpace)
	}
	err := errors.New("Another error")
	if reachedMaxUserWatches(err) {
		t.Errorf("This error does not concern inotify limits: %#v", err)
	}

	err = reachedMaxUserWatchesError("test-folder")
	if err.Error() != msg {
		t.Errorf("Expected error about inotify limits, but got: %#v", err)
	}
}
