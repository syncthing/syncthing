// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package beacon

import (
	"os"
	"strings"

	"github.com/syncthing/syncthing/lib/logger"
)

var (
	l = logger.DefaultLogger.NewFacility("beacon", "Multicast and broadcast discovery")
)

func init() {
	l.SetDebug("beacon", strings.Contains(os.Getenv("STTRACE"), "beacon") || os.Getenv("STTRACE") == "all")
}
