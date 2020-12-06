// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// Package config implements reading and writing of the syncthing configuration file.
package config

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/url"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/pkg/errors"

	"github.com/syncthing/syncthing/lib/build"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/rand"
	"github.com/syncthing/syncthing/lib/util"
)

const (
	OldestHandledVersion = 10
	CurrentVersion       = 32
	MaxRescanIntervalS   = 365 * 24 * 60 * 60
)

var (
	// DefaultTCPPort defines default TCP port used if the URI does not specify one, for example tcp://0.0.0.0
	DefaultTCPPort = 22000
	// DefaultQUICPort defines default QUIC port used if the URI does not specify one, for example quic://0.0.0.0
	DefaultQUICPort = 22000
	// DefaultListenAddresses should be substituted when the configuration
	// contains <listenAddress>default</listenAddress>. This is done by the
	// "consumer" of the configuration as we don't want these saved to the
	// config.
	DefaultListenAddresses = []string{
		util.Address("tcp", net.JoinHostPort("0.0.0.0", strconv.Itoa(DefaultTCPPort))),
		"dynamic+https://relays.syncthing.net/endpoint",
		util.Address("quic", net.JoinHostPort("0.0.0.0", strconv.Itoa(DefaultQUICPort))),
	}
	DefaultGUIPort = 8384
	// DefaultDiscoveryServersV4 should be substituted when the configuration
	// contains <globalAnnounceServer>default-v4</globalAnnounceServer>.
	DefaultDiscoveryServersV4 = []string{
		"https://discovery.syncthing.net/v2/?noannounce&id=LYXKCHX-VI3NYZR-ALCJBHF-WMZYSPK-QG6QJA3-MPFYMSO-U56GTUK-NA2MIAW",
		"https://discovery-v4.syncthing.net/v2/?nolookup&id=LYXKCHX-VI3NYZR-ALCJBHF-WMZYSPK-QG6QJA3-MPFYMSO-U56GTUK-NA2MIAW",
	}
	// DefaultDiscoveryServersV6 should be substituted when the configuration
	// contains <globalAnnounceServer>default-v6</globalAnnounceServer>.
	DefaultDiscoveryServersV6 = []string{
		"https://discovery.syncthing.net/v2/?noannounce&id=LYXKCHX-VI3NYZR-ALCJBHF-WMZYSPK-QG6QJA3-MPFYMSO-U56GTUK-NA2MIAW",
		"https://discovery-v6.syncthing.net/v2/?nolookup&id=LYXKCHX-VI3NYZR-ALCJBHF-WMZYSPK-QG6QJA3-MPFYMSO-U56GTUK-NA2MIAW",
	}
	// DefaultDiscoveryServers should be substituted when the configuration
	// contains <globalAnnounceServer>default</globalAnnounceServer>.
	DefaultDiscoveryServers = append(DefaultDiscoveryServersV4, DefaultDiscoveryServersV6...)
	// DefaultTheme is the default and fallback theme for the web UI.
	DefaultTheme = "default"
	// Default stun servers should be substituted when the configuration
	// contains <stunServer>default</stunServer>.

	// DefaultPrimaryStunServers are servers provided by us (to avoid causing the public servers burden)
	DefaultPrimaryStunServers = []string{
		"stun.syncthing.net:3478",
	}
	DefaultSecondaryStunServers = []string{
		"stun.callwithus.com:3478",
		"stun.counterpath.com:3478",
		"stun.counterpath.net:3478",
		"stun.ekiga.net:3478",
		"stun.ideasip.com:3478",
		"stun.internetcalls.com:3478",
		"stun.schlund.de:3478",
		"stun.sipgate.net:10000",
		"stun.sipgate.net:3478",
		"stun.voip.aebc.com:3478",
		"stun.voiparound.com:3478",
		"stun.voipbuster.com:3478",
		"stun.voipstunt.com:3478",
		"stun.xten.com:3478",
	}
)

var (
	errFolderIDEmpty     = errors.New("folder has empty ID")
	errFolderIDDuplicate = errors.New("folder has duplicate ID")
	errFolderPathEmpty   = errors.New("folder has empty path")
)

func New(myID protocol.DeviceID) Configuration {
	var cfg Configuration
	cfg.Version = CurrentVersion

	if !build.IsIOS() {
		// FIXME
		cfg.Options.UnackedNotificationIDs = []string{"authenticationUserAndPassword"}
	}

	util.SetDefaults(&cfg)
	util.SetDefaults(&cfg.Options)
	util.SetDefaults(&cfg.GUI)

	if build.IsIOS() {
		cfg.Options.URSeen = 999999 // maxint so we never send usage reports on iOS
		cfg.Options.DefaultFolderPath = "."
		// FIXME Find better solution than blank user and password, but suppress this notification for now
	} else {
		cfg.Options.UnackedNotificationIDs = []string{"authenticationUserAndPassword"}
	}

	// Can't happen.
	if err := cfg.prepare(myID); err != nil {
		l.Warnln("bug: error in preparing new folder:", err)
		panic("error in preparing new folder")
	}

	return cfg
}

func NewWithFreePorts(myID protocol.DeviceID) (Configuration, error) {
	cfg := New(myID)

	port, err := getFreePort("127.0.0.1", DefaultGUIPort)
	if err != nil {
		return Configuration{}, errors.Wrap(err, "get free port (GUI)")
	}
	cfg.GUI.RawAddress = fmt.Sprintf("127.0.0.1:%d", port)

	port, err = getFreePort("0.0.0.0", DefaultTCPPort)
	if err != nil {
		return Configuration{}, errors.Wrap(err, "get free port (BEP)")
	}
	if port == DefaultTCPPort {
		cfg.Options.RawListenAddresses = []string{"default"}
	} else {
		cfg.Options.RawListenAddresses = []string{
			util.Address("tcp", net.JoinHostPort("0.0.0.0", strconv.Itoa(port))),
			"dynamic+https://relays.syncthing.net/endpoint",
			util.Address("quic", net.JoinHostPort("0.0.0.0", strconv.Itoa(port))),
		}
	}

	return cfg, nil
}

type xmlConfiguration struct {
	Configuration
	XMLName xml.Name `xml:"configuration"`
}

func ReadXML(r io.Reader, myID protocol.DeviceID) (Configuration, int, error) {
	var cfg xmlConfiguration

	util.SetDefaults(&cfg)
	util.SetDefaults(&cfg.Options)
	util.SetDefaults(&cfg.GUI)

	if err := xml.NewDecoder(r).Decode(&cfg); err != nil {
		return Configuration{}, 0, err
	}

	originalVersion := cfg.Version

	if err := cfg.prepare(myID); err != nil {
		return Configuration{}, originalVersion, err
	}
	return cfg.Configuration, originalVersion, nil
}

func ReadJSON(r io.Reader, myID protocol.DeviceID) (Configuration, error) {
	var cfg Configuration

	util.SetDefaults(&cfg)
	util.SetDefaults(&cfg.Options)
	util.SetDefaults(&cfg.GUI)

	bs, err := ioutil.ReadAll(r)
	if err != nil {
		return Configuration{}, err
	}

	if err := json.Unmarshal(bs, &cfg); err != nil {
		return Configuration{}, err
	}

	if err := cfg.prepare(myID); err != nil {
		return Configuration{}, err
	}
	return cfg, nil
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
	newCfg.GUI = cfg.GUI.Copy()

	// DeviceIDs are values
	newCfg.IgnoredDevices = make([]ObservedDevice, len(cfg.IgnoredDevices))
	copy(newCfg.IgnoredDevices, cfg.IgnoredDevices)

	newCfg.PendingDevices = make([]ObservedDevice, len(cfg.PendingDevices))
	copy(newCfg.PendingDevices, cfg.PendingDevices)

	return newCfg
}

func (cfg *Configuration) WriteXML(w io.Writer) error {
	e := xml.NewEncoder(w)
	e.Indent("", "    ")
	xmlCfg := xmlConfiguration{Configuration: *cfg}
	err := e.Encode(xmlCfg)
	if err != nil {
		return err
	}
	_, err = w.Write([]byte("\n"))
	return err
}

func (cfg *Configuration) prepare(myID protocol.DeviceID) error {
	var myName string

	// Ensure this device is present in the config
	for _, device := range cfg.Devices {
		if device.DeviceID == myID {
			goto found
		}
	}

	myName, _ = os.Hostname()
	cfg.Devices = append(cfg.Devices, DeviceConfiguration{
		DeviceID: myID,
		Name:     myName,
	})

found:

	if err := cfg.clean(); err != nil {
		return err
	}

	// Ensure that we are part of the devices
	for i := range cfg.Folders {
		cfg.Folders[i].Devices = ensureDevicePresent(cfg.Folders[i].Devices, myID)
	}

	return nil
}

func (cfg *Configuration) clean() error {
	util.FillNilSlices(&cfg.Options)

	// Ensure that the device list is
	// - free from duplicates
	// - no devices with empty ID
	// - sorted by ID
	// Happen before preparting folders as that needs a correct device list.
	cfg.Devices = ensureNoDuplicateOrEmptyIDDevices(cfg.Devices)
	sort.Slice(cfg.Devices, func(a, b int) bool {
		return cfg.Devices[a].DeviceID.Compare(cfg.Devices[b].DeviceID) == -1
	})

	// Prepare folders and check for duplicates. Duplicates are bad and
	// dangerous, can't currently be resolved in the GUI, and shouldn't
	// happen when configured by the GUI. We return with an error in that
	// situation.
	existingFolders := make(map[string]*FolderConfiguration)
	for i := range cfg.Folders {
		folder := &cfg.Folders[i]
		folder.prepare()

		if folder.ID == "" {
			return errFolderIDEmpty
		}

		if folder.Path == "" {
			return fmt.Errorf("folder %q: %w", folder.ID, errFolderPathEmpty)
		}

		if _, ok := existingFolders[folder.ID]; ok {
			return fmt.Errorf("folder %q: %w", folder.ID, errFolderIDDuplicate)
		}

		existingFolders[folder.ID] = folder
	}

	cfg.Options.RawListenAddresses = util.UniqueTrimmedStrings(cfg.Options.RawListenAddresses)
	cfg.Options.RawGlobalAnnServers = util.UniqueTrimmedStrings(cfg.Options.RawGlobalAnnServers)

	if cfg.Version > 0 && cfg.Version < OldestHandledVersion {
		l.Warnf("Configuration version %d is deprecated. Attempting best effort conversion, but please verify manually.", cfg.Version)
	}

	// Upgrade configuration versions as appropriate
	migrationsMut.Lock()
	migrations.apply(cfg)
	migrationsMut.Unlock()

	// Build a list of available devices
	existingDevices := make(map[protocol.DeviceID]bool)
	for _, device := range cfg.Devices {
		existingDevices[device.DeviceID] = true
	}

	// Ensure that the folder list is sorted by ID
	sort.Slice(cfg.Folders, func(a, b int) bool {
		return cfg.Folders[a].ID < cfg.Folders[b].ID
	})

	// Ensure that in all folder configs
	// - any loose devices are not present in the wrong places
	// - there are no duplicate devices
	// - the versioning configuration parameter map is not nil
	sharedFolders := make(map[protocol.DeviceID][]string, len(cfg.Devices))
	for i := range cfg.Folders {
		cfg.Folders[i].Devices = ensureExistingDevices(cfg.Folders[i].Devices, existingDevices)
		cfg.Folders[i].Devices = ensureNoDuplicateFolderDevices(cfg.Folders[i].Devices)
		if cfg.Folders[i].Versioning.Params == nil {
			cfg.Folders[i].Versioning.Params = map[string]string{}
		}
		sort.Slice(cfg.Folders[i].Devices, func(a, b int) bool {
			return cfg.Folders[i].Devices[a].DeviceID.Compare(cfg.Folders[i].Devices[b].DeviceID) == -1
		})
		for _, dev := range cfg.Folders[i].Devices {
			sharedFolders[dev.DeviceID] = append(sharedFolders[dev.DeviceID], cfg.Folders[i].ID)
		}
	}

	for i := range cfg.Devices {
		cfg.Devices[i].prepare(sharedFolders[cfg.Devices[i].DeviceID])
	}

	// Very short reconnection intervals are annoying
	if cfg.Options.ReconnectIntervalS < 5 {
		cfg.Options.ReconnectIntervalS = 5
	}

	if cfg.GUI.APIKey == "" {
		cfg.GUI.APIKey = rand.String(32)
	}

	// The list of ignored devices should not contain any devices that have
	// been manually added to the config.
	var newIgnoredDevices []ObservedDevice
	ignoredDevices := make(map[protocol.DeviceID]bool)
	for _, dev := range cfg.IgnoredDevices {
		if !existingDevices[dev.ID] {
			ignoredDevices[dev.ID] = true
			newIgnoredDevices = append(newIgnoredDevices, dev)
		}
	}
	cfg.IgnoredDevices = newIgnoredDevices

	// The list of pending devices should not contain devices that were added manually, nor should it contain
	// ignored devices.

	// Sort by time, so that in case of duplicates latest "time" is used.
	sort.Slice(cfg.PendingDevices, func(i, j int) bool {
		return cfg.PendingDevices[i].Time.Before(cfg.PendingDevices[j].Time)
	})

	var newPendingDevices []ObservedDevice
nextPendingDevice:
	for _, pendingDevice := range cfg.PendingDevices {
		if !existingDevices[pendingDevice.ID] && !ignoredDevices[pendingDevice.ID] {
			// Deduplicate
			for _, existingPendingDevice := range newPendingDevices {
				if existingPendingDevice.ID == pendingDevice.ID {
					continue nextPendingDevice
				}
			}
			newPendingDevices = append(newPendingDevices, pendingDevice)
		}
	}
	cfg.PendingDevices = newPendingDevices

	// Deprecated protocols are removed from the list of listeners and
	// device addresses. So far just kcp*.
	for _, prefix := range []string{"kcp"} {
		cfg.Options.RawListenAddresses = filterURLSchemePrefix(cfg.Options.RawListenAddresses, prefix)
		for i := range cfg.Devices {
			dev := &cfg.Devices[i]
			dev.Addresses = filterURLSchemePrefix(dev.Addresses, prefix)
		}
	}

	// Initialize any empty slices
	if cfg.Folders == nil {
		cfg.Folders = []FolderConfiguration{}
	}
	if cfg.IgnoredDevices == nil {
		cfg.IgnoredDevices = []ObservedDevice{}
	}
	if cfg.PendingDevices == nil {
		cfg.PendingDevices = []ObservedDevice{}
	}
	if cfg.Options.AlwaysLocalNets == nil {
		cfg.Options.AlwaysLocalNets = []string{}
	}
	if cfg.Options.UnackedNotificationIDs == nil {
		cfg.Options.UnackedNotificationIDs = []string{}
	} else if cfg.GUI.User != "" && cfg.GUI.Password != "" {
		for i, key := range cfg.Options.UnackedNotificationIDs {
			if key == "authenticationUserAndPassword" {
				cfg.Options.UnackedNotificationIDs = append(cfg.Options.UnackedNotificationIDs[:i], cfg.Options.UnackedNotificationIDs[i+1:]...)
				break
			}
		}
	}
	if cfg.Options.FeatureFlags == nil {
		cfg.Options.FeatureFlags = []string{}
	}

	return nil
}

// DeviceMap returns a map of device ID to device configuration for the given configuration.
func (cfg *Configuration) DeviceMap() map[protocol.DeviceID]DeviceConfiguration {
	m := make(map[protocol.DeviceID]DeviceConfiguration, len(cfg.Devices))
	for _, dev := range cfg.Devices {
		m[dev.DeviceID] = dev
	}
	return m
}

// FolderPasswords returns the folder passwords set for this device, for
// folders that have an encryption password set.
func (cfg Configuration) FolderPasswords(device protocol.DeviceID) map[string]string {
	res := make(map[string]string, len(cfg.Folders))
nextFolder:
	for _, folder := range cfg.Folders {
		for _, dev := range folder.Devices {
			if dev.DeviceID == device && dev.EncryptionPassword != "" {
				res[folder.ID] = dev.EncryptionPassword
				continue nextFolder
			}
		}
	}
	return res
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

func ensureNoDuplicateOrEmptyIDDevices(devices []DeviceConfiguration) []DeviceConfiguration {
	count := len(devices)
	i := 0
	seenDevices := make(map[protocol.DeviceID]bool)
loop:
	for i < count {
		id := devices[i].DeviceID
		if _, ok := seenDevices[id]; ok || id == protocol.EmptyDeviceID {
			devices[i] = devices[count-1]
			count--
			continue loop
		}
		seenDevices[id] = true
		i++
	}
	return devices[0:count]
}

func cleanSymlinks(filesystem fs.Filesystem, dir string) {
	if runtime.GOOS == "windows" {
		// We don't do symlinks on Windows. Additionally, there may
		// be things that look like symlinks that are not, which we
		// should leave alone. Deduplicated files, for example.
		return
	}
	filesystem.Walk(dir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsSymlink() {
			l.Infoln("Removing incorrectly versioned symlink", path)
			filesystem.Remove(path)
			return fs.SkipDir
		}
		return nil
	})
}

// filterURLSchemePrefix returns the list of addresses after removing all
// entries whose URL scheme matches the given prefix.
func filterURLSchemePrefix(addrs []string, prefix string) []string {
	for i := 0; i < len(addrs); i++ {
		uri, err := url.Parse(addrs[i])
		if err != nil {
			continue
		}
		if strings.HasPrefix(uri.Scheme, prefix) {
			// Remove this entry
			copy(addrs[i:], addrs[i+1:])
			addrs = addrs[:len(addrs)-1]
			i--
		}
	}
	return addrs
}

// tried in succession and the first to succeed is returned. If none succeed,
// a random high port is returned.
func getFreePort(host string, ports ...int) (int, error) {
	for _, port := range ports {
		c, err := net.Listen("tcp", fmt.Sprintf("%s:%d", host, port))
		if err == nil {
			c.Close()
			return port, nil
		}
	}

	c, err := net.Listen("tcp", host+":0")
	if err != nil {
		return 0, err
	}
	addr := c.Addr().(*net.TCPAddr)
	c.Close()
	return addr.Port, nil
}
