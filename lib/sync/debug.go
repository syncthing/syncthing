// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package sync

import (
	"context"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/syncthing/syncthing/internal/slogutil"
)

var (
	threshold = 100 * time.Millisecond
	l         = slogutil.NewAdapter("Mutexes")

	// We make an exception in this package and have an actual "if debug { ...
	// }" variable, as it may be rather performance critical and does
	// nonstandard things (from a debug logging PoV).
	debug = l.Enabled(context.Background(), slog.LevelDebug)
)

func init() {
	if n, _ := strconv.Atoi(os.Getenv("STLOCKTHRESHOLD")); n > 0 {
		threshold = time.Duration(n) * time.Millisecond
	}
	l.Debugf("Enabling lock logging at %v threshold", threshold)
}
