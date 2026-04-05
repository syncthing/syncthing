// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package sqlite

import "github.com/syncthing/syncthing/internal/slogutil"

func init() { slogutil.RegisterPackage("SQLite database") }
