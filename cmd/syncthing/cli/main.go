// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/flynn-archive/go-shlex"
	"github.com/pkg/errors"
	"github.com/urfave/cli"

	"github.com/syncthing/syncthing/cmd/syncthing/cmdutil"
	"github.com/syncthing/syncthing/lib/config"
)

type preCli struct {
	GUIAddress string `name:"gui-address"`
	GUIAPIKey  string `name:"gui-apikey"`
	HomeDir    string `name:"home"`
	ConfDir    string `name:"config"`
	DataDir    string `name:"data"`
}

func Run() error {
	// This is somewhat a hack around a chicken and egg problem. We need to set
	// the home directory and potentially other flags to know where the
	// syncthing instance is running in order to get it's config ... which we
	// then use to construct the actual CLI ... at which point it's too late to
	// add flags there...
	c := preCli{}
	parseFlags(&c)

	// Not set as default above because the strings can be really long.
	err := cmdutil.SetConfigDataLocationsFromFlags(c.HomeDir, c.ConfDir, c.DataDir)
	if err != nil {
		return errors.Wrap(err, "Command line options:")
	}
	clientFactory := &apiClientFactory{
		cfg: config.GUIConfiguration{
			RawAddress: c.GUIAddress,
			APIKey:     c.GUIAPIKey,
		},
	}

	configCommand, err := getConfigCommand(clientFactory)
	if err != nil {
		return err
	}

	// Implement the same flags at the upper CLI, but do nothing with them.
	// This is so that the usage text is the same
	fakeFlags := []cli.Flag{
		cli.StringFlag{
			Name:  "gui-address",
			Usage: "Override GUI address to `URL` (e.g. \"http://192.0.2.42:8443\")",
		},
		cli.StringFlag{
			Name:  "gui-apikey",
			Usage: "Override GUI API key to `API-KEY`",
		},
		cli.StringFlag{
			Name:  "home",
			Usage: "Set configuration and data directory to `PATH`",
		},
		cli.StringFlag{
			Name:  "config",
			Usage: "Set configuration directory (config and keys) to `PATH`",
		},
		cli.StringFlag{
			Name:  "data",
			Usage: "Set data directory (database and logs) to `PATH`",
		},
	}

	// Construct the actual CLI
	app := cli.NewApp()
	app.Author = "The Syncthing Authors"
	app.Metadata = map[string]interface{}{
		"clientFactory": clientFactory,
	}
	app.Commands = []cli.Command{{
		Name:  "cli",
		Usage: "Syncthing command line interface",
		Flags: fakeFlags,
		Subcommands: []cli.Command{
			configCommand,
			showCommand,
			awaitInsyncCommand,
			operationCommand,
			errorsCommand,
			debugCommand,
			{
				Name:     "-",
				HideHelp: true,
				Usage:    "Read commands from stdin",
				Action: func(ctx *cli.Context) error {
					if ctx.NArg() > 0 {
						return errors.New("command does not expect any arguments")
					}

					// Drop the `-` not to recurse into self.
					args := make([]string, len(os.Args)-1)
					copy(args, os.Args)

					fmt.Println("Reading commands from stdin...", args)
					scanner := bufio.NewScanner(os.Stdin)
					for scanner.Scan() {
						input, err := shlex.Split(scanner.Text())
						if err != nil {
							return errors.Wrap(err, "parsing input")
						}
						if len(input) == 0 {
							continue
						}
						err = app.Run(append(args, input...))
						if err != nil {
							return err
						}
					}
					return scanner.Err()
				},
			},
		},
	}}

	return app.Run(os.Args)
}

func parseFlags(c *preCli) error {
	// kong only needs to parse the global arguments after "cli" and before the
	// subcommand (if any).
	if len(os.Args) <= 2 {
		return nil
	}
	args := os.Args[2:]
	for i := 0; i < len(args); i++ {
		if !strings.HasPrefix(args[i], "--") {
			args = args[:i]
			break
		}
		if !strings.Contains(args[i], "=") {
			i++
		}
	}
	// We don't want kong to print anything nor os.Exit (e.g. on -h)
	parser, err := kong.New(c, kong.Writers(io.Discard, io.Discard), kong.Exit(func(int) {}))
	if err != nil {
		return err
	}
	_, err = parser.Parse(args)
	return err
}
