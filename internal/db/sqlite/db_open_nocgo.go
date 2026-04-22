// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build !cgo && !wazero

package sqlite

import (
	"github.com/syncthing/syncthing/lib/build"
	_ "modernc.org/sqlite" // register sqlite database driver
)

const (
	dbDriver      = "sqlite"
	commonOptions = "_pragma=foreign_keys(1)&_pragma=recursive_triggers(1)&_pragma=synchronous(1)&_txlock=immediate"
)

func init() {
	build.AddTag("modernc-sqlite")
}
