// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build !cgo && wazero

package sqlite

import (
	_ "github.com/ncruces/go-sqlite3/driver" // register sqlite database driver
	_ "github.com/ncruces/go-sqlite3/embed"  // register sqlite database driver
	"github.com/syncthing/syncthing/lib/build"
)

const (
	dbDriver      = "sqlite3"
	commonOptions = "_pragma=foreign_keys(1)&_pragma=recursive_triggers(1)&_pragma=cache_size(-65536)&_pragma=synchronous(1)"
)

func init() {
	build.AddTag("ncruces-sqlite")
}
