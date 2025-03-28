// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package watchaggregator

import (
	"github.com/syncthing/syncthing/lib/logger"
)

var l = logger.DefaultLogger.NewFacility("watchaggregator", "Filesystem event watcher")
