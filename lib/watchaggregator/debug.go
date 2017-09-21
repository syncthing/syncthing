// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package watchaggregator

import (
	"os"
	"strings"

	"github.com/syncthing/syncthing/lib/logger"
)

var facilityName = "watchaggregator"

var (
	l = logger.DefaultLogger.NewFacility(facilityName, "Filesystem event watcher")
)

func init() {
	l.SetDebug(facilityName, strings.Contains(os.Getenv("STTRACE"), facilityName) || os.Getenv("STTRACE") == "all")
}
