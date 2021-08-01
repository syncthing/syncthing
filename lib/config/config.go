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

	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/util"
)

const (
	OldestHandledVersion = 10
	CurrentVersion       = 35
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

	cfg.Options.UnackedNotificationIDs = []string{"authenticationUserAndPassword"}

	util.SetDefaults(&cfg)

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
	bs, err := ioutil.ReadAll(r)
	if err != nil {
		return Configuration{}, err
	}

	var cfg Configuration

	util.SetDefaults(&cfg)

	if err := json.Unmarshal(bs, &cfg); err != nil {
		return Configuration{}, err
	}

	// Unmarshal list of devices and folders separately to set defaults
	var rawFoldersDevices struct {
		Folders []json.RawMessage
		Devices []json.RawMessage
	}
	if err := json.Unmarshal(bs, &rawFoldersDevices); err != nil {
		return Configuration{}, err
	}

	cfg.Folders = make([]FolderConfiguration, len(rawFoldersDevices.Folders))
	for i, bs := range rawFoldersDevices.Folders {
		cfg.Folders[i] = cfg.Defaults.Folder.Copy()
		if err := json.Unmarshal(bs, &cfg.Folders[i]); err != nil {
			return Configuration{}, err
		}
	}

	cfg.Devices = make([]DeviceConfiguration, len(rawFoldersDevices.Devices))
	for i, bs := range rawFoldersDevices.Devices {
		cfg.Devices[i] = cfg.Defaults.Device.Copy()
		if err := json.Unmarshal(bs, &cfg.Devices[i]); err != nil {
			return Configuration{}, err
		}
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
	cfg.ensureMyDevice(myID)

	existingDevices, err := cfg.prepareFoldersAndDevices(myID)
	if err != nil {
		return err
	}

	cfg.GUI.prepare()

	guiPWIsSet := cfg.GUI.User != "" && cfg.GUI.Password != ""
	cfg.Options.prepare(guiPWIsSet)

	cfg.prepareIgnoredDevices(existingDevices)

	cfg.Defaults.prepare(myID, existingDevices)

	cfg.removeDeprecatedProtocols()

	util.FillNilExceptDeprecated(cfg)

	// TestIssue1750 relies on migrations happening after preparing options.
	cfg.applyMigrations()

	return nil
}

func (cfg *Configuration) ensureMyDevice(myID protocol.DeviceID) {
	for _, device := range cfg.Devices {
		if device.DeviceID == myID {
			return
		}
	}

	myName, _ := os.Hostname()
	cfg.Devices = append(cfg.Devices, DeviceConfiguration{
		DeviceID: myID,
		Name:     myName,
	})
}

func (cfg *Configuration) prepareFoldersAndDevices(myID protocol.DeviceID) (map[protocol.DeviceID]bool, error) {
	existingDevices := cfg.prepareDeviceList()

	sharedFolders, err := cfg.prepareFolders(myID, existingDevices)
	if err != nil {
		return nil, err
	}

	cfg.prepareDevices(sharedFolders)

	return existingDevices, nil
}

func (cfg *Configuration) prepareDeviceList() map[protocol.DeviceID]bool {
	// Ensure that the device list is
	// - free from duplicates
	// - no devices with empty ID
	// - sorted by ID
	// Happen before preparting folders as that needs a correct device list.
	cfg.Devices = ensureNoDuplicateOrEmptyIDDevices(cfg.Devices)
	sort.Slice(cfg.Devices, func(a, b int) bool {
		return cfg.Devices[a].DeviceID.Compare(cfg.Devices[b].DeviceID) == -1
	})

	// Build a list of available devices
	existingDevices := make(map[protocol.DeviceID]bool, len(cfg.Devices))
	for _, device := range cfg.Devices {
		existingDevices[device.DeviceID] = true
	}
	return existingDevices
}

func (cfg *Configuration) prepareFolders(myID protocol.DeviceID, existingDevices map[protocol.DeviceID]bool) (map[protocol.DeviceID][]string, error) {
	// Prepare folders and check for duplicates. Duplicates are bad and
	// dangerous, can't currently be resolved in the GUI, and shouldn't
	// happen when configured by the GUI. We return with an error in that
	// situation.
	sharedFolders := make(map[protocol.DeviceID][]string, len(cfg.Devices))
	existingFolders := make(map[string]*FolderConfiguration, len(cfg.Folders))
	for i := range cfg.Folders {
		folder := &cfg.Folders[i]

		if folder.ID == "" {
			return nil, errFolderIDEmpty
		}

		if folder.Path == "" {
			return nil, fmt.Errorf("folder %q: %w", folder.ID, errFolderPathEmpty)
		}

		if _, ok := existingFolders[folder.ID]; ok {
			return nil, fmt.Errorf("folder %q: %w", folder.ID, errFolderIDDuplicate)
		}

		folder.prepare(myID, existingDevices)

		existingFolders[folder.ID] = folder

		for _, dev := range folder.Devices {
			sharedFolders[dev.DeviceID] = append(sharedFolders[dev.DeviceID], folder.ID)
		}
	}
	// Ensure that the folder list is sorted by ID
	sort.Slice(cfg.Folders, func(a, b int) bool {
		return cfg.Folders[a].ID < cfg.Folders[b].ID
	})
	return sharedFolders, nil
}

func (cfg *Configuration) prepareDevices(sharedFolders map[protocol.DeviceID][]string) {
	for i := range cfg.Devices {
		cfg.Devices[i].prepare(sharedFolders[cfg.Devices[i].DeviceID])
	}
}

func (cfg *Configuration) prepareIgnoredDevices(existingDevices map[protocol.DeviceID]bool) map[protocol.DeviceID]bool {
	// The list of ignored devices should not contain any devices that have
	// been manually added to the config.
	newIgnoredDevices := cfg.IgnoredDevices[:0]
	ignoredDevices := make(map[protocol.DeviceID]bool, len(cfg.IgnoredDevices))
	for _, dev := range cfg.IgnoredDevices {
		if !existingDevices[dev.ID] {
			ignoredDevices[dev.ID] = true
			newIgnoredDevices = append(newIgnoredDevices, dev)
		}
	}
	cfg.IgnoredDevices = newIgnoredDevices
	return ignoredDevices
}

func (cfg *Configuration) removeDeprecatedProtocols() {
	// Deprecated protocols are removed from the list of listeners and
	// device addresses. So far just kcp*.
	for _, prefix := range []string{"kcp"} {
		cfg.Options.RawListenAddresses = filterURLSchemePrefix(cfg.Options.RawListenAddresses, prefix)
		for i := range cfg.Devices {
			dev := &cfg.Devices[i]
			dev.Addresses = filterURLSchemePrefix(dev.Addresses, prefix)
		}
	}
}

func (cfg *Configuration) applyMigrations() {
	if cfg.Version > 0 && cfg.Version < OldestHandledVersion {
		l.Warnf("Configuration version %d is deprecated. Attempting best effort conversion, but please verify manually.", cfg.Version)
	}

	// Upgrade configuration versions as appropriate
	migrationsMut.Lock()
	migrations.apply(cfg)
	migrationsMut.Unlock()
}

func (cfg *Configuration) Device(id protocol.DeviceID) (DeviceConfiguration, int, bool) {
	for i, device := range cfg.Devices {
		if device.DeviceID == id {
			return device, i, true
		}
	}
	return DeviceConfiguration{}, 0, false
}

// DeviceMap returns a map of device ID to device configuration for the given configuration.
func (cfg *Configuration) DeviceMap() map[protocol.DeviceID]DeviceConfiguration {
	m := make(map[protocol.DeviceID]DeviceConfiguration, len(cfg.Devices))
	for _, dev := range cfg.Devices {
		m[dev.DeviceID] = dev
	}
	return m
}

func (cfg *Configuration) SetDevice(device DeviceConfiguration) {
	cfg.SetDevices([]DeviceConfiguration{device})
}

func (cfg *Configuration) SetDevices(devices []DeviceConfiguration) {
	inds := make(map[protocol.DeviceID]int, len(cfg.Devices))
	for i, device := range cfg.Devices {
		inds[device.DeviceID] = i
	}
	filtered := devices[:0]
	for _, device := range devices {
		if i, ok := inds[device.DeviceID]; ok {
			cfg.Devices[i] = device
		} else {
			filtered = append(filtered, device)
		}
	}
	cfg.Devices = append(cfg.Devices, filtered...)
}

func (cfg *Configuration) Folder(id string) (FolderConfiguration, int, bool) {
	for i, folder := range cfg.Folders {
		if folder.ID == id {
			return folder, i, true
		}
	}
	return FolderConfiguration{}, 0, false
}

// FolderMap returns a map of folder ID to folder configuration for the given configuration.
func (cfg *Configuration) FolderMap() map[string]FolderConfiguration {
	m := make(map[string]FolderConfiguration, len(cfg.Folders))
	for _, folder := range cfg.Folders {
		m[folder.ID] = folder
	}
	return m
}

// FolderPasswords returns the folder passwords set for this device, for
// folders that have an encryption password set.
func (cfg Configuration) FolderPasswords(device protocol.DeviceID) map[string]string {
	res := make(map[string]string, len(cfg.Folders))
	for _, folder := range cfg.Folders {
		if dev, ok := folder.Device(device); ok && dev.EncryptionPassword != "" {
			res[folder.ID] = dev.EncryptionPassword
		}
	}
	return res
}

func (cfg *Configuration) SetFolder(folder FolderConfiguration) {
	cfg.SetFolders([]FolderConfiguration{folder})
}

func (cfg *Configuration) SetFolders(folders []FolderConfiguration) {
	inds := make(map[string]int, len(cfg.Folders))
	for i, folder := range cfg.Folders {
		inds[folder.ID] = i
	}
	filtered := folders[:0]
	for _, folder := range folders {
		if i, ok := inds[folder.ID]; ok {
			cfg.Folders[i] = folder
		} else {
			filtered = append(filtered, folder)
		}
	}
	cfg.Folders = append(cfg.Folders, filtered...)
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

func (defaults *Defaults) prepare(myID protocol.DeviceID, existingDevices map[protocol.DeviceID]bool) {
	ensureZeroForNodefault(&FolderConfiguration{}, &defaults.Folder)
	ensureZeroForNodefault(&DeviceConfiguration{}, &defaults.Device)
	defaults.Folder.prepare(myID, existingDevices)
	defaults.Device.prepare(nil)
}

func ensureZeroForNodefault(empty interface{}, target interface{}) {
	util.CopyMatchingTag(empty, target, "nodefault", func(v string) bool {
		if len(v) > 0 && v != "true" {
			panic(fmt.Sprintf(`unexpected tag value: %s. expected untagged or "true"`, v))
		}
		return len(v) > 0
	})
}

func (i Ignores) Copy() Ignores {
	out := Ignores{Lines: make([]string, len(i.Lines))}
	copy(out.Lines, i.Lines)
	return out
}
