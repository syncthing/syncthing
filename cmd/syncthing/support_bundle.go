// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"archive/zip"
	"io"

	"github.com/syncthing/syncthing/lib/config"
)

// GetRedactedConfig redacting some parts of config
func getRedactedConfig(s *apiService) config.Configuration {
	rawConf := s.cfg.RawCopy()
	rawConf.GUI.APIKey = "REDACTED"
	if rawConf.GUI.Password != "" {
		rawConf.GUI.Password = "REDACTED"
	}
	if rawConf.GUI.User != "" {
		rawConf.GUI.User = "REDACTED"
	}
	return rawConf
}

// ZipBuffer creates a zip and adds fileEntry(name, []byte]) array to zip
func zipBuffer(writer io.Writer, files []fileEntry) error {
	zipWriter := zip.NewWriter(writer)
	defer func() {
		if err := zipWriter.Close(); err != nil {
			l.Infoln("Zipwriter not close: ", err)
		}
	}()

	// Add files to zip
	for _, file := range files {
		zipFile, err := zipWriter.Create(file.name)
		if err != nil {
			l.Infoln("Zipwriter not create filename:", err)
			return err
		}
		_, err = zipFile.Write(file.data)
		if err != nil {
			l.Infof("Zipwriter not write the file %s data: %s", file, err)
			return err
		}
	}
	return nil
}
