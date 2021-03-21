// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package cli

import (
	"github.com/urfave/cli"
)

var showCommand = cli.Command{
	Name:     "show",
	HideHelp: true,
	Usage:    "Show command group",
	Subcommands: []cli.Command{
		{
			Name:   "version",
			Usage:  "Show syncthing client version",
			Action: expects(0, dumpOutput("system/version")),
		},
		{
			Name:   "config-status",
			Usage:  "Show configuration status, whether or not a restart is required for changes to take effect",
			Action: expects(0, dumpOutput("config/restart-required")),
		},
		{
			Name:   "system",
			Usage:  "Show system status",
			Action: expects(0, dumpOutput("system/status")),
		},
		{
			Name:   "connections",
			Usage:  "Report about connections to other devices",
			Action: expects(0, dumpOutput("system/connections")),
		},
		{
			Name:   "usage",
			Usage:  "Show usage report",
			Action: expects(0, dumpOutput("svc/report")),
		},
	},
}
