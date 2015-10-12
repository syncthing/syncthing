// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package db

import (
	"os"
	"strings"

	"github.com/calmh/logger"
)

var (
	debug   = strings.Contains(os.Getenv("STTRACE"), "files") || os.Getenv("STTRACE") == "all"
	debugDB = strings.Contains(os.Getenv("STTRACE"), "db") || os.Getenv("STTRACE") == "all"
	l       = logger.DefaultLogger
)
