// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package sync

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/sasha-s/go-deadlock"
	"github.com/syncthing/syncthing/lib/logger"
)

var (
	threshold = 100 * time.Millisecond
	l         = logger.DefaultLogger.NewFacility("sync", "Mutexes")

	// We make an exception in this package and have an actual "if debug { ...
	// }" variable, as it may be rather performance critical and does
	// nonstandard things (from a debug logging PoV).
	debug       = strings.Contains(os.Getenv("STTRACE"), "sync") || os.Getenv("STTRACE") == "all"
	useDeadlock = os.Getenv("STDEADLOCK") != ""
)

func init() {
	l.SetDebug("sync", strings.Contains(os.Getenv("STTRACE"), "sync") || os.Getenv("STTRACE") == "all")

	if n, err := strconv.Atoi(os.Getenv("STLOCKTHRESHOLD")); err == nil {
		threshold = time.Duration(n) * time.Millisecond
	}
	if n, err := strconv.Atoi(os.Getenv("STDEADLOCK")); err == nil {
		deadlock.Opts.DeadlockTimeout = time.Duration(n) * time.Second
	}
	l.Debugf("Enabling lock logging at %v threshold", threshold)
}
