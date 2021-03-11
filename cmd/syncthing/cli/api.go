// Copyright (C) 2021 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package cli

import (
	"github.com/urfave/cli"
)

var apiCommand = cli.Command{
	Name:     "api",
	HideHelp: true,
	Usage:    "Directly interact with rest api",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "method",
			Usage: "HTTP request method",
			Value: "GET",
		},
		cli.StringFlag{
			Name:  "data",
			Usage: "JSON data to post to the api",
		},
	},
	Action: expects(1, func(c *cli.Context) error {
		client := c.App.Metadata["client"].(*APIClient)
		response, err := client.Do(c.Args().Get(0), c.String("method"), c.String("data"))
		if err != nil {
			return err
		}
		return prettyPrintResponse(c, response)
	}),
}
