// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package dialer

import (
	"os"
	"strings"

	"github.com/syncthing/syncthing/lib/logger"
)

var (
	l = logger.DefaultLogger.NewFacility("dialer", "Dialing connections")
	// To run before init() of other files that log on init.
	_ = func() error {
		l.SetDebug("dialer", strings.Contains(os.Getenv("STTRACE"), "dialer") || os.Getenv("STTRACE") == "all")
		return nil
	}()
)
