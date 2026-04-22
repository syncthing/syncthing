// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package api

import (
	"archive/zip"
	"io"

	"github.com/syncthing/syncthing/lib/config"
)

// getRedactedConfig redacting some parts of config
func getRedactedConfig(s *service) config.Configuration {
	rawConf := s.cfg.RawCopy()
	rawConf.GUI.APIKey = "REDACTED"
	if rawConf.GUI.Password != "" {
		rawConf.GUI.Password = "REDACTED"
	}
	if rawConf.GUI.User != "" {
		rawConf.GUI.User = "REDACTED"
	}

	for folderIdx, folderCfg := range rawConf.Folders {
		for deviceIdx, deviceCfg := range folderCfg.Devices {
			if deviceCfg.EncryptionPassword != "" {
				rawConf.Folders[folderIdx].Devices[deviceIdx].EncryptionPassword = "REDACTED"
			}
		}
	}

	return rawConf
}

// writeZip writes a zip file containing the given entries
func writeZip(writer io.Writer, files []fileEntry) error {
	zipWriter := zip.NewWriter(writer)
	defer zipWriter.Close()

	for _, file := range files {
		zipFile, err := zipWriter.Create(file.name)
		if err != nil {
			return err
		}

		_, err = zipFile.Write(file.data)
		if err != nil {
			return err
		}
	}

	return zipWriter.Close()
}
