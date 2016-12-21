// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package main

import (
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/protocol"
)

type mockedConfig struct {
	gui config.GUIConfiguration
}

func (c *mockedConfig) GUI() config.GUIConfiguration {
	return c.gui
}

func (c *mockedConfig) ListenAddresses() []string {
	return nil
}

func (c *mockedConfig) RawCopy() config.Configuration {
	return config.Configuration{}
}

func (c *mockedConfig) Options() config.OptionsConfiguration {
	return config.OptionsConfiguration{}
}

func (c *mockedConfig) Replace(cfg config.Configuration) error {
	return nil
}

func (c *mockedConfig) Subscribe(cm config.Committer) {}

func (c *mockedConfig) Folders() map[string]config.FolderConfiguration {
	return nil
}

func (c *mockedConfig) Devices() map[protocol.DeviceID]config.DeviceConfiguration {
	return nil
}

func (c *mockedConfig) SetDevice(config.DeviceConfiguration) error {
	return nil
}

func (c *mockedConfig) Save() error {
	return nil
}

func (c *mockedConfig) RequiresRestart() bool {
	return false
}
