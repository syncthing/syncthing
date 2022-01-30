// Copyright (C) 2021 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package cli

import (
	"net/url"

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
			Name:  "folders",
			Usage: "Show pending folders",
			Flags: []cli.Flag{
				cli.StringFlag{Name: "device", Usage: "Show pending folders offered by given device"},
			},
			Action: expects(0, folders()),
		},
	},
}

func folders() cli.ActionFunc {
	return func(c *cli.Context) error {
		if c.String("device") != "" {
			query := make(url.Values)
			query.Set("device", c.String("device"))
			return indexDumpOutput("cluster/pending/folders?" + query.Encode())(c)
		}
		return indexDumpOutput("cluster/pending/folders")(c)
	}
}
