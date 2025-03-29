// Copyright (C) 2021 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package cmdutil

// DirOptions are reused among several subcommands
type DirOptions struct {
	ConfDir string `name:"config" short:"C" placeholder:"PATH" env:"STCONFDIR" help:"Set configuration directory (config and keys)"`
	HomeDir string `name:"home" short:"H" placeholder:"PATH" env:"STHOMEDIR" help:"Set configuration and data directory"`
	DataDir string `name:"data" short:"D" placeholder:"PATH" env:"STDATADIR" help:"Set data directory (database and logs)"`
}
