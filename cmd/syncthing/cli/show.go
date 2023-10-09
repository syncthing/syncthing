// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package cli

import (
	"github.com/alecthomas/kong"
)

type showCommand struct {
	Version      struct{}       `cmd:"" help:"Show syncthing client version"`
	ConfigStatus struct{}       `cmd:"" help:"Show configuration status, whether or not a restart is required for changes to take effect"`
	System       struct{}       `cmd:"" help:"Show system status"`
	Connections  struct{}       `cmd:"" help:"Report about connections to other devices"`
	Discovery    struct{}       `cmd:"" help:"Show the discovered addresses of remote devices (from cache of the running syncthing instance)"`
	Usage        struct{}       `cmd:"" help:"Show usage report"`
	Pending      pendingCommand `cmd:"" help:"Pending subcommand group"`
}

func (s *showCommand) Run(ctx Context, kongCtx *kong.Context) error {
	indexDumpOutput := indexDumpOutputWrapper(ctx.clientFactory)

	switch kongCtx.Selected().Name {
	case "version":
		return indexDumpOutput("system/version")
	case "config-status":
		return indexDumpOutput("config/restart-required")
	case "system":
		return indexDumpOutput("system/status")
	case "connections":
		return indexDumpOutput("system/connections")
	case "discovery":
		return indexDumpOutput("system/discovery")
	case "usage":
		return indexDumpOutput("svc/report")
	}

	return nil
}
