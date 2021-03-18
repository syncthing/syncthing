// Copyright (C) 2021 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package cli

import (
	"fmt"

	"github.com/urfave/cli"
)

var debugCommand = cli.Command{
	Name:     "debug",
	HideHelp: true,
	Usage:    "Debug command group",
	Subcommands: []cli.Command{
		{
			Name:   "enable",
			Usage:  "Enable debugging",
			Action: expects(0, setDebug(true)),
		},
		{
			Name:   "disable",
			Usage:  "Disable debugging",
			Action: expects(0, setDebug(true)),
		},
		{
			Name:      "file",
			Usage:     "Show information about a file (or directory/symlink)",
			ArgsUsage: "FOLDER-ID PATH",
			Action:    expects(2, debugFile()),
		},
	},
}

func setDebug(enabled bool) func(c *cli.Context) error {
	return func(c *cli.Context) error {
		client := c.App.Metadata["client"].(*APIClient)
		_, err := client.Patch("config/gui", fmt.Sprintf(`{"debugging": %v}`, enabled))
		return err
	}
}

func debugFile() cli.ActionFunc {
	return func(c *cli.Context) error {
		return dumpOutput(fmt.Sprintf("debug/file?folder=%v&file=%v", c.Args()[0], c.Args()[1]))(c)
	}
}
