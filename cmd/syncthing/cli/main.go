// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package cli

import (
	"bufio"
	"fmt"
	"os"

	"github.com/alecthomas/kong"
	"github.com/kballard/go-shellquote"

	"github.com/syncthing/syncthing/cmd/syncthing/cmdutil"
	"github.com/syncthing/syncthing/lib/config"
)

type CLI struct {
	cmdutil.CommonOptions
	DataDir    string `name:"data" placeholder:"PATH" env:"STDATADIR" help:"Set data directory (database and logs)"`
	GUIAddress string `name:"gui-address"`
	GUIAPIKey  string `name:"gui-apikey"`

	Show       showCommand      `cmd:"" help:"Show command group"`
	Debug      debugCommand     `cmd:"" help:"Debug command group"`
	Operations operationCommand `cmd:"" help:"Operation command group"`
	Errors     errorsCommand    `cmd:"" help:"Error command group"`
	Config     configCommand    `cmd:"" help:"Configuration modification command group" passthrough:""`
	Stdin      stdinCommand     `cmd:"" name:"-" help:"Read commands from stdin"`
}

type Context struct {
	clientFactory *apiClientFactory
}

func (cli CLI) AfterApply(kongCtx *kong.Context) error {
	err := cmdutil.SetConfigDataLocationsFromFlags(cli.HomeDir, cli.ConfDir, cli.DataDir)
	if err != nil {
		return fmt.Errorf("command line options: %w", err)
	}

	clientFactory := &apiClientFactory{
		cfg: config.GUIConfiguration{
			RawAddress: cli.GUIAddress,
			APIKey:     cli.GUIAPIKey,
		},
	}

	context := Context{
		clientFactory: clientFactory,
	}

	kongCtx.Bind(context)
	return nil
}

type stdinCommand struct{}

func (*stdinCommand) Run() error {
	// Drop the `-` not to recurse into self.
	args := make([]string, len(os.Args)-1)
	copy(args, os.Args)

	fmt.Println("Reading commands from stdin...", args)
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		input, err := shellquote.Split(scanner.Text())
		if err != nil {
			return fmt.Errorf("parsing input: %w", err)
		}
		if len(input) == 0 {
			continue
		}

		var cli CLI
		p, err := kong.New(&cli)
		if err != nil {
			// can't happen, really
			return fmt.Errorf("creating parser: %w", err)
		}
		ctx, err := p.Parse(input)
		if err != nil {
			fmt.Println("Error:", err)
			continue
		}
		if err := ctx.Run(); err != nil {
			fmt.Println("Error:", err)
			continue
		}
	}
	return scanner.Err()
}
