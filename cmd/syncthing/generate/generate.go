// Copyright (C) 2021 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// Package generate implements the `syncthing generate` subcommand.
package generate

import (
	"crypto/tls"
	"fmt"
	"log"
	"os"

	"github.com/pkg/errors"

	"github.com/syncthing/syncthing/cmd/syncthing/cmdutil"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/locations"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/syncthing"
)

type CLI struct {
	cmdutil.CommonOptions
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

	if err := Generate(c.ConfDir, c.NoDefaultFolder); err != nil {
		return errors.Wrap(err, "Failed to generate config and keys")
	}
	return nil
}

func Generate(confDir string, noDefaultFolder bool) error {
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
			return errors.Wrap(err, "create certificate")
		}
	}
	myID = protocol.NewDeviceID(cert.Certificate[0])
	log.Println("Device ID:", myID)

	cfgFile := locations.Get(locations.ConfigFile)
	if _, err := os.Stat(cfgFile); err == nil {
		log.Println("WARNING: Config exists; will not overwrite.")
		return nil
	}
	cfg, err := syncthing.DefaultConfig(cfgFile, myID, events.NoopLogger, noDefaultFolder)
	if err != nil {
		return err
	}
	if err := cfg.Save(); err != nil {
		return errors.Wrap(err, "save config")
	}
	return nil
}
