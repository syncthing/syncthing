// Copyright (C) 2021 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package cli

import (
	"github.com/urfave/cli"
)

var pendingCommand = cli.Command{
	Name:     "pending",
	HideHelp: true,
	Usage:    "Pending subcommand group",
	Subcommands: []cli.Command{
		{
			Name:   "devices",
			Usage:  "Show pending devices",
			Action: expects(0, indexDumpOutput("cluster/pending/devices")),
		},
		{
			Name:   "folders",
			Usage:  "Show pending folders",
			Action: expects(0, indexDumpOutput("cluster/pending/folders")),
		},
	},
}
