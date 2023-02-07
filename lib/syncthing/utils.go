// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package syncthing

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/db/backend"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/locations"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/tlsutil"
)

func EnsureDir(dir string, mode fs.FileMode) error {
	fs := fs.NewFilesystem(fs.FilesystemTypeBasic, dir)
	err := fs.MkdirAll(".", mode)
	if err != nil {
		return err
	}

	if fi, err := fs.Stat("."); err == nil {
		// Apparently the stat may fail even though the mkdirall passed. If it
		// does, we'll just assume things are in order and let other things
		// fail (like loading or creating the config...).
		currentMode := fi.Mode() & 0777
		if currentMode != mode {
			err := fs.Chmod(".", mode)
			// This can fail on crappy filesystems, nothing we can do about it.
			if err != nil {
				l.Warnln(err)
			}
		}
	}
	return nil
}

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

func DefaultConfig(path string, myID protocol.DeviceID, evLogger events.Logger, noDefaultFolder, skipPortProbing bool) (config.Wrapper, error) {
	newCfg := config.New(myID)

	if skipPortProbing {
		l.Infoln("Using default network port numbers instead of probing for free ports")
	} else if err := newCfg.ProbeFreePorts(); err != nil {
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
	fcfg.Path = locations.GetRelative(locations.DefFolder)
	newCfg.Folders = append(newCfg.Folders, fcfg)
	l.Infoln("Default folder created and/or linked to new config")
	return config.Wrap(path, newCfg, myID, evLogger), nil
}

// LoadConfigAtStartup loads an existing config. If it doesn't yet exist, it
// creates a default one, without the default folder if noDefaultFolder is true.
// Otherwise it checks the version, and archives and upgrades the config if
// necessary or returns an error, if the version isn't compatible.
func LoadConfigAtStartup(path string, cert tls.Certificate, evLogger events.Logger, allowNewerConfig, noDefaultFolder, skipPortProbing bool) (config.Wrapper, error) {
	myID := protocol.NewDeviceID(cert.Certificate[0])
	cfg, originalVersion, err := config.Load(path, myID, evLogger)
	if fs.IsNotExist(err) {
		cfg, err = DefaultConfig(path, myID, evLogger, noDefaultFolder, skipPortProbing)
		if err != nil {
			return nil, fmt.Errorf("failed to generate default config: %w", err)
		}
		err = cfg.Save()
		if err != nil {
			return nil, fmt.Errorf("failed to save default config: %w", err)
		}
		l.Infof("Default config saved. Edit %s to taste (with Syncthing stopped) or use the GUI", cfg.ConfigPath())
	} else if err == io.EOF {
		return nil, errors.New("failed to load config: unexpected end of file. Truncated or empty configuration?")
	} else if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
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
			return nil, fmt.Errorf("config archive: %w", err)
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
	bs, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	if err := os.WriteFile(dst, bs, 0600); err != nil {
		// Attempt to clean up
		os.Remove(dst)
		return err
	}

	return nil
}

func OpenDBBackend(path string, tuning config.Tuning) (backend.Backend, error) {
	return backend.Open(path, backend.Tuning(tuning))
}
