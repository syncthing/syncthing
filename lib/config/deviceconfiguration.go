// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package config

import (
	"sort"

	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/util"
)

type DeviceConfiguration struct {
	DeviceID                 protocol.DeviceID    `xml:"id,attr" json:"deviceID"`
	Name                     string               `xml:"name,attr,omitempty" json:"name"`
	Addresses                []string             `xml:"address,omitempty" json:"addresses" default:"dynamic"`
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
	IgnoredFolders           []ObservedFolder     `xml:"ignoredFolder" json:"ignoredFolders"`
	PendingFolders           []ObservedFolder     `xml:"pendingFolder" json:"pendingFolders"`
	MaxRequestKiB            int                  `xml:"maxRequestKiB" json:"maxRequestKiB"`
}

func NewDeviceConfiguration(id protocol.DeviceID, name string) DeviceConfiguration {
	d := DeviceConfiguration{
		DeviceID: id,
		Name:     name,
	}

	util.SetDefaults(&d)

	d.prepare(nil)
	return d
}

func (cfg DeviceConfiguration) Copy() DeviceConfiguration {
	c := cfg
	c.Addresses = make([]string, len(cfg.Addresses))
	copy(c.Addresses, cfg.Addresses)
	c.AllowedNetworks = make([]string, len(cfg.AllowedNetworks))
	copy(c.AllowedNetworks, cfg.AllowedNetworks)
	c.IgnoredFolders = make([]ObservedFolder, len(cfg.IgnoredFolders))
	copy(c.IgnoredFolders, cfg.IgnoredFolders)
	c.PendingFolders = make([]ObservedFolder, len(cfg.PendingFolders))
	copy(c.PendingFolders, cfg.PendingFolders)
	return c
}

func (cfg *DeviceConfiguration) prepare(sharedFolders []string) {
	if len(cfg.Addresses) == 0 || len(cfg.Addresses) == 1 && cfg.Addresses[0] == "" {
		cfg.Addresses = []string{"dynamic"}
	}
	if len(cfg.AllowedNetworks) == 0 {
		cfg.AllowedNetworks = []string{}
	}

	ignoredFolders := deduplicateObservedFoldersToMap(cfg.IgnoredFolders)
	pendingFolders := deduplicateObservedFoldersToMap(cfg.PendingFolders)

	for id := range ignoredFolders {
		delete(pendingFolders, id)
	}

	for _, sharedFolder := range sharedFolders {
		delete(ignoredFolders, sharedFolder)
		delete(pendingFolders, sharedFolder)
	}

	cfg.IgnoredFolders = sortedObservedFolderSlice(ignoredFolders)
	cfg.PendingFolders = sortedObservedFolderSlice(pendingFolders)
}

func (cfg *DeviceConfiguration) IgnoredFolder(folder string) bool {
	for _, ignoredFolder := range cfg.IgnoredFolders {
		if ignoredFolder.ID == folder {
			return true
		}
	}
	return false
}

func sortedObservedFolderSlice(input map[string]ObservedFolder) []ObservedFolder {
	output := make([]ObservedFolder, 0, len(input))
	for _, folder := range input {
		output = append(output, folder)
	}
	sort.Slice(output, func(i, j int) bool {
		return output[i].Time.Before(output[j].Time)
	})
	return output
}

func deduplicateObservedFoldersToMap(input []ObservedFolder) map[string]ObservedFolder {
	output := make(map[string]ObservedFolder, len(input))
	for _, folder := range input {
		if existing, ok := output[folder.ID]; !ok || existing.Time.Before(folder.Time) {
			output[folder.ID] = folder
		}
	}

	return output
}
