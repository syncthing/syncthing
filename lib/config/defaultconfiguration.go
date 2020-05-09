// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package config

import "github.com/syncthing/syncthing/lib/protocol"

type DefaultConfiguration struct {
	Folder FolderDefaultConfig `xml:"folder" json:"folder"`
	Device DeviceDefaultConfig `xml:"device" json:"device"`
}

type FolderDefaultConfig struct {
	RescanIntervalS int                     `xml:"rescanIntervalS,attr" json:"rescanIntervalS" default:"3600"`
	Type            FolderType              `xml:"type,attr" json:"type"`
	Versioning      VersioningConfiguration `xml:"versioning" json:"versioning"`
	Order           PullOrder               `xml:"order" json:"order"`
	MinDiskFree     Size                    `xml:"minDiskFree" json:"minDiskFree" default:"1%"`
}

type DeviceDefaultConfig struct {
	Compression       protocol.Compression `xml:"compression,attr" json:"compression"`
	Introducer        bool                 `xml:"introducer,attr" json:"introducer"`
	AutoAcceptFolders bool                 `xml:"autoAcceptFolders" json:"autoAcceptFolders"`
	MaxSendKbps       int                  `xml:"maxSendKbps" json:"maxSendKbps"`
	MaxRecvKbps       int                  `xml:"maxRecvKbps" json:"maxRecvKbps"`
}

func (defCfg DefaultConfiguration) SetDefaultFolderConf(conf FolderConfiguration) {
	conf.RescanIntervalS = defCfg.Folder.RescanIntervalS
	conf.Type = defCfg.Folder.Type
	conf.Versioning = defCfg.Folder.Versioning
	conf.Order = defCfg.Folder.Order
	conf.MinDiskFree = defCfg.Folder.MinDiskFree
}
