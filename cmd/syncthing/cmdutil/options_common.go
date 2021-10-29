// Copyright (C) 2021 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package cmdutil

// CommonOptions are reused among several subcommands
type CommonOptions struct {
	buildCommonOptions
	ConfDir         string `name:"config" placeholder:"PATH" help:"Set configuration directory (config and keys)"`
	HomeDir         string `name:"home" placeholder:"PATH" help:"Set configuration and data directory"`
	NoDefaultFolder bool   `env:"STNODEFAULTFOLDER" help:"Don't create the \"default\" folder on first startup"`
}
