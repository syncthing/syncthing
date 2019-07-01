// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package stun

import (
	"os"
	"strings"

	"github.com/syncthing/syncthing/lib/logger"
)

var (
	l = logger.DefaultLogger.NewFacility("stun", "STUN functionality")
)

func init() {
	l.SetDebug("stun", strings.Contains(os.Getenv("STTRACE"), "stun") || os.Getenv("STTRACE") == "all")
}
