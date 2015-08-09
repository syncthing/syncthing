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

	"github.com/calmh/logger"
)

var (
	debug     = strings.Contains(os.Getenv("STTRACE"), "locks") || os.Getenv("STTRACE") == "all"
	threshold = time.Duration(100 * time.Millisecond)
	l         = logger.DefaultLogger
)

func init() {
	if n, err := strconv.Atoi(os.Getenv("STLOCKTHRESHOLD")); debug && err == nil {
		threshold = time.Duration(n) * time.Millisecond
	}
	if debug {
		l.Debugf("Enabling lock logging at %v threshold", threshold)
	}
}
