// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package cmdutil

import (
	"errors"

	"github.com/syncthing/syncthing/lib/locations"
)

func SetConfigDataLocationsFromFlags(homeDir, confDir, dataDir string) error {
	homeSet := homeDir != ""
	confSet := confDir != ""
	dataSet := dataDir != ""
	switch {
	case dataSet != confSet:
		return errors.New("either both or none of -conf and -data must be given, use -home to set both at once")
	case homeSet && dataSet:
		return errors.New("-home must not be used together with -conf and -data")
	case homeSet:
		confDir = homeDir
		dataDir = homeDir
		fallthrough
	case dataSet:
		if err := locations.SetBaseDir(locations.ConfigBaseDir, confDir); err != nil {
			return err
		}
		return locations.SetBaseDir(locations.DataBaseDir, dataDir)
	}
	return nil
}
