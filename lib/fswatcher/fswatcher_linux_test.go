// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package fswatcher

import (
	"errors"
	"syscall"
	"testing"
)

func TestErrorInotifyInterpretation(t *testing.T) {
	msg := "Failed to install inotify handler for test-folder." +
		" Please increase inotify limits," +
		" see http://bit.ly/1PxkdUC for more information."
	var errTooManyFiles syscall.Errno = 24
	var errNoSpace syscall.Errno = 28

	if !isWatchesTooFew(errTooManyFiles) {
		t.Errorf("Errno 24 shoulb be recognised to be about inotify limits.")
	}
	if !isWatchesTooFew(errNoSpace) {
		t.Errorf("Errno 28 shoulb be recognised to be about inotify limits.")
	}
	err := errors.New("Another error")
	if isWatchesTooFew(err) {
		t.Errorf("This error does not concern inotify limits: %#v", err)
	}

	err = WatchesLimitTooLowError("test-folder")
	if err.Error() != msg {
		t.Errorf("Expected error about inotify limits, but got: %#v", err)
	}
}
