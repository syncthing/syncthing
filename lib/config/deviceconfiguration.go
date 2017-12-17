// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package config

import "github.com/syncthing/syncthing/lib/protocol"

type DeviceConfiguration struct {
	DeviceID                 protocol.DeviceID    `xml:"id,attr" json:"deviceID"`
	Name                     string               `xml:"name,attr,omitempty" json:"name"`
	Addresses                []string             `xml:"address,omitempty" json:"addresses"`
	Compression              protocol.Compression `xml:"compression,attr" json:"compression"`
	CertName                 string               `xml:"certName,attr,omitempty" json:"certName"`
	Introducer               bool                 `xml:"introducer,attr" json:"introducer"`
	SkipIntroductionRemovals bool                 `xml:"skipIntroductionRemovals,attr" json:"skipIntroductionRemovals"`
	IntroducedBy             protocol.DeviceID    `xml:"introducedBy,attr" json:"introducedBy"`
	Paused                   bool                 `xml:"paused" json:"paused"`
	AllowedNetworks          []string             `xml:"allowedNetwork,omitempty" json:"allowedNetworks"`
	AutoAcceptFolders        bool                 `xml:"autoAcceptFolders" json:"autoAcceptFolders"`
	MaxSendKbps              int                  `xml:"maxSendKbps" json:"maxSendKbps"`
	MaxRecvKbps              int                  `xml:"maxRecvKbps" json:"maxRecvKbps"`
}

func NewDeviceConfiguration(id protocol.DeviceID, name string) DeviceConfiguration {
	d := DeviceConfiguration{
		DeviceID: id,
		Name:     name,
	}
	d.prepare()
	return d
}

func (cfg DeviceConfiguration) Copy() DeviceConfiguration {
	c := cfg
	c.Addresses = make([]string, len(cfg.Addresses))
	copy(c.Addresses, cfg.Addresses)
	c.AllowedNetworks = make([]string, len(cfg.AllowedNetworks))
	copy(c.AllowedNetworks, cfg.AllowedNetworks)
	return c
}

func (cfg *DeviceConfiguration) prepare() {
	if len(cfg.Addresses) == 0 || len(cfg.Addresses) == 1 && cfg.Addresses[0] == "" {
		cfg.Addresses = []string{"dynamic"}
	}
	if len(cfg.AllowedNetworks) == 0 {
		cfg.AllowedNetworks = []string{}
	}
}

type DeviceConfigurationList []DeviceConfiguration

func (l DeviceConfigurationList) Less(a, b int) bool {
	return l[a].DeviceID.Compare(l[b].DeviceID) == -1
}

func (l DeviceConfigurationList) Swap(a, b int) {
	l[a], l[b] = l[b], l[a]
}

func (l DeviceConfigurationList) Len() int {
	return len(l)
}
