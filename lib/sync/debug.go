// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package sync

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/syncthing/syncthing/lib/logger"
)

var (
	threshold = time.Duration(100 * time.Millisecond)
	l         = logger.DefaultLogger.NewFacility("sync", "Mutexes")

	// We make an exception in this package and have an actual "if debug { ...
	// }" variable, as it may be rather performance critical and does
	// nonstandard things (from a debug logging PoV).
	debug = strings.Contains(os.Getenv("STTRACE"), "sync") || os.Getenv("STTRACE") == "all"
)

func init() {
	l.SetDebug("sync", strings.Contains(os.Getenv("STTRACE"), "sync") || os.Getenv("STTRACE") == "all")

	if n, err := strconv.Atoi(os.Getenv("STLOCKTHRESHOLD")); err == nil {
		threshold = time.Duration(n) * time.Millisecond
	}
	l.Debugf("Enabling lock logging at %v threshold", threshold)
}
