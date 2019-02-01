// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"bufio"
	"crypto/tls"
	"encoding/json"
	"flag"
	"log"
	"os"
	"reflect"
	"strings"

	"github.com/AudriusButkevicius/recli"
	"github.com/flynn-archive/go-shlex"
	"github.com/mattn/go-isatty"
	"github.com/pkg/errors"
	"github.com/syncthing/syncthing/lib/build"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/locations"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/urfave/cli"
)

func main() {
	// This is somewhat a hack around a chicken and egg problem.
	// We need to set the home directory and potentially other flags to know where the syncthing instance is running
	// in order to get it's config ... which we then use to construct the actual CLI ... at which point it's too late
	// to add flags there...
	homeBaseDir := locations.GetBaseDir(locations.ConfigBaseDir)
	guiCfg := config.GUIConfiguration{}

	flags := flag.NewFlagSet("", flag.ContinueOnError)
	flags.StringVar(&guiCfg.RawAddress, "gui-address", guiCfg.RawAddress, "Override GUI address (e.g. \"http://192.0.2.42:8443\")")
	flags.StringVar(&guiCfg.APIKey, "gui-apikey", guiCfg.APIKey, "Override GUI API key")
	flags.StringVar(&homeBaseDir, "home", homeBaseDir, "Set configuration directory")

	// Implement the same flags at the lower CLI, with the same default values (pre-parse), but do nothing with them.
	// This is so that we could reuse os.Args
	fakeFlags := []cli.Flag{
		cli.StringFlag{
			Name:  "gui-address",
			Value: guiCfg.RawAddress,
			Usage: "Override GUI address (e.g. \"http://192.0.2.42:8443\")",
		},
		cli.StringFlag{
			Name:  "gui-apikey",
			Value: guiCfg.APIKey,
			Usage: "Override GUI API key",
		},
		cli.StringFlag{
			Name:  "home",
			Value: homeBaseDir,
			Usage: "Set configuration directory",
		},
	}

	// Do not print usage of these flags, and ignore errors as this can't understand plenty of things
	flags.Usage = func() {}
	_ = flags.Parse(os.Args[1:])

	// Now if the API key and address is not provided (we are not connecting to a remote instance),
	// try to rip it out of the config.
	if guiCfg.RawAddress == "" && guiCfg.APIKey == "" {
		// Update the base directory
		err := locations.SetBaseDir(locations.ConfigBaseDir, homeBaseDir)
		if err != nil {
			log.Fatal(errors.Wrap(err, "setting home"))
		}

		// Load the certs and get the ID
		cert, err := tls.LoadX509KeyPair(
			locations.Get(locations.CertFile),
			locations.Get(locations.KeyFile),
		)
		if err != nil {
			log.Fatal(errors.Wrap(err, "reading device ID"))
		}

		myID := protocol.NewDeviceID(cert.Certificate[0])

		// Load the config
		cfg, err := config.Load(locations.Get(locations.ConfigFile), myID)
		if err != nil {
			log.Fatalln(errors.Wrap(err, "loading config"))
		}

		guiCfg = cfg.GUI()
	} else if guiCfg.Address() == "" || guiCfg.APIKey == "" {
		log.Fatalln("Both -gui-address and -gui-apikey should be specified")
	}

	if guiCfg.Address() == "" {
		log.Fatalln("Could not find GUI Address")
	}

	if guiCfg.APIKey == "" {
		log.Fatalln("Could not find GUI API key")
	}

	client := getClient(guiCfg)

	cfg, err := getConfig(client)
	original := cfg.Copy()
	if err != nil {
		log.Fatalln(errors.Wrap(err, "getting config"))
	}

	// Copy the config and set the default flags
	recliCfg := recli.DefaultConfig
	recliCfg.IDTag.Name = "xml"
	recliCfg.SkipTag.Name = "json"

	commands, err := recli.New(recliCfg).Construct(&cfg)
	if err != nil {
		log.Fatalln(errors.Wrap(err, "config reflect"))
	}

	// Construct the actual CLI
	app := cli.NewApp()
	app.Name = "stcli"
	app.HelpName = app.Name
	app.Author = "The Syncthing Authors"
	app.Usage = "Syncthing command line interface"
	app.Version = strings.Replace(build.LongVersion, "syncthing", app.Name, 1)
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

	tty := isatty.IsTerminal(os.Stdin.Fd()) || isatty.IsCygwinTerminal(os.Stdin.Fd())
	if !tty {
		// Not a TTY, consume from stdin
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			input, err := shlex.Split(scanner.Text())
			if err != nil {
				log.Fatalln(errors.Wrap(err, "parsing input"))
			}
			if len(input) == 0 {
				continue
			}
			err = app.Run(append(os.Args, input...))
			if err != nil {
				log.Fatalln(err)
			}
		}
		err = scanner.Err()
		if err != nil {
			log.Fatalln(err)
		}
	} else {
		err = app.Run(os.Args)
		if err != nil {
			log.Fatalln(err)
		}
	}

	if !reflect.DeepEqual(cfg, original) {
		body, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			log.Fatalln(err)
		}
		resp, err := client.Post("system/config", string(body))
		if err != nil {
			log.Fatalln(err)
		}
		if resp.StatusCode != 200 {
			body, err := responseToBArray(resp)
			if err != nil {
				log.Fatalln(err)
			}
			log.Fatalln(string(body))
		}
	}
}
