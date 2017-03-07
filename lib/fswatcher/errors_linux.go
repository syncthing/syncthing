// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package fswatcher

import (
	"syscall"
)

func interpretNotifyWatchError(err error, folder string) error {
	if errno, converted := err.(syscall.Errno); converted &&
		errno == 24 || errno == 28 {
		return WatchesLimitTooLowError(folder)
	}
	return err
}
