// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// Package config implements reading and writing of the syncthing configuration file.
package config

import (
	"encoding/json"
	"encoding/xml"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"path"
	"sort"
	"strings"

	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/rand"
	"github.com/syncthing/syncthing/lib/util"
)

const (
	OldestHandledVersion = 10
	CurrentVersion       = 15
	MaxRescanIntervalS   = 365 * 24 * 60 * 60
)

var (
	// DefaultListenAddresses should be substituted when the configuration
	// contains <listenAddress>default</listenAddress>. This is done by the
	// "consumer" of the configuration as we don't want these saved to the
	// config.
	DefaultListenAddresses = []string{
		"tcp://0.0.0.0:22000",
		"dynamic+https://relays.syncthing.net/endpoint",
	}
	// DefaultDiscoveryServersV4 should be substituted when the configuration
	// contains <globalAnnounceServer>default-v4</globalAnnounceServer>.
	DefaultDiscoveryServersV4 = []string{
		"https://discovery-v4-1.syncthing.net/v2/?id=SR7AARM-TCBUZ5O-VFAXY4D-CECGSDE-3Q6IZ4G-XG7AH75-OBIXJQV-QJ6NLQA", // 194.126.249.5, Sweden
		"https://discovery-v4-2.syncthing.net/v2/?id=DVU36WY-H3LVZHW-E6LLFRE-YAFN5EL-HILWRYP-OC2M47J-Z4PE62Y-ADIBDQC", // 45.55.230.38, USA
		"https://discovery-v4-3.syncthing.net/v2/?id=VK6HNJ3-VVMM66S-HRVWSCR-IXEHL2H-U4AQ4MW-UCPQBWX-J2L2UBK-NVZRDQZ", // 128.199.95.124, Singapore
	}
	// DefaultDiscoveryServersV6 should be substituted when the configuration
	// contains <globalAnnounceServer>default-v6</globalAnnounceServer>.
	DefaultDiscoveryServersV6 = []string{
		"https://discovery-v6-1.syncthing.net/v2/?id=SR7AARM-TCBUZ5O-VFAXY4D-CECGSDE-3Q6IZ4G-XG7AH75-OBIXJQV-QJ6NLQA", // 2001:470:28:4d6::5, Sweden
		"https://discovery-v6-2.syncthing.net/v2/?id=DVU36WY-H3LVZHW-E6LLFRE-YAFN5EL-HILWRYP-OC2M47J-Z4PE62Y-ADIBDQC", // 2604:a880:800:10::182:a001, USA
		"https://discovery-v6-3.syncthing.net/v2/?id=VK6HNJ3-VVMM66S-HRVWSCR-IXEHL2H-U4AQ4MW-UCPQBWX-J2L2UBK-NVZRDQZ", // 2400:6180:0:d0::d9:d001, Singapore
	}
	// DefaultDiscoveryServers should be substituted when the configuration
	// contains <globalAnnounceServer>default</globalAnnounceServer>.
	DefaultDiscoveryServers = append(DefaultDiscoveryServersV4, DefaultDiscoveryServersV6...)

	// DefaultTheme is the default and fallback theme for the web UI.
	DefaultTheme = "default"
)

func New(myID protocol.DeviceID) Configuration {
	var cfg Configuration
	cfg.Version = CurrentVersion
	cfg.OriginalVersion = CurrentVersion

	util.SetDefaults(&cfg)
	util.SetDefaults(&cfg.Options)
	util.SetDefaults(&cfg.GUI)

	cfg.prepare(myID)

	return cfg
}

func ReadXML(r io.Reader, myID protocol.DeviceID) (Configuration, error) {
	var cfg Configuration

	util.SetDefaults(&cfg)
	util.SetDefaults(&cfg.Options)
	util.SetDefaults(&cfg.GUI)

	err := xml.NewDecoder(r).Decode(&cfg)
	cfg.OriginalVersion = cfg.Version

	cfg.prepare(myID)
	return cfg, err
}

func ReadJSON(r io.Reader, myID protocol.DeviceID) (Configuration, error) {
	var cfg Configuration

	util.SetDefaults(&cfg)
	util.SetDefaults(&cfg.Options)
	util.SetDefaults(&cfg.GUI)

	bs, err := ioutil.ReadAll(r)
	if err != nil {
		return cfg, err
	}

	err = json.Unmarshal(bs, &cfg)
	cfg.OriginalVersion = cfg.Version

	cfg.prepare(myID)
	return cfg, err
}

type Configuration struct {
	Version        int                   `xml:"version,attr" json:"version"`
	Folders        []FolderConfiguration `xml:"folder" json:"folders"`
	Devices        []DeviceConfiguration `xml:"device" json:"devices"`
	GUI            GUIConfiguration      `xml:"gui" json:"gui"`
	Options        OptionsConfiguration  `xml:"options" json:"options"`
	IgnoredDevices []protocol.DeviceID   `xml:"ignoredDevice" json:"ignoredDevices"`
	XMLName        xml.Name              `xml:"configuration" json:"-"`

	OriginalVersion int `xml:"-" json:"-"` // The version we read from disk, before any conversion
}

func (cfg Configuration) Copy() Configuration {
	newCfg := cfg

	// Deep copy FolderConfigurations
	newCfg.Folders = make([]FolderConfiguration, len(cfg.Folders))
	for i := range newCfg.Folders {
		newCfg.Folders[i] = cfg.Folders[i].Copy()
	}

	// Deep copy DeviceConfigurations
	newCfg.Devices = make([]DeviceConfiguration, len(cfg.Devices))
	for i := range newCfg.Devices {
		newCfg.Devices[i] = cfg.Devices[i].Copy()
	}

	newCfg.Options = cfg.Options.Copy()

	// DeviceIDs are values
	newCfg.IgnoredDevices = make([]protocol.DeviceID, len(cfg.IgnoredDevices))
	copy(newCfg.IgnoredDevices, cfg.IgnoredDevices)

	return newCfg
}

func (cfg *Configuration) WriteXML(w io.Writer) error {
	e := xml.NewEncoder(w)
	e.Indent("", "    ")
	err := e.Encode(cfg)
	if err != nil {
		return err
	}
	_, err = w.Write([]byte("\n"))
	return err
}

func (cfg *Configuration) prepare(myID protocol.DeviceID) {
	util.FillNilSlices(&cfg.Options)

	// Initialize any empty slices
	if cfg.Folders == nil {
		cfg.Folders = []FolderConfiguration{}
	}
	if cfg.IgnoredDevices == nil {
		cfg.IgnoredDevices = []protocol.DeviceID{}
	}
	if cfg.Options.AlwaysLocalNets == nil {
		cfg.Options.AlwaysLocalNets = []string{}
	}

	// Check for missing, bad or duplicate folder ID:s
	var seenFolders = map[string]*FolderConfiguration{}
	for i := range cfg.Folders {
		folder := &cfg.Folders[i]
		folder.prepare()

		if seen, ok := seenFolders[folder.ID]; ok {
			l.Warnf("Multiple folders with ID %q; disabling", folder.ID)
			seen.Invalid = "duplicate folder ID"
			folder.Invalid = "duplicate folder ID"
		} else {
			seenFolders[folder.ID] = folder
		}
	}

	cfg.Options.ListenAddresses = util.UniqueStrings(cfg.Options.ListenAddresses)
	cfg.Options.GlobalAnnServers = util.UniqueStrings(cfg.Options.GlobalAnnServers)

	if cfg.Version > 0 && cfg.Version < OldestHandledVersion {
		l.Warnf("Configuration version %d is deprecated. Attempting best effort conversion, but please verify manually.", cfg.Version)
	}

	// Upgrade configuration versions as appropriate
	if cfg.Version <= 10 {
		convertV10V11(cfg)
	}
	if cfg.Version == 11 {
		convertV11V12(cfg)
	}
	if cfg.Version == 12 {
		convertV12V13(cfg)
	}
	if cfg.Version == 13 {
		convertV13V14(cfg)
	}
	if cfg.Version == 14 {
		convertV14V15(cfg)
	}

	// Build a list of available devices
	existingDevices := make(map[protocol.DeviceID]bool)
	for _, device := range cfg.Devices {
		existingDevices[device.DeviceID] = true
	}

	// Ensure this device is present in the config
	if !existingDevices[myID] {
		myName, _ := os.Hostname()
		cfg.Devices = append(cfg.Devices, DeviceConfiguration{
			DeviceID: myID,
			Name:     myName,
		})
		existingDevices[myID] = true
	}

	// Ensure that the device list is free from duplicates
	cfg.Devices = ensureNoDuplicateDevices(cfg.Devices)

	sort.Sort(DeviceConfigurationList(cfg.Devices))
	// Ensure that any loose devices are not present in the wrong places
	// Ensure that there are no duplicate devices
	// Ensure that puller settings are sane
	// Ensure that the versioning configuration parameter map is not nil
	for i := range cfg.Folders {
		cfg.Folders[i].Devices = ensureDevicePresent(cfg.Folders[i].Devices, myID)
		cfg.Folders[i].Devices = ensureExistingDevices(cfg.Folders[i].Devices, existingDevices)
		cfg.Folders[i].Devices = ensureNoDuplicateFolderDevices(cfg.Folders[i].Devices)
		if cfg.Folders[i].Versioning.Params == nil {
			cfg.Folders[i].Versioning.Params = map[string]string{}
		}
		sort.Sort(FolderDeviceConfigurationList(cfg.Folders[i].Devices))
	}

	// An empty address list is equivalent to a single "dynamic" entry
	for i := range cfg.Devices {
		n := &cfg.Devices[i]
		if len(n.Addresses) == 0 || len(n.Addresses) == 1 && n.Addresses[0] == "" {
			n.Addresses = []string{"dynamic"}
		}
	}

	// Very short reconnection intervals are annoying
	if cfg.Options.ReconnectIntervalS < 5 {
		cfg.Options.ReconnectIntervalS = 5
	}

	if cfg.GUI.APIKey == "" {
		cfg.GUI.APIKey = rand.String(32)
	}
}

func convertV14V15(cfg *Configuration) {
	// Undo v0.13.0 broken migration

	for i, addr := range cfg.Options.GlobalAnnServers {
		switch addr {
		case "default-v4v2/":
			cfg.Options.GlobalAnnServers[i] = "default-v4"
		case "default-v6v2/":
			cfg.Options.GlobalAnnServers[i] = "default-v6"
		}
	}

	cfg.Version = 15
}

func convertV13V14(cfg *Configuration) {
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
			for i, addr := range cfg.Options.ListenAddresses {
				if addr == "tcp://0.0.0.0:22000" {
					cfg.Options.ListenAddresses[i] = "default"
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
		cfg.Options.ListenAddresses = append(cfg.Options.ListenAddresses, addr)
	}

	cfg.Options.DeprecatedRelayServers = nil

	// For consistency
	sort.Strings(cfg.Options.ListenAddresses)

	var newAddrs []string
	for _, addr := range cfg.Options.GlobalAnnServers {
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
	cfg.Options.GlobalAnnServers = newAddrs

	for i, fcfg := range cfg.Folders {
		if fcfg.DeprecatedReadOnly {
			cfg.Folders[i].Type = FolderTypeReadOnly
		} else {
			cfg.Folders[i].Type = FolderTypeReadWrite
		}
		cfg.Folders[i].DeprecatedReadOnly = false
	}
	// v0.13-beta already had config version 13 but did not get the new URL
	if cfg.Options.ReleasesURL == "https://api.github.com/repos/syncthing/syncthing/releases?per_page=30" {
		cfg.Options.ReleasesURL = "https://upgrades.syncthing.net/meta.json"
	}

	cfg.Version = 14
}

func convertV12V13(cfg *Configuration) {
	if cfg.Options.ReleasesURL == "https://api.github.com/repos/syncthing/syncthing/releases?per_page=30" {
		cfg.Options.ReleasesURL = "https://upgrades.syncthing.net/meta.json"
	}

	cfg.Version = 13
}

func convertV11V12(cfg *Configuration) {
	// Change listen address schema
	for i, addr := range cfg.Options.ListenAddresses {
		if len(addr) > 0 && !strings.HasPrefix(addr, "tcp://") {
			cfg.Options.ListenAddresses[i] = util.Address("tcp", addr)
		}
	}

	for i, device := range cfg.Devices {
		for j, addr := range device.Addresses {
			if addr != "dynamic" && addr != "" {
				cfg.Devices[i].Addresses[j] = util.Address("tcp", addr)
			}
		}
	}

	// Use new discovery server
	var newDiscoServers []string
	var useDefault bool
	for _, addr := range cfg.Options.GlobalAnnServers {
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
	cfg.Options.GlobalAnnServers = newDiscoServers

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

	cfg.Version = 12
}

func convertV10V11(cfg *Configuration) {
	// Set minimum disk free of existing folders to 1%
	for i := range cfg.Folders {
		cfg.Folders[i].MinDiskFreePct = 1
	}
	cfg.Version = 11
}

func ensureDevicePresent(devices []FolderDeviceConfiguration, myID protocol.DeviceID) []FolderDeviceConfiguration {
	for _, device := range devices {
		if device.DeviceID.Equals(myID) {
			return devices
		}
	}

	devices = append(devices, FolderDeviceConfiguration{
		DeviceID: myID,
	})

	return devices
}

func ensureExistingDevices(devices []FolderDeviceConfiguration, existingDevices map[protocol.DeviceID]bool) []FolderDeviceConfiguration {
	count := len(devices)
	i := 0
loop:
	for i < count {
		if _, ok := existingDevices[devices[i].DeviceID]; !ok {
			devices[i] = devices[count-1]
			count--
			continue loop
		}
		i++
	}
	return devices[0:count]
}

func ensureNoDuplicateFolderDevices(devices []FolderDeviceConfiguration) []FolderDeviceConfiguration {
	count := len(devices)
	i := 0
	seenDevices := make(map[protocol.DeviceID]bool)
loop:
	for i < count {
		id := devices[i].DeviceID
		if _, ok := seenDevices[id]; ok {
			devices[i] = devices[count-1]
			count--
			continue loop
		}
		seenDevices[id] = true
		i++
	}
	return devices[0:count]
}

func ensureNoDuplicateDevices(devices []DeviceConfiguration) []DeviceConfiguration {
	count := len(devices)
	i := 0
	seenDevices := make(map[protocol.DeviceID]bool)
loop:
	for i < count {
		id := devices[i].DeviceID
		if _, ok := seenDevices[id]; ok {
			devices[i] = devices[count-1]
			count--
			continue loop
		}
		seenDevices[id] = true
		i++
	}
	return devices[0:count]
}
