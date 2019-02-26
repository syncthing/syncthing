// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/util"
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

func (c *mockedConfig) LDAP() config.LDAPConfiguration {
	return config.LDAPConfiguration{}
}

func (c *mockedConfig) RawCopy() config.Configuration {
	cfg := config.Configuration{}
	util.SetDefaults(&cfg.Options)
	return cfg
}

func (c *mockedConfig) Options() config.OptionsConfiguration {
	return config.OptionsConfiguration{}
}

func (c *mockedConfig) Replace(cfg config.Configuration) (config.Waiter, error) {
	return noopWaiter{}, nil
}

func (c *mockedConfig) Subscribe(cm config.Committer) {}

func (c *mockedConfig) Unsubscribe(cm config.Committer) {}

func (c *mockedConfig) Folders() map[string]config.FolderConfiguration {
	return nil
}

func (c *mockedConfig) Devices() map[protocol.DeviceID]config.DeviceConfiguration {
	return nil
}

func (c *mockedConfig) SetDevice(config.DeviceConfiguration) (config.Waiter, error) {
	return noopWaiter{}, nil
}

func (c *mockedConfig) SetDevices([]config.DeviceConfiguration) (config.Waiter, error) {
	return noopWaiter{}, nil
}

func (c *mockedConfig) Save() error {
	return nil
}

func (c *mockedConfig) RequiresRestart() bool {
	return false
}

func (c *mockedConfig) AddOrUpdatePendingDevice(device protocol.DeviceID, name, address string) {}

func (c *mockedConfig) AddOrUpdatePendingFolder(id, label string, device protocol.DeviceID) {}

func (m *mockedConfig) MyName() string {
	return ""
}

func (m *mockedConfig) ConfigPath() string {
	return ""
}

func (m *mockedConfig) SetGUI(gui config.GUIConfiguration) (config.Waiter, error) {
	return noopWaiter{}, nil
}

func (m *mockedConfig) SetOptions(opts config.OptionsConfiguration) (config.Waiter, error) {
	return noopWaiter{}, nil
}

func (m *mockedConfig) Folder(id string) (config.FolderConfiguration, bool) {
	return config.FolderConfiguration{}, false
}

func (m *mockedConfig) FolderList() []config.FolderConfiguration {
	return nil
}

func (m *mockedConfig) SetFolder(fld config.FolderConfiguration) (config.Waiter, error) {
	return noopWaiter{}, nil
}

func (m *mockedConfig) Device(id protocol.DeviceID) (config.DeviceConfiguration, bool) {
	return config.DeviceConfiguration{}, false
}

func (m *mockedConfig) RemoveDevice(id protocol.DeviceID) (config.Waiter, error) {
	return noopWaiter{}, nil
}

func (m *mockedConfig) IgnoredDevice(id protocol.DeviceID) bool {
	return false
}

func (m *mockedConfig) IgnoredFolder(device protocol.DeviceID, folder string) bool {
	return false
}

func (m *mockedConfig) GlobalDiscoveryServers() []string {
	return nil
}

type noopWaiter struct{}

func (noopWaiter) Wait() {}
