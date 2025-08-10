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
	"log/slog"
	"os"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/locations"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/syncthing"
)

type CLI struct {
	GUIUser       string `placeholder:"STRING" help:"Specify new GUI authentication user name"`
	GUIPassword   string `placeholder:"STRING" help:"Specify new GUI authentication password (use - to read from standard input)"`
	NoPortProbing bool   `help:"Don't try to find free ports for GUI and listen addresses on first startup" env:"STNOPORTPROBING"`
}

func (c *CLI) Run() error {
	// Support reading the password from a pipe or similar
	if c.GUIPassword == "-" {
		reader := bufio.NewReader(os.Stdin)
		password, _, err := reader.ReadLine()
		if err != nil {
			return fmt.Errorf("failed reading GUI password: %w", err)
		}
		c.GUIPassword = string(password)
	}

	if err := Generate(locations.GetBaseDir(locations.ConfigBaseDir), c.GUIUser, c.GUIPassword, c.NoPortProbing); err != nil {
		return fmt.Errorf("failed to generate config and keys: %w", err)
	}
	return nil
}

func Generate(confDir, guiUser, guiPassword string, skipPortProbing bool) error {
	dir, err := fs.ExpandTilde(confDir)
	if err != nil {
		return err
	}

	if err := syncthing.EnsureDir(dir, 0o700); err != nil {
		return err
	}
	locations.SetBaseDir(locations.ConfigBaseDir, dir)

	var myID protocol.DeviceID
	certFile, keyFile := locations.Get(locations.CertFile), locations.Get(locations.KeyFile)
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err == nil {
		slog.Warn("Key exists; will not overwrite")
	} else {
		cert, err = syncthing.GenerateCertificate(certFile, keyFile)
		if err != nil {
			return fmt.Errorf("create certificate: %w", err)
		}
	}

	myID = protocol.NewDeviceID(cert.Certificate[0])
	slog.Info("Calculated device ID", slog.String("device", myID.String()))

	cfgFile := locations.Get(locations.ConfigFile)
	cfg, _, err := config.Load(cfgFile, myID, events.NoopLogger)
	if fs.IsNotExist(err) {
		if cfg, err = syncthing.DefaultConfig(cfgFile, myID, events.NoopLogger, skipPortProbing); err != nil {
			return fmt.Errorf("create config: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go cfg.Serve(ctx)
	defer cancel()

	var updateErr error
	waiter, err := cfg.Modify(func(cfg *config.Configuration) {
		updateErr = updateGUIAuthentication(&cfg.GUI, guiUser, guiPassword)
	})
	if err != nil {
		return fmt.Errorf("modify config: %w", err)
	}

	waiter.Wait()
	if updateErr != nil {
		return updateErr
	}
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	return nil
}

func updateGUIAuthentication(guiCfg *config.GUIConfiguration, guiUser, guiPassword string) error {
	if guiUser != "" && guiCfg.User != guiUser {
		guiCfg.User = guiUser
		slog.Info("Updated GUI authentication user", "name", guiUser)
	}

	if guiPassword != "" && guiCfg.Password != guiPassword {
		if err := guiCfg.SetPassword(guiPassword); err != nil {
			return fmt.Errorf("failed to set GUI authentication password: %w", err)
		}
		slog.Info("Updated GUI authentication password")
	}
	return nil
}
