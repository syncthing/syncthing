// Copyright (C) 2022 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// Package override_credentials implements the `syncthing override-credentials` subcommand.
package override_credentials

import (
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/locations"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/syncthing"
)

type CLI struct {
	SourceDirectory string `name:"src-dir" placeholder:"PATH" help:"Directory with generated key, cert and config files"`
}

type SourceDestinationPair struct {
	source      string
	destination string
}

func (c *CLI) Run() error {
	log.SetFlags(0)
	if c.SourceDirectory == "" {
		return fmt.Errorf("--src-dir not specified")
	}

	if err := OverrideCredentials(c.SourceDirectory); err != nil {
		return fmt.Errorf("Failed to override config and keys: %w", err)
	}
	return nil
}

func OverrideCredentials(srcDir string) error {
	dir, err := fs.ExpandTilde(srcDir)
	if err != nil {
		return err
	}
	err = syncthing.EnsureDir(dir, 0700)
	if err != nil {
		return err
	}

	confPath := locations.Get(locations.ConfigFile)
	certPath := locations.Get(locations.CertFile)
	keyPath := locations.Get(locations.KeyFile)

	srcKey := filepath.Join(dir, filepath.Base(keyPath))
	srcCert := filepath.Join(dir, filepath.Base(certPath))
	srcConf := filepath.Join(dir, filepath.Base(confPath))

	_, err = tls.LoadX509KeyPair(srcCert, srcKey)
	if err != nil {
		return fmt.Errorf("Couldn't load certificate from the source directory: %w", err)
	}

	srcDestPairs := [3]SourceDestinationPair{
		{srcKey, keyPath},
		{srcCert, certPath},
		{srcConf, confPath},
	}
	for _, p := range srcDestPairs {
		if p.source == p.destination {
			return fmt.Errorf("Source and destination directories are the same")
		}
	}
	for _, p := range srcDestPairs {
		source, err := os.Open(p.source)
		if err != nil {
			return err
		}
		defer source.Close()
		destination, err := os.OpenFile(p.destination, os.O_WRONLY|os.O_TRUNC, os.ModePerm)
		if err != nil {
			return err
		}
		defer destination.Close()
		_, err = io.Copy(destination, source)
		if err != nil {
			return err
		}
	}

	certFile, keyFile := locations.Get(locations.CertFile), locations.Get(locations.KeyFile)
	cert, _ := tls.LoadX509KeyPair(certFile, keyFile)
	myID := protocol.NewDeviceID(cert.Certificate[0])
	log.Println("New device ID:", myID)

	return nil
}
