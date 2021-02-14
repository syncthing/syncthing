// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package cli

import (
	"bufio"
	"crypto/tls"
	"encoding/json"
	"log"
	"os"
	"reflect"

	"github.com/AudriusButkevicius/recli"
	"github.com/flynn-archive/go-shlex"
	"github.com/mattn/go-isatty"
	"github.com/pkg/errors"

	"github.com/syncthing/syncthing/lib/build"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/locations"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/svcutil"

	"github.com/urfave/cli"
)

type CLI struct {
	GUIAddress string   `name:"gui-address" placeholder:"URL" help:"Override GUI address (e.g. \"http://192.0.2.42:8443\")"`
	GUIAPIKey  string   `name:"gui-apikey" placeholder:"API-KEY" help:"Override GUI API key"`
	HomeDir    string   `name:"home" placeholder:"PATH" help:"Set configuration and data directory"`
	ConfDir    string   `name:"conf" placeholder:"PATH" help:"Set configuration directory (config and keys)"`
	Args       []string `arg:"" optional:""`
}

func (c *CLI) Run() error {
	// Not set as default above because the strings can be really long.
	var err error
	homeSet := c.HomeDir != ""
	confSet := c.ConfDir != ""
	switch {
	case homeSet && confSet:
		err = errors.New("-home must not be used together with -conf")
	case homeSet:
		err = locations.SetBaseDir(locations.ConfigBaseDir, c.HomeDir)
	case confSet:
		err = locations.SetBaseDir(locations.ConfigBaseDir, c.ConfDir)
	}
	if err != nil {
		log.Println("Command line options:", err)
		os.Exit(svcutil.ExitError.AsInt())
	}
	guiCfg := config.GUIConfiguration{
		RawAddress: c.GUIAddress,
		APIKey:     c.GUIAPIKey,
	}

	// Now if the API key and address is not provided (we are not connecting to a remote instance),
	// try to rip it out of the config.
	if guiCfg.RawAddress == "" && guiCfg.APIKey == "" {
		// Load the certs and get the ID
		cert, err := tls.LoadX509KeyPair(
			locations.Get(locations.CertFile),
			locations.Get(locations.KeyFile),
		)
		if err != nil {
			return errors.Wrap(err, "reading device ID")
		}

		myID := protocol.NewDeviceID(cert.Certificate[0])

		// Load the config
		cfg, _, err := config.Load(locations.Get(locations.ConfigFile), myID, events.NoopLogger)
		if err != nil {
			return errors.Wrap(err, "loading config")
		}

		guiCfg = cfg.GUI()
	} else if guiCfg.Address() == "" || guiCfg.APIKey == "" {
		return errors.New("Both --gui-address and --gui-apikey should be specified")
	}

	if guiCfg.Address() == "" {
		return errors.New("Could not find GUI Address")
	}

	if guiCfg.APIKey == "" {
		return errors.New("Could not find GUI API key")
	}

	client := getClient(guiCfg)

	cfg, err := getConfig(client)
	original := cfg.Copy()
	if err != nil {
		return errors.Wrap(err, "getting config")
	}

	// Copy the config and set the default flags
	recliCfg := recli.DefaultConfig
	recliCfg.IDTag.Name = "xml"
	recliCfg.SkipTag.Name = "json"

	commands, err := recli.New(recliCfg).Construct(&cfg)
	if err != nil {
		return errors.Wrap(err, "config reflect")
	}

	// Implement the same flags at the upper CLI, but do nothing with them.
	// This is so that the usage text is the same
	fakeFlags := []cli.Flag{
		cli.StringFlag{
			Name:  "gui-address",
			Value: "URL",
			Usage: "Override GUI address (e.g. \"http://192.0.2.42:8443\")",
		},
		cli.StringFlag{
			Name:  "gui-apikey",
			Value: "API-KEY",
			Usage: "Override GUI API key",
		},
		cli.StringFlag{
			Name:  "home",
			Value: "PATH",
			Usage: "Set configuration and data directory",
		},
		cli.StringFlag{
			Name:  "conf",
			Value: "PATH",
			Usage: "Set configuration directory (config and keys)",
		},
	}

	// Construct the actual CLI
	app := cli.NewApp()
	app.Name = "syncthing"
	app.HelpName = app.Name
	app.Author = "The Syncthing Authors"
	app.Usage = "Syncthing command line interface"
	app.Version = build.Version
	app.Flags = fakeFlags
	app.Metadata = map[string]interface{}{
		"client": client,
	}
	app.Commands = []cli.Command{
		{
			Name:        "config",
			HideHelp:    true,
			Usage:       "Configuration modification command group",
			Subcommands: commands,
		},
		showCommand,
		operationCommand,
		errorsCommand,
	}

	// It expects to be give os.Args which has argv[0] set to executable name, so fake it.
	c.Args = append([]string{"cli"}, c.Args...)

	tty := isatty.IsTerminal(os.Stdin.Fd()) || isatty.IsCygwinTerminal(os.Stdin.Fd())
	if !tty {
		// Not a TTY, consume from stdin
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			input, err := shlex.Split(scanner.Text())
			if err != nil {
				return errors.Wrap(err, "parsing input")
			}
			if len(input) == 0 {
				continue
			}
			err = app.Run(append(c.Args, input...))
			if err != nil {
				return err
			}
		}
		err = scanner.Err()
		if err != nil {
			return err
		}
	} else {
		err = app.Run(c.Args)
		if err != nil {
			return err
		}
	}

	if !reflect.DeepEqual(cfg, original) {
		body, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			return err
		}
		resp, err := client.Post("system/config", string(body))
		if err != nil {
			return err
		}
		if resp.StatusCode != 200 {
			body, err := responseToBArray(resp)
			if err != nil {
				return err
			}
			return errors.New(string(body))
		}
	}
	return nil
}
