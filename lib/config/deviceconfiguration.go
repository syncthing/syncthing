// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package config

import (
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
	c.PendingFolders = make([]ObservedFolder, len(cfg.PendingFolders))
	copy(c.PendingFolders, cfg.PendingFolders)
	return c
}

func (cfg *DeviceConfiguration) prepare(sharedFolders []string) {
	if len(cfg.Addresses) == 0 || len(cfg.Addresses) == 1 && cfg.Addresses[0] == "" {
		cfg.Addresses = []string{"dynamic"}
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
