// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package config

import (
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/syncthing/syncthing/lib/build"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/netutil"
	"github.com/syncthing/syncthing/lib/upgrade"
)

// migrations is the set of config migration functions, with their target
// config version. The conversion function can be nil in which case we just
// update the config version. The order of migrations doesn't matter here,
// put the newest on top for readability.
var (
	migrations = migrationSet{
		{37, migrateToConfigV37},
		{36, migrateToConfigV36},
		{35, migrateToConfigV35},
		{34, migrateToConfigV34},
		{33, migrateToConfigV33},
		{32, migrateToConfigV32},
		{31, migrateToConfigV31},
		{30, migrateToConfigV30},
		{29, migrateToConfigV29},
		{28, migrateToConfigV28},
		{27, migrateToConfigV27},
		{26, nil}, // triggers database update
		{25, migrateToConfigV25},
		{24, migrateToConfigV24},
		{23, migrateToConfigV23},
		{22, migrateToConfigV22},
		{21, migrateToConfigV21},
		{20, migrateToConfigV20},
		{19, nil}, // Triggers a database tweak
		{18, migrateToConfigV18},
		{17, nil}, // Fsync = true removed
		{16, nil}, // Triggers a database tweak
		{15, migrateToConfigV15},
		{14, migrateToConfigV14},
		{13, migrateToConfigV13},
		{12, migrateToConfigV12},
		{11, migrateToConfigV11},
	}
	migrationsMut = sync.Mutex{}
)

type migrationSet []migration

// apply applies all the migrations in the set, as required by the current
// version and target version, in the correct order.
func (ms migrationSet) apply(cfg *Configuration) {
	// Make sure we apply the migrations in target version order regardless
	// of how it was defined.
	sort.Slice(ms, func(a, b int) bool {
		return ms[a].targetVersion < ms[b].targetVersion
	})

	// Apply all migrations.
	for _, m := range ms {
		m.apply(cfg)
	}
}

// A migration is a target config version and a function to do the needful
// to reach that version. The function does not need to change the actual
// cfg.Version field.
type migration struct {
	targetVersion int
	convert       func(cfg *Configuration)
}

// apply applies the conversion function if the current version is below the
// target version and the function is not nil, and updates the current
// version.
func (m migration) apply(cfg *Configuration) {
	if cfg.Version >= m.targetVersion {
		return
	}
	if m.convert != nil {
		m.convert(cfg)
	}
	cfg.Version = m.targetVersion
}

func migrateToConfigV37(cfg *Configuration) {
	// "scan ownership" changed name to "send ownership"
	for i := range cfg.Folders {
		cfg.Folders[i].SendOwnership = cfg.Folders[i].DeprecatedScanOwnership
		cfg.Folders[i].DeprecatedScanOwnership = false
	}
}

func migrateToConfigV36(cfg *Configuration) {
	for i := range cfg.Folders {
		delete(cfg.Folders[i].Versioning.Params, "cleanInterval")
	}
}

func migrateToConfigV35(cfg *Configuration) {
	for i, fcfg := range cfg.Folders {
		params := fcfg.Versioning.Params
		if params["fsType"] != "" {
			var fsType FilesystemType
			_ = fsType.UnmarshalText([]byte(params["fsType"]))
			cfg.Folders[i].Versioning.FSType = fsType
		}
		if params["versionsPath"] != "" && params["fsPath"] == "" {
			params["fsPath"] = params["versionsPath"]
		}
		cfg.Folders[i].Versioning.FSPath = params["fsPath"]
		delete(cfg.Folders[i].Versioning.Params, "fsType")
		delete(cfg.Folders[i].Versioning.Params, "fsPath")
		delete(cfg.Folders[i].Versioning.Params, "versionsPath")
	}
}

func migrateToConfigV34(cfg *Configuration) {
	cfg.Defaults.Folder.Path = cfg.Options.DeprecatedDefaultFolderPath
	cfg.Options.DeprecatedDefaultFolderPath = ""
}

func migrateToConfigV33(cfg *Configuration) {
	for i := range cfg.Devices {
		cfg.Devices[i].DeprecatedPendingFolders = nil
	}
	cfg.DeprecatedPendingDevices = nil
}

func migrateToConfigV32(cfg *Configuration) {
	for i := range cfg.Folders {
		cfg.Folders[i].JunctionsAsDirs = true
	}
}

func migrateToConfigV31(cfg *Configuration) {
	// Show a notification about setting User and Password
	cfg.Options.UnackedNotificationIDs = append(cfg.Options.UnackedNotificationIDs, "authenticationUserAndPassword")
}

func migrateToConfigV30(cfg *Configuration) {
	// The "max concurrent scans" option is now spelled "max folder concurrency"
	// to be more general.
	cfg.Options.RawMaxFolderConcurrency = cfg.Options.DeprecatedMaxConcurrentScans
	cfg.Options.DeprecatedMaxConcurrentScans = 0
}

func migrateToConfigV29(cfg *Configuration) {
	// The new crash reporting option should follow the state of global
	// discovery / usage reporting, and we should display an appropriate
	// notification.
	if cfg.Options.GlobalAnnEnabled || cfg.Options.URAccepted > 0 {
		cfg.Options.CREnabled = true
		cfg.Options.UnackedNotificationIDs = append(cfg.Options.UnackedNotificationIDs, "crAutoEnabled")
	} else {
		cfg.Options.CREnabled = false
		cfg.Options.UnackedNotificationIDs = append(cfg.Options.UnackedNotificationIDs, "crAutoDisabled")
	}
}

func migrateToConfigV28(cfg *Configuration) {
	// Show a notification about enabling filesystem watching
	cfg.Options.UnackedNotificationIDs = append(cfg.Options.UnackedNotificationIDs, "fsWatcherNotification")
}

func migrateToConfigV27(cfg *Configuration) {
	for i := range cfg.Folders {
		f := &cfg.Folders[i]
		if f.DeprecatedPullers != 0 {
			f.PullerMaxPendingKiB = 128 * f.DeprecatedPullers
			f.DeprecatedPullers = 0
		}
	}
}

func migrateToConfigV25(cfg *Configuration) {
	for i := range cfg.Folders {
		cfg.Folders[i].FSWatcherDelayS = 10
	}
}

func migrateToConfigV24(cfg *Configuration) {
	cfg.Options.URSeen = 2
}

func migrateToConfigV23(cfg *Configuration) {
	permBits := fs.FileMode(0o777)
	if build.IsWindows {
		// Windows has no umask so we must chose a safer set of bits to
		// begin with.
		permBits = 0o700
	}

	// Upgrade code remains hardcoded for .stfolder despite configurable
	// marker name in later versions.

	for i := range cfg.Folders {
		fs := cfg.Folders[i].Filesystem(nil)
		// Invalid config posted, or tests.
		if fs == nil {
			continue
		}
		if stat, err := fs.Stat(DefaultMarkerName); err == nil && !stat.IsDir() {
			err = fs.Remove(DefaultMarkerName)
			if err == nil {
				err = fs.Mkdir(DefaultMarkerName, permBits)
				fs.Hide(DefaultMarkerName) // ignore error
			}
			if err != nil {
				l.Infoln("Failed to upgrade folder marker:", err)
			}
		}
	}
}

func migrateToConfigV22(cfg *Configuration) {
	for i := range cfg.Folders {
		cfg.Folders[i].FilesystemType = FilesystemTypeBasic
		// Migrate to templated external versioner commands
		if cfg.Folders[i].Versioning.Type == "external" {
			cfg.Folders[i].Versioning.Params["command"] += " %FOLDER_PATH% %FILE_PATH%"
		}
	}
}

func migrateToConfigV21(cfg *Configuration) {
	for _, folder := range cfg.Folders {
		if folder.FilesystemType != FilesystemTypeBasic {
			continue
		}
		switch folder.Versioning.Type {
		case "simple", "trashcan":
			// Clean out symlinks in the known place
			cleanSymlinks(folder.Filesystem(nil), ".stversions")
		case "staggered":
			versionDir := folder.Versioning.Params["versionsPath"]
			if versionDir == "" {
				// default place
				cleanSymlinks(folder.Filesystem(nil), ".stversions")
			} else if filepath.IsAbs(versionDir) {
				// absolute
				cleanSymlinks(fs.NewFilesystem(fs.FilesystemTypeBasic, versionDir), ".")
			} else {
				// relative to folder
				cleanSymlinks(folder.Filesystem(nil), versionDir)
			}
		}
	}
}

func migrateToConfigV20(cfg *Configuration) {
	cfg.Options.MinHomeDiskFree = Size{Value: cfg.Options.DeprecatedMinHomeDiskFreePct, Unit: "%"}
	cfg.Options.DeprecatedMinHomeDiskFreePct = 0

	for i := range cfg.Folders {
		cfg.Folders[i].MinDiskFree = Size{Value: cfg.Folders[i].DeprecatedMinDiskFreePct, Unit: "%"}
		cfg.Folders[i].DeprecatedMinDiskFreePct = 0
	}
}

func migrateToConfigV18(cfg *Configuration) {
	// Do channel selection for existing users. Those who have auto upgrades
	// and usage reporting on default to the candidate channel. Others get
	// stable.
	if cfg.Options.URAccepted > 0 && cfg.Options.AutoUpgradeEnabled() {
		cfg.Options.UpgradeToPreReleases = true
	}

	// Show a notification to explain what's going on, except if upgrades
	// are disabled by compilation or environment variable in which case
	// it's not relevant.
	if !upgrade.DisabledByCompilation && os.Getenv("STNOUPGRADE") == "" {
		cfg.Options.UnackedNotificationIDs = append(cfg.Options.UnackedNotificationIDs, "channelNotification")
	}
}

func migrateToConfigV15(cfg *Configuration) {
	// Undo v0.13.0 broken migration

	for i, addr := range cfg.Options.RawGlobalAnnServers {
		switch addr {
		case "default-v4v2/":
			cfg.Options.RawGlobalAnnServers[i] = "default-v4"
		case "default-v6v2/":
			cfg.Options.RawGlobalAnnServers[i] = "default-v6"
		}
	}
}

func migrateToConfigV14(cfg *Configuration) {
	// Not using the ignore cache is the new default. Disable it on existing
	// configurations.
	cfg.Options.CacheIgnoredFiles = false

	// Migrate UPnP -> NAT options
	cfg.Options.NATEnabled = cfg.Options.DeprecatedUPnPEnabled
	cfg.Options.DeprecatedUPnPEnabled = false
	cfg.Options.NATLeaseM = cfg.Options.DeprecatedUPnPLeaseM
	cfg.Options.DeprecatedUPnPLeaseM = 0
	cfg.Options.NATRenewalM = cfg.Options.DeprecatedUPnPRenewalM
	cfg.Options.DeprecatedUPnPRenewalM = 0
	cfg.Options.NATTimeoutS = cfg.Options.DeprecatedUPnPTimeoutS
	cfg.Options.DeprecatedUPnPTimeoutS = 0

	// Replace the default listen address "tcp://0.0.0.0:22000" with the
	// string "default", but only if we also have the default relay pool
	// among the relay servers as this is implied by the new "default"
	// entry.
	hasDefault := false
	for _, raddr := range cfg.Options.DeprecatedRelayServers {
		if raddr == "dynamic+https://relays.syncthing.net/endpoint" {
			for i, addr := range cfg.Options.RawListenAddresses {
				if addr == "tcp://0.0.0.0:22000" {
					cfg.Options.RawListenAddresses[i] = "default"
					hasDefault = true
					break
				}
			}
			break
		}
	}

	// Copy relay addresses into listen addresses.
	for _, addr := range cfg.Options.DeprecatedRelayServers {
		if hasDefault && addr == "dynamic+https://relays.syncthing.net/endpoint" {
			// Skip the default relay address if we already have the
			// "default" entry in the list.
			continue
		}
		if addr == "" {
			continue
		}
		cfg.Options.RawListenAddresses = append(cfg.Options.RawListenAddresses, addr)
	}

	cfg.Options.DeprecatedRelayServers = nil

	// For consistency
	sort.Strings(cfg.Options.RawListenAddresses)

	var newAddrs []string
	for _, addr := range cfg.Options.RawGlobalAnnServers {
		uri, err := url.Parse(addr)
		if err != nil {
			// That's odd. Skip the broken address.
			continue
		}
		if uri.Scheme == "https" {
			uri.Path = path.Join(uri.Path, "v2") + "/"
			addr = uri.String()
		}

		newAddrs = append(newAddrs, addr)
	}
	cfg.Options.RawGlobalAnnServers = newAddrs

	for i, fcfg := range cfg.Folders {
		if fcfg.DeprecatedReadOnly {
			cfg.Folders[i].Type = FolderTypeSendOnly
		} else {
			cfg.Folders[i].Type = FolderTypeSendReceive
		}
		cfg.Folders[i].DeprecatedReadOnly = false
	}
	// v0.13-beta already had config version 13 but did not get the new URL
	if cfg.Options.ReleasesURL == "https://api.github.com/repos/syncthing/syncthing/releases?per_page=30" {
		cfg.Options.ReleasesURL = "https://upgrades.syncthing.net/meta.json"
	}
}

func migrateToConfigV13(cfg *Configuration) {
	if cfg.Options.ReleasesURL == "https://api.github.com/repos/syncthing/syncthing/releases?per_page=30" {
		cfg.Options.ReleasesURL = "https://upgrades.syncthing.net/meta.json"
	}
}

func migrateToConfigV12(cfg *Configuration) {
	// Change listen address schema
	for i, addr := range cfg.Options.RawListenAddresses {
		if len(addr) > 0 && !strings.HasPrefix(addr, "tcp://") {
			cfg.Options.RawListenAddresses[i] = netutil.AddressURL("tcp", addr)
		}
	}

	for i, device := range cfg.Devices {
		for j, addr := range device.Addresses {
			if addr != "dynamic" && addr != "" {
				cfg.Devices[i].Addresses[j] = netutil.AddressURL("tcp", addr)
			}
		}
	}

	// Use new discovery server
	var newDiscoServers []string
	var useDefault bool
	for _, addr := range cfg.Options.RawGlobalAnnServers {
		if addr == "udp4://announce.syncthing.net:22026" {
			useDefault = true
		} else if addr == "udp6://announce-v6.syncthing.net:22026" {
			useDefault = true
		} else {
			newDiscoServers = append(newDiscoServers, addr)
		}
	}
	if useDefault {
		newDiscoServers = append(newDiscoServers, "default")
	}
	cfg.Options.RawGlobalAnnServers = newDiscoServers

	// Use new multicast group
	if cfg.Options.LocalAnnMCAddr == "[ff32::5222]:21026" {
		cfg.Options.LocalAnnMCAddr = "[ff12::8384]:21027"
	}

	// Use new local discovery port
	if cfg.Options.LocalAnnPort == 21025 {
		cfg.Options.LocalAnnPort = 21027
	}

	// Set MaxConflicts to unlimited
	for i := range cfg.Folders {
		cfg.Folders[i].MaxConflicts = -1
	}
}

func migrateToConfigV11(cfg *Configuration) {
	// Set minimum disk free of existing folders to 1%
	for i := range cfg.Folders {
		cfg.Folders[i].DeprecatedMinDiskFreePct = 1
	}
}
