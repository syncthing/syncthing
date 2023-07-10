// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package config

import (
	"fmt"
	"sort"
)

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

	// We must always allow at least one connection per device.
	if cfg.MultipleConnections < 1 {
		cfg.MultipleConnections = 1
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
