// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package internal

import (
	"os"
	"strings"

	"github.com/syncthing/syncthing/lib/logger"
)

var (
	l = logger.DefaultLogger.NewFacility("tray", "Tray icon")
)

func init() {
	l.SetDebug("tray", strings.Contains(os.Getenv("STTRACE"), "tray") || os.Getenv("STTRACE") == "all")
}
