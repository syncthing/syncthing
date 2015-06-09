// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package main

import (
	"os"
	"strings"
)

var (
	debugNet    = strings.Contains(os.Getenv("STTRACE"), "net") || os.Getenv("STTRACE") == "all"
	debugHTTP   = strings.Contains(os.Getenv("STTRACE"), "http") || os.Getenv("STTRACE") == "all"
	debugSuture = strings.Contains(os.Getenv("STTRACE"), "suture") || os.Getenv("STTRACE") == "all"
)
