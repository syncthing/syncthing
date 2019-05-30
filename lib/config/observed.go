// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package config

import (
	"time"

	"github.com/syncthing/syncthing/lib/protocol"
)

type ObservedFolder struct {
	Time  time.Time `xml:"time,attr" json:"time"`
	ID    string    `xml:"id,attr" json:"id"`
	Label string    `xml:"label,attr" json:"label"`
}

type ObservedDevice struct {
	Time    time.Time         `xml:"time,attr" json:"time"`
	ID      protocol.DeviceID `xml:"id,attr" json:"deviceID"`
	Name    string            `xml:"name,attr" json:"name"`
	Address string            `xml:"address,attr,omitempty" json:"address"`
}

type ObservedCandidateDevice struct {
	ID           protocol.DeviceID `xml:"id,attr" json:"deviceID"`
	CertName     string            `xml:"certName,attr,omitempty" json:"certName"`
	Addresses    []string          `xml:"address,omitempty" json:"addresses"`
	IntroducedBy []ObservedDevice  `xml:"introducedBy,omitempty" json:"introducedBy"`
}

// Generate a map of remote devices who introduced this candidate, keyed by device ID
func (d *ObservedCandidateDevice) Introducers() map[protocol.DeviceID]ObservedDevice {
	introducerMap := make(map[protocol.DeviceID]ObservedDevice, len(d.IntroducedBy))
	for _, dev := range d.IntroducedBy {
		introducerMap[dev.ID] = dev
	}
	return introducerMap
}

// Add or update information from an introducer to the candidate device description
func (d *ObservedCandidateDevice) SetIntroducer(introducer protocol.DeviceID, name string) {
	// Sort devices who introduced this candidate into a map for deduplication
	introducerMap := d.Introducers()
	introducerMap[introducer] = ObservedDevice{
		Time: time.Now().Round(time.Second),
		ID:   introducer,
		Name: name,
	}
	d.IntroducedBy = make([]ObservedDevice, 0, len(introducerMap))
	for _, n := range introducerMap {
		d.IntroducedBy = append(d.IntroducedBy, n)
	}
}

// Collect addresses to try for contacting a candidate device later
func (d *ObservedCandidateDevice) CollectAddresses(addresses []string) {
	if len(addresses) == 0 {
		return
	}
	// Sort addresses into a map for deduplication
	addressMap := make(map[string]struct{}, len(d.Addresses))
	for _, s := range d.Addresses {
		addressMap[s] = struct{}{}
	}
	for _, s := range addresses {
		addressMap[s] = struct{}{}
	}
	d.Addresses = make([]string, 0, len(addressMap))
	for a := range addressMap {
		d.Addresses = append(d.Addresses, a)
	}
}
