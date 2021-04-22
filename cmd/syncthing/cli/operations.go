// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package cli

import (
	"fmt"

	"github.com/urfave/cli"
)

var operationCommand = cli.Command{
	Name:     "operations",
	HideHelp: true,
	Usage:    "Operation command group",
	Subcommands: []cli.Command{
		{
			Name:   "restart",
			Usage:  "Restart syncthing",
			Action: expects(0, emptyPost("system/restart")),
		},
		{
			Name:   "shutdown",
			Usage:  "Shutdown syncthing",
			Action: expects(0, emptyPost("system/shutdown")),
		},
		{
			Name:   "upgrade",
			Usage:  "Upgrade syncthing (if a newer version is available)",
			Action: expects(0, emptyPost("system/upgrade")),
		},
		{
			Name:      "folder-override",
			Usage:     "Override changes on folder (remote for sendonly, local for receiveonly). WARNING: Destructive - deletes/changes your data.",
			ArgsUsage: "[folder id]",
			Action:    expects(1, foldersOverride),
		},
	},
}

func foldersOverride(c *cli.Context) error {
	client := c.App.Metadata["client"].(APIClient)
	cfg, err := getConfig(client)
	if err != nil {
		return err
	}
	rid := c.Args()[0]
	for _, folder := range cfg.Folders {
		if folder.ID == rid {
			response, err := client.Post("db/override", "")
			if err != nil {
				return err
			}
			if response.StatusCode != 200 {
				errStr := fmt.Sprint("Failed to override changes\nStatus code: ", response.StatusCode)
				bytes, err := responseToBArray(response)
				if err != nil {
					return err
				}
				body := string(bytes)
				if body != "" {
					errStr += "\nBody: " + body
				}
				return fmt.Errorf(errStr)
			}
			return nil
		}
	}
	return fmt.Errorf("Folder " + rid + " not found")
}
