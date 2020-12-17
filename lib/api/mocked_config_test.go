// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package api

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

func (c *mockedConfig) SetLDAP(config.LDAPConfiguration) (config.Waiter, error) {
	return noopWaiter{}, nil
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

func (c *mockedConfig) Subscribe(cm config.Committer) config.Configuration {
	return config.Configuration{}
}

func (c *mockedConfig) Unsubscribe(cm config.Committer) {}

func (c *mockedConfig) Folders() map[string]config.FolderConfiguration {
	return nil
}

func (c *mockedConfig) Devices() map[protocol.DeviceID]config.DeviceConfiguration {
	return nil
}

func (c *mockedConfig) DeviceList() []config.DeviceConfiguration {
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

func (c *mockedConfig) ConfigPath() string {
	return ""
}

func (c *mockedConfig) SetGUI(gui config.GUIConfiguration) (config.Waiter, error) {
	return noopWaiter{}, nil
}

func (c *mockedConfig) SetOptions(opts config.OptionsConfiguration) (config.Waiter, error) {
	return noopWaiter{}, nil
}

func (c *mockedConfig) Folder(id string) (config.FolderConfiguration, bool) {
	return config.FolderConfiguration{}, false
}

func (c *mockedConfig) FolderList() []config.FolderConfiguration {
	return nil
}

func (c *mockedConfig) SetFolder(fld config.FolderConfiguration) (config.Waiter, error) {
	return noopWaiter{}, nil
}

func (c *mockedConfig) SetFolders(folders []config.FolderConfiguration) (config.Waiter, error) {
	return noopWaiter{}, nil
}

func (c *mockedConfig) RemoveFolder(id string) (config.Waiter, error) {
	return noopWaiter{}, nil
}

func (c *mockedConfig) FolderPasswords(device protocol.DeviceID) map[string]string {
	return nil
}

func (c *mockedConfig) Device(id protocol.DeviceID) (config.DeviceConfiguration, bool) {
	return config.DeviceConfiguration{}, false
}

func (c *mockedConfig) RemoveDevice(id protocol.DeviceID) (config.Waiter, error) {
	return noopWaiter{}, nil
}

func (c *mockedConfig) IgnoredDevice(id protocol.DeviceID) bool {
	return false
}

func (c *mockedConfig) IgnoredDevices() []config.ObservedDevice {
	return nil
}

func (c *mockedConfig) IgnoredFolder(device protocol.DeviceID, folder string) bool {
	return false
}

func (c *mockedConfig) GlobalDiscoveryServers() []string {
	return nil
}

func (c *mockedConfig) StunServers() []string {
	return nil
}

func (c *mockedConfig) MyID() protocol.DeviceID {
	return protocol.DeviceID{}
}

type noopWaiter struct{}

func (noopWaiter) Wait() {}
