// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package config

import (
	"fmt"
	"sort"

	"github.com/syncthing/syncthing/lib/protocol"
)

const defaultNumConnections = 1 // number of connections to use by default; may change in the future.

type DeviceConfiguration struct {
	DeviceID                 protocol.DeviceID `json:"deviceID" xml:"id,attr" nodefault:"true"`
	Name                     string            `json:"name" xml:"name,attr,omitempty"`
	Addresses                []string          `json:"addresses" xml:"address,omitempty"`
	Compression              Compression       `json:"compression" xml:"compression,attr"`
	CertName                 string            `json:"certName" xml:"certName,attr,omitempty"`
	Introducer               bool              `json:"introducer" xml:"introducer,attr"`
	SkipIntroductionRemovals bool              `json:"skipIntroductionRemovals" xml:"skipIntroductionRemovals,attr"`
	IntroducedBy             protocol.DeviceID `json:"introducedBy" xml:"introducedBy,attr" nodefault:"true"`
	Paused                   bool              `json:"paused" xml:"paused"`
	AllowedNetworks          []string          `json:"allowedNetworks" xml:"allowedNetwork,omitempty"`
	AutoAcceptFolders        bool              `json:"autoAcceptFolders" xml:"autoAcceptFolders"`
	MaxSendKbps              int               `json:"maxSendKbps" xml:"maxSendKbps"`
	MaxRecvKbps              int               `json:"maxRecvKbps" xml:"maxRecvKbps"`
	IgnoredFolders           []ObservedFolder  `json:"ignoredFolders" xml:"ignoredFolder"`
	DeprecatedPendingFolders []ObservedFolder  `json:"-" xml:"pendingFolder,omitempty"` // Deprecated: Do not use.
	MaxRequestKiB            int               `json:"maxRequestKiB" xml:"maxRequestKiB"`
	Untrusted                bool              `json:"untrusted" xml:"untrusted"`
	RemoteGUIPort            int               `json:"remoteGUIPort" xml:"remoteGUIPort"`
	RawNumConnections        int               `json:"numConnections" xml:"numConnections"`
}

func (cfg DeviceConfiguration) Copy() DeviceConfiguration {
	c := cfg
	c.Addresses = make([]string, len(cfg.Addresses))
	copy(c.Addresses, cfg.Addresses)
	c.AllowedNetworks = make([]string, len(cfg.AllowedNetworks))
	copy(c.AllowedNetworks, cfg.AllowedNetworks)
	c.IgnoredFolders = make([]ObservedFolder, len(cfg.IgnoredFolders))
	copy(c.IgnoredFolders, cfg.IgnoredFolders)
	return c
}

func (cfg *DeviceConfiguration) prepare(sharedFolders []string) {
	if len(cfg.Addresses) == 0 || len(cfg.Addresses) == 1 && cfg.Addresses[0] == "" {
		cfg.Addresses = []string{"dynamic"}
	}

	ignoredFolders := deduplicateObservedFoldersToMap(cfg.IgnoredFolders)

	for _, sharedFolder := range sharedFolders {
		delete(ignoredFolders, sharedFolder)
	}

	cfg.IgnoredFolders = sortedObservedFolderSlice(ignoredFolders)

	// A device cannot be simultaneously untrusted and an introducer, nor
	// auto accept folders.
	if cfg.Untrusted {
		if cfg.Introducer {
			l.Warnf("Device %s (%s) is both untrusted and an introducer, removing introducer flag", cfg.DeviceID.Short(), cfg.Name)
			cfg.Introducer = false
		}
		if cfg.AutoAcceptFolders {
			l.Warnf("Device %s (%s) is both untrusted and auto-accepting folders, removing auto-accept flag", cfg.DeviceID.Short(), cfg.Name)
			cfg.AutoAcceptFolders = false
		}
	}
}

func (cfg *DeviceConfiguration) NumConnections() int {
	switch {
	case cfg.RawNumConnections == 0:
		return defaultNumConnections
	case cfg.RawNumConnections < 0:
		return 1
	default:
		return cfg.RawNumConnections
	}
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

func (cfg *DeviceConfiguration) Description() string {
	if cfg.Name == "" {
		return cfg.DeviceID.Short().String()
	}
	return fmt.Sprintf("%s (%s)", cfg.Name, cfg.DeviceID.Short())
}
