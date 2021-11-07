// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package syncthing

import (
	"crypto/tls"
	"fmt"
	"io"
	"io/ioutil"
	"os"

	"github.com/pkg/errors"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/db/backend"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/locations"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/tlsutil"
)

func LoadOrGenerateCertificate(certFile, keyFile string) (tls.Certificate, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return GenerateCertificate(certFile, keyFile)
	}
	return cert, nil
}

func GenerateCertificate(certFile, keyFile string) (tls.Certificate, error) {
	l.Infof("Generating ECDSA key and certificate for %s...", tlsDefaultCommonName)
	return tlsutil.NewCertificate(certFile, keyFile, tlsDefaultCommonName, deviceCertLifetimeDays)
}

func DefaultConfig(path string, myID protocol.DeviceID, evLogger events.Logger, noDefaultFolder bool) (config.Wrapper, error) {
	newCfg, err := config.NewWithFreePorts(myID)
	if err != nil {
		return nil, err
	}

	if noDefaultFolder {
		l.Infoln("We will skip creation of a default folder on first start")
		return config.Wrap(path, newCfg, myID, evLogger), nil
	}

	fcfg := newCfg.Defaults.Folder.Copy()
	fcfg.ID = "default"
	fcfg.Label = "Default Folder"
	fcfg.FilesystemType = fs.FilesystemTypeBasic
	fcfg.Path = locations.Get(locations.DefFolder)
	newCfg.Folders = append(newCfg.Folders, fcfg)
	l.Infoln("Default folder created and/or linked to new config")
	return config.Wrap(path, newCfg, myID, evLogger), nil
}

// LoadConfigAtStartup loads an existing config. If it doesn't yet exist, it
// creates a default one, without the default folder if noDefaultFolder is ture.
// Otherwise it checks the version, and archives and upgrades the config if
// necessary or returns an error, if the version isn't compatible.
func LoadConfigAtStartup(path string, cert tls.Certificate, evLogger events.Logger, allowNewerConfig, noDefaultFolder bool) (config.Wrapper, error) {
	myID := protocol.NewDeviceID(cert.Certificate[0])
	cfg, originalVersion, err := config.Load(path, myID, evLogger)
	if fs.IsNotExist(err) {
		cfg, err = DefaultConfig(path, myID, evLogger, noDefaultFolder)
		if err != nil {
			return nil, errors.Wrap(err, "failed to generate default config")
		}
		err = cfg.Save()
		if err != nil {
			return nil, errors.Wrap(err, "failed to save default config")
		}
		l.Infof("Default config saved. Edit %s to taste (with Syncthing stopped) or use the GUI", cfg.ConfigPath())
	} else if err == io.EOF {
		return nil, errors.New("failed to load config: unexpected end of file. Truncated or empty configuration?")
	} else if err != nil {
		return nil, errors.Wrap(err, "failed to load config")
	}

	if originalVersion != config.CurrentVersion {
		if originalVersion == config.CurrentVersion+1101 {
			l.Infof("Now, THAT's what we call a config from the future! Don't worry. As long as you hit that wire with the connecting hook at precisely eighty-eight miles per hour the instant the lightning strikes the tower... everything will be fine.")
		}
		if originalVersion > config.CurrentVersion && !allowNewerConfig {
			return nil, fmt.Errorf("config file version (%d) is newer than supported version (%d). If this is expected, use --allow-newer-config to override.", originalVersion, config.CurrentVersion)
		}
		err = archiveAndSaveConfig(cfg, originalVersion)
		if err != nil {
			return nil, errors.Wrap(err, "config archive")
		}
	}

	return cfg, nil
}

func archiveAndSaveConfig(cfg config.Wrapper, originalVersion int) error {
	// Copy the existing config to an archive copy
	archivePath := cfg.ConfigPath() + fmt.Sprintf(".v%d", originalVersion)
	l.Infoln("Archiving a copy of old config file format at:", archivePath)
	if err := copyFile(cfg.ConfigPath(), archivePath); err != nil {
		return err
	}

	// Do a regular atomic config sve
	return cfg.Save()
}

func copyFile(src, dst string) error {
	bs, err := ioutil.ReadFile(src)
	if err != nil {
		return err
	}

	if err := ioutil.WriteFile(dst, bs, 0600); err != nil {
		// Attempt to clean up
		os.Remove(dst)
		return err
	}

	return nil
}

func OpenDBBackend(path string, tuning config.Tuning) (backend.Backend, error) {
	return backend.Open(path, backend.Tuning(tuning))
}
