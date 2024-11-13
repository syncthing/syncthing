// Copyright (C) 2024 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package config

type Configuration struct {
	Version                  int                   `json:"version" xml:"version,attr"`
	Folders                  []FolderConfiguration `json:"folders" xml:"folder"`
	Devices                  []DeviceConfiguration `json:"devices" xml:"device"`
	GUI                      GUIConfiguration      `json:"gui" xml:"gui"`
	LDAP                     LDAPConfiguration     `json:"ldap" xml:"ldap"`
	Options                  OptionsConfiguration  `json:"options" xml:"options"`
	IgnoredDevices           []ObservedDevice      `json:"remoteIgnoredDevices" xml:"remoteIgnoredDevice"`
	DeprecatedPendingDevices []ObservedDevice      `json:"-" xml:"pendingDevice,omitempty"` // Deprecated: Do not use.
	Defaults                 Defaults              `json:"defaults" xml:"defaults"`
}

type Defaults struct {
	Folder  FolderConfiguration `json:"folder" xml:"folder"`
	Device  DeviceConfiguration `json:"device" xml:"device"`
	Ignores Ignores             `json:"ignores" xml:"ignores"`
}

type Ignores struct {
	Lines []string `json:"lines" xml:"line"`
}
