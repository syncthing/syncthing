// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

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
	Paused                   bool                 `xml:"paused" json:"pause"`
}

func NewDeviceConfiguration(id protocol.DeviceID, name string) DeviceConfiguration {
	return DeviceConfiguration{
		DeviceID: id,
		Name:     name,
	}
}

func (orig DeviceConfiguration) Copy() DeviceConfiguration {
	c := orig
	c.Addresses = make([]string, len(orig.Addresses))
	copy(c.Addresses, orig.Addresses)
	return c
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
