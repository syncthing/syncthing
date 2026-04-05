// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package events

import (
	"github.com/syncthing/syncthing/internal/slogutil"
)

var dl = slogutil.NewAdapter("Event generation and logging")
