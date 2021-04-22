// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/urfave/cli"
)

var errorsCommand = cli.Command{
	Name:     "errors",
	HideHelp: true,
	Usage:    "Error command group",
	Subcommands: []cli.Command{
		{
			Name:   "show",
			Usage:  "Show pending errors",
			Action: expects(0, indexDumpOutput("system/error")),
		},
		{
			Name:      "push",
			Usage:     "Push an error to active clients",
			ArgsUsage: "[error message]",
			Action:    expects(1, errorsPush),
		},
		{
			Name:   "clear",
			Usage:  "Clear pending errors",
			Action: expects(0, emptyPost("system/error/clear")),
		},
	},
}

func errorsPush(c *cli.Context) error {
	client := c.App.Metadata["client"].(*APIClient)
	errStr := strings.Join(c.Args(), " ")
	response, err := client.Post("system/error", strings.TrimSpace(errStr))
	if err != nil {
		return err
	}
	if response.StatusCode != 200 {
		errStr = fmt.Sprint("Failed to push error\nStatus code: ", response.StatusCode)
		bytes, err := responseToBArray(response)
		if err != nil {
			return err
		}
		body := string(bytes)
		if body != "" {
			errStr += "\nBody: " + body
		}
		return errors.New(errStr)
	}
	return nil
}
