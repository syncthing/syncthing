// Copyright (C) 2021 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// Package generate implements the `syncthing generate` subcommand.
package generate

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"os"

	"github.com/syncthing/syncthing/cmd/syncthing/cmdutil"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/locations"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/syncthing"
)

type CLI struct {
	cmdutil.CommonOptions
	GUIUser     string `placeholder:"STRING" help:"Specify new GUI authentication user name"`
	GUIPassword string `placeholder:"STRING" help:"Specify new GUI authentication password (use - to read from standard input)"`
}

func (c *CLI) Run() error {
	log.SetFlags(0)

	if c.HideConsole {
		osutil.HideConsole()
	}

	if c.HomeDir != "" {
		if c.ConfDir != "" {
			return fmt.Errorf("--home must not be used together with --config")
		}
		c.ConfDir = c.HomeDir
	}
	if c.ConfDir == "" {
		c.ConfDir = locations.GetBaseDir(locations.ConfigBaseDir)
	}

	// Support reading the password from a pipe or similar
	if c.GUIPassword == "-" {
		reader := bufio.NewReader(os.Stdin)
		password, _, err := reader.ReadLine()
		if err != nil {
			return fmt.Errorf("Failed reading GUI password: %w", err)
		}
		c.GUIPassword = string(password)
	}

	if err := Generate(c.ConfDir, c.GUIUser, c.GUIPassword, c.NoDefaultFolder); err != nil {
		return fmt.Errorf("Failed to generate config and keys: %w", err)
	}
	return nil
}

func Generate(confDir, guiUser, guiPassword string, noDefaultFolder bool) error {
	dir, err := fs.ExpandTilde(confDir)
	if err != nil {
		return err
	}

	if err := syncthing.EnsureDir(dir, 0700); err != nil {
		return err
	}
	locations.SetBaseDir(locations.ConfigBaseDir, dir)

	var myID protocol.DeviceID
	certFile, keyFile := locations.Get(locations.CertFile), locations.Get(locations.KeyFile)
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err == nil {
		log.Println("WARNING: Key exists; will not overwrite.")
	} else {
		cert, err = syncthing.GenerateCertificate(certFile, keyFile)
		if err != nil {
			return fmt.Errorf("create certificate: %w", err)
		}
	}
	myID = protocol.NewDeviceID(cert.Certificate[0])
	log.Println("Device ID:", myID)

	cfgFile := locations.Get(locations.ConfigFile)
	var cfg config.Wrapper
	if _, err := os.Stat(cfgFile); err == nil {
		if guiUser == "" && guiPassword == "" {
			log.Println("WARNING: Config exists; will not overwrite.")
			return nil
		}

		if cfg, _, err = config.Load(cfgFile, myID, events.NoopLogger); err != nil {
			return fmt.Errorf("load config: %w", err)
		}
	} else {
		if cfg, err = syncthing.DefaultConfig(cfgFile, myID, events.NoopLogger, noDefaultFolder); err != nil {
			return fmt.Errorf("create config: %w", err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	go cfg.Serve(ctx)
	defer cancel()

	guiCfg := cfg.GUI()
	waiter, err := cfg.Modify(func(cfg *config.Configuration) {
		if changed := updateGUIAuthentication(&guiCfg, guiUser, guiPassword); changed {
			cfg.GUI = guiCfg
		}
	})
	if err != nil {
		return fmt.Errorf("modify config: %w", err)
	}

	waiter.Wait()
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	return nil
}

func updateGUIAuthentication(guiCfg *config.GUIConfiguration, guiUser, guiPassword string) bool {
	changed := false
	if guiUser != "" && guiCfg.User != guiUser {
		guiCfg.User = guiUser
		log.Println("Updated GUI authentication user name:", guiUser)
		changed = true
	}

	if guiPassword != "" && guiCfg.Password != guiPassword {
		if err := guiCfg.HashAndSetPassword(guiPassword); err != nil {
			log.Fatal("Failed to set GUI authentication password.")
		} else {
			log.Println("Updated GUI authentication password.")
			changed = true
		}
	}
	return changed
}
