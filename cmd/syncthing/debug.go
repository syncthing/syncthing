// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"os"
	"strings"

	"github.com/syncthing/syncthing/lib/logger"
)

var (
	l     = logger.DefaultLogger.NewFacility("main", "Main package")
	httpl = logger.DefaultLogger.NewFacility("http", "REST API")
)

func shouldDebugHTTP() bool {
	return l.ShouldDebug("http")
}

func init() {
	l.SetDebug("main", strings.Contains(os.Getenv("STTRACE"), "main") || os.Getenv("STTRACE") == "all")
	l.SetDebug("http", strings.Contains(os.Getenv("STTRACE"), "http") || os.Getenv("STTRACE") == "all")
}
