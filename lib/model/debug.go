// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"os"
	"strings"

	"github.com/syncthing/syncthing/lib/logger"
)

var (
	l = logger.DefaultLogger.NewFacility("model", "The root hub")
)

func init() {
	l.SetDebug("model", strings.Contains(os.Getenv("STTRACE"), "model") || os.Getenv("STTRACE") == "all")
}

func shouldDebug() bool {
	return l.ShouldDebug("model")
}
