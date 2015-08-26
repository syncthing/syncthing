// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// Package config implements reading and writing of the syncthing configuration file.
package config

import (
	"encoding/xml"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/syncthing/protocol"
	"github.com/syncthing/syncthing/lib/osutil"
	"golang.org/x/crypto/bcrypt"
)

const (
	OldestHandledVersion = 5
	CurrentVersion       = 12
	MaxRescanIntervalS   = 365 * 24 * 60 * 60
)

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

type FolderConfiguration struct {
	ID                    string                      `xml:"id,attr" json:"id"`
	RawPath               string                      `xml:"path,attr" json:"path"`
	Devices               []FolderDeviceConfiguration `xml:"device" json:"devices"`
	ReadOnly              bool                        `xml:"ro,attr" json:"readOnly"`
	RescanIntervalS       int                         `xml:"rescanIntervalS,attr" json:"rescanIntervalS"`
	IgnorePerms           bool                        `xml:"ignorePerms,attr" json:"ignorePerms"`
	AutoNormalize         bool                        `xml:"autoNormalize,attr" json:"autoNormalize"`
	MinDiskFreePct        int                         `xml:"minDiskFreePct" json:"minDiskFreePct"`
	Versioning            VersioningConfiguration     `xml:"versioning" json:"versioning"`
	Copiers               int                         `xml:"copiers" json:"copiers"` // This defines how many files are handled concurrently.
	Pullers               int                         `xml:"pullers" json:"pullers"` // Defines how many blocks are fetched at the same time, possibly between separate copier routines.
	Hashers               int                         `xml:"hashers" json:"hashers"` // Less than one sets the value to the number of cores. These are CPU bound due to hashing.
	Order                 PullOrder                   `xml:"order" json:"order"`
	IgnoreDelete          bool                        `xml:"ignoreDelete" json:"ignoreDelete"`
	ScanProgressIntervalS int                         `xml:"scanProgressInterval" json:"scanProgressInterval"` // Set to a negative value to disable. Value of 0 will get replaced with value of 2 (default value)

	Invalid string `xml:"-" json:"invalid"` // Set at runtime when there is an error, not saved
}

func (f FolderConfiguration) Copy() FolderConfiguration {
	c := f
	c.Devices = make([]FolderDeviceConfiguration, len(f.Devices))
	copy(c.Devices, f.Devices)
	return c
}

func (f FolderConfiguration) Path() string {
	// This is intentionally not a pointer method, because things like
	// cfg.Folders["default"].Path() should be valid.

	// Attempt tilde expansion; leave unchanged in case of error
	if path, err := osutil.ExpandTilde(f.RawPath); err == nil {
		f.RawPath = path
	}

	// Attempt absolutification; leave unchanged in case of error
	if !filepath.IsAbs(f.RawPath) {
		// Abs() looks like a fairly expensive syscall on Windows, while
		// IsAbs() is a whole bunch of string mangling. I think IsAbs() may be
		// somewhat faster in the general case, hence the outer if...
		if path, err := filepath.Abs(f.RawPath); err == nil {
			f.RawPath = path
		}
	}

	// Attempt to enable long filename support on Windows. We may still not
	// have an absolute path here if the previous steps failed.
	if runtime.GOOS == "windows" && filepath.IsAbs(f.RawPath) && !strings.HasPrefix(f.RawPath, `\\`) {
		return `\\?\` + f.RawPath
	}

	return f.RawPath
}

func (f *FolderConfiguration) CreateMarker() error {
	if !f.HasMarker() {
		marker := filepath.Join(f.Path(), ".stfolder")
		fd, err := os.Create(marker)
		if err != nil {
			return err
		}
		fd.Close()
		osutil.HideFile(marker)
	}

	return nil
}

func (f *FolderConfiguration) HasMarker() bool {
	_, err := os.Stat(filepath.Join(f.Path(), ".stfolder"))
	if err != nil {
		return false
	}
	return true
}

func (f *FolderConfiguration) DeviceIDs() []protocol.DeviceID {
	deviceIDs := make([]protocol.DeviceID, len(f.Devices))
	for i, n := range f.Devices {
		deviceIDs[i] = n.DeviceID
	}
	return deviceIDs
}

type VersioningConfiguration struct {
	Type   string            `xml:"type,attr" json:"type"`
	Params map[string]string `json:"params"`
}

type InternalVersioningConfiguration struct {
	Type   string          `xml:"type,attr,omitempty"`
	Params []InternalParam `xml:"param"`
}

type InternalParam struct {
	Key string `xml:"key,attr"`
	Val string `xml:"val,attr"`
}

func (c *VersioningConfiguration) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	var tmp InternalVersioningConfiguration
	tmp.Type = c.Type
	for k, v := range c.Params {
		tmp.Params = append(tmp.Params, InternalParam{k, v})
	}

	return e.EncodeElement(tmp, start)

}

func (c *VersioningConfiguration) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var tmp InternalVersioningConfiguration
	err := d.DecodeElement(&tmp, &start)
	if err != nil {
		return err
	}

	c.Type = tmp.Type
	c.Params = make(map[string]string, len(tmp.Params))
	for _, p := range tmp.Params {
		c.Params[p.Key] = p.Val
	}
	return nil
}

type DeviceConfiguration struct {
	DeviceID    protocol.DeviceID    `xml:"id,attr" json:"deviceID"`
	Name        string               `xml:"name,attr,omitempty" json:"name"`
	Addresses   []string             `xml:"address,omitempty" json:"addresses"`
	Compression protocol.Compression `xml:"compression,attr" json:"compression"`
	CertName    string               `xml:"certName,attr,omitempty" json:"certName"`
	Introducer  bool                 `xml:"introducer,attr" json:"introducer"`
}

func (orig DeviceConfiguration) Copy() DeviceConfiguration {
	c := orig
	c.Addresses = make([]string, len(orig.Addresses))
	copy(c.Addresses, orig.Addresses)
	return c
}

type FolderDeviceConfiguration struct {
	DeviceID protocol.DeviceID `xml:"id,attr" json:"deviceID"`
}

type OptionsConfiguration struct {
	ListenAddress           []string `xml:"listenAddress" json:"listenAddress" default:"tcp://0.0.0.0:22000"`
	GlobalAnnServers        []string `xml:"globalAnnounceServer" json:"globalAnnounceServers" json:"globalAnnounceServer" default:"udp4://announce.syncthing.net:22027, udp6://announce-v6.syncthing.net:22027"`
	GlobalAnnEnabled        bool     `xml:"globalAnnounceEnabled" json:"globalAnnounceEnabled" default:"true"`
	LocalAnnEnabled         bool     `xml:"localAnnounceEnabled" json:"localAnnounceEnabled" default:"true"`
	LocalAnnPort            int      `xml:"localAnnouncePort" json:"localAnnouncePort" default:"21027"`
	LocalAnnMCAddr          string   `xml:"localAnnounceMCAddr" json:"localAnnounceMCAddr" default:"[ff12::8384]:21027"`
	RelayServers            []string `xml:"relayServer" json:"relayServers" default:"dynamic+https://relays.syncthing.net"`
	MaxSendKbps             int      `xml:"maxSendKbps" json:"maxSendKbps"`
	MaxRecvKbps             int      `xml:"maxRecvKbps" json:"maxRecvKbps"`
	ReconnectIntervalS      int      `xml:"reconnectionIntervalS" json:"reconnectionIntervalS" default:"60"`
	RelaysEnabled           bool     `xml:"relaysEnabled" json:"relaysEnabled" default:"true"`
	RelayReconnectIntervalM int      `xml:"relayReconnectIntervalM" json:"relayReconnectIntervalM" default:"10"`
	RelayWithoutGlobalAnn   bool     `xml:"relayWithoutGlobalAnn" json:"relayWithoutGlobalAnn" default:"false"`
	StartBrowser            bool     `xml:"startBrowser" json:"startBrowser" default:"true"`
	UPnPEnabled             bool     `xml:"upnpEnabled" json:"upnpEnabled" default:"true"`
	UPnPLeaseM              int      `xml:"upnpLeaseMinutes" json:"upnpLeaseMinutes" default:"60"`
	UPnPRenewalM            int      `xml:"upnpRenewalMinutes" json:"upnpRenewalMinutes" default:"30"`
	UPnPTimeoutS            int      `xml:"upnpTimeoutSeconds" json:"upnpTimeoutSeconds" default:"10"`
	URAccepted              int      `xml:"urAccepted" json:"urAccepted"` // Accepted usage reporting version; 0 for off (undecided), -1 for off (permanently)
	URUniqueID              string   `xml:"urUniqueID" json:"urUniqueId"` // Unique ID for reporting purposes, regenerated when UR is turned on.
	RestartOnWakeup         bool     `xml:"restartOnWakeup" json:"restartOnWakeup" default:"true"`
	AutoUpgradeIntervalH    int      `xml:"autoUpgradeIntervalH" json:"autoUpgradeIntervalH" default:"12"` // 0 for off
	KeepTemporariesH        int      `xml:"keepTemporariesH" json:"keepTemporariesH" default:"24"`         // 0 for off
	CacheIgnoredFiles       bool     `xml:"cacheIgnoredFiles" json:"cacheIgnoredFiles" default:"true"`
	ProgressUpdateIntervalS int      `xml:"progressUpdateIntervalS" json:"progressUpdateIntervalS" default:"5"`
	SymlinksEnabled         bool     `xml:"symlinksEnabled" json:"symlinksEnabled" default:"true"`
	LimitBandwidthInLan     bool     `xml:"limitBandwidthInLan" json:"limitBandwidthInLan" default:"false"`
	DatabaseBlockCacheMiB   int      `xml:"databaseBlockCacheMiB" json:"databaseBlockCacheMiB" default:"0"`
	PingTimeoutS            int      `xml:"pingTimeoutS" json:"pingTimeoutS" default:"30"`
	PingIdleTimeS           int      `xml:"pingIdleTimeS" json:"pingIdleTimeS" default:"60"`
	MinHomeDiskFreePct      int      `xml:"minHomeDiskFreePct" json:"minHomeDiskFreePct" default:"1"`
}

func (orig OptionsConfiguration) Copy() OptionsConfiguration {
	c := orig
	c.ListenAddress = make([]string, len(orig.ListenAddress))
	copy(c.ListenAddress, orig.ListenAddress)
	c.GlobalAnnServers = make([]string, len(orig.GlobalAnnServers))
	copy(c.GlobalAnnServers, orig.GlobalAnnServers)
	return c
}

type GUIConfiguration struct {
	Enabled  bool   `xml:"enabled,attr" json:"enabled" default:"true"`
	Address  string `xml:"address" json:"address" default:"127.0.0.1:8384"`
	User     string `xml:"user,omitempty" json:"user"`
	Password string `xml:"password,omitempty" json:"password"`
	UseTLS   bool   `xml:"tls,attr" json:"useTLS"`
	APIKey   string `xml:"apikey,omitempty" json:"apiKey"`
}

func New(myID protocol.DeviceID) Configuration {
	var cfg Configuration
	cfg.Version = CurrentVersion
	cfg.OriginalVersion = CurrentVersion

	setDefaults(&cfg)
	setDefaults(&cfg.Options)
	setDefaults(&cfg.GUI)

	cfg.prepare(myID)

	return cfg
}

func ReadXML(r io.Reader, myID protocol.DeviceID) (Configuration, error) {
	var cfg Configuration

	setDefaults(&cfg)
	setDefaults(&cfg.Options)
	setDefaults(&cfg.GUI)

	err := xml.NewDecoder(r).Decode(&cfg)
	cfg.OriginalVersion = cfg.Version

	cfg.prepare(myID)
	return cfg, err
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
	fillNilSlices(&cfg.Options)

	// Initialize an empty slices
	if cfg.Folders == nil {
		cfg.Folders = []FolderConfiguration{}
	}
	if cfg.IgnoredDevices == nil {
		cfg.IgnoredDevices = []protocol.DeviceID{}
	}

	// Check for missing, bad or duplicate folder ID:s
	var seenFolders = map[string]*FolderConfiguration{}
	for i := range cfg.Folders {
		folder := &cfg.Folders[i]

		if len(folder.RawPath) == 0 {
			folder.Invalid = "no directory configured"
			continue
		}

		// The reason it's done like this:
		// C:          ->  C:\            ->  C:\        (issue that this is trying to fix)
		// C:\somedir  ->  C:\somedir\    ->  C:\somedir
		// C:\somedir\ ->  C:\somedir\\   ->  C:\somedir
		// This way in the tests, we get away without OS specific separators
		// in the test configs.
		folder.RawPath = filepath.Dir(folder.RawPath + string(filepath.Separator))

		if folder.ID == "" {
			folder.ID = "default"
		}

		if folder.RescanIntervalS > MaxRescanIntervalS {
			folder.RescanIntervalS = MaxRescanIntervalS
		} else if folder.RescanIntervalS < 0 {
			folder.RescanIntervalS = 0
		}

		if seen, ok := seenFolders[folder.ID]; ok {
			l.Warnf("Multiple folders with ID %q; disabling", folder.ID)
			seen.Invalid = "duplicate folder ID"
			folder.Invalid = "duplicate folder ID"
		} else {
			seenFolders[folder.ID] = folder
		}
	}

	cfg.Options.ListenAddress = uniqueStrings(cfg.Options.ListenAddress)
	cfg.Options.GlobalAnnServers = uniqueStrings(cfg.Options.GlobalAnnServers)

	if cfg.Version < OldestHandledVersion {
		l.Warnf("Configuration version %d is deprecated. Attempting best effort conversion, but please verify manually.", cfg.Version)
	}

	// Upgrade configuration versions as appropriate
	if cfg.Version <= 5 {
		convertV5V6(cfg)
	}
	if cfg.Version == 6 {
		convertV6V7(cfg)
	}
	if cfg.Version == 7 {
		convertV7V8(cfg)
	}
	if cfg.Version == 8 {
		convertV8V9(cfg)
	}
	if cfg.Version == 9 {
		convertV9V10(cfg)
	}
	if cfg.Version == 10 {
		convertV10V11(cfg)
	}
	if cfg.Version == 11 {
		convertV11V12(cfg)
	}

	// Hash old cleartext passwords
	if len(cfg.GUI.Password) > 0 && cfg.GUI.Password[0] != '$' {
		hash, err := bcrypt.GenerateFromPassword([]byte(cfg.GUI.Password), 0)
		if err != nil {
			l.Warnln("bcrypting password:", err)
		} else {
			cfg.GUI.Password = string(hash)
		}
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

	sort.Sort(DeviceConfigurationList(cfg.Devices))
	// Ensure that any loose devices are not present in the wrong places
	// Ensure that there are no duplicate devices
	// Ensure that puller settings are sane
	for i := range cfg.Folders {
		cfg.Folders[i].Devices = ensureDevicePresent(cfg.Folders[i].Devices, myID)
		cfg.Folders[i].Devices = ensureExistingDevices(cfg.Folders[i].Devices, existingDevices)
		cfg.Folders[i].Devices = ensureNoDuplicates(cfg.Folders[i].Devices)
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
		cfg.GUI.APIKey = randomString(32)
	}
}

// ChangeRequiresRestart returns true if updating the configuration requires a
// complete restart.
func ChangeRequiresRestart(from, to Configuration) bool {
	// Adding, removing or changing folders requires restart
	if !reflect.DeepEqual(from.Folders, to.Folders) {
		return true
	}

	// Removing a device requres restart
	toDevs := make(map[protocol.DeviceID]bool, len(from.Devices))
	for _, dev := range to.Devices {
		toDevs[dev.DeviceID] = true
	}
	for _, dev := range from.Devices {
		if _, ok := toDevs[dev.DeviceID]; !ok {
			return true
		}
	}

	// Changing usage reporting to on or off does not require a restart.
	to.Options.URAccepted = from.Options.URAccepted
	to.Options.URUniqueID = from.Options.URUniqueID

	// All of the generic options require restart
	if !reflect.DeepEqual(from.Options, to.Options) || !reflect.DeepEqual(from.GUI, to.GUI) {
		return true
	}

	return false
}

func convertV10V11(cfg *Configuration) {
	// Set minimum disk free of existing folders to 1%
	for i := range cfg.Folders {
		cfg.Folders[i].MinDiskFreePct = 1
	}
	cfg.Version = 11
}

func convertV11V12(cfg *Configuration) {
	// Change listen address schema
	for i, addr := range cfg.Options.ListenAddress {
		if len(addr) > 0 && !strings.HasPrefix(addr, "tcp://") {
			cfg.Options.ListenAddress[i] = fmt.Sprintf("tcp://%s", addr)
		}
	}

	for i, device := range cfg.Devices {
		for j, addr := range device.Addresses {
			if addr != "dynamic" && addr != "" {
				cfg.Devices[i].Addresses[j] = fmt.Sprintf("tcp://%s", addr)
			}
		}
	}

	// Use new discovery server
	for i, addr := range cfg.Options.GlobalAnnServers {
		if addr == "udp4://announce.syncthing.net:22026" {
			cfg.Options.GlobalAnnServers[i] = "udp4://announce.syncthing.net:22027"
		} else if addr == "udp6://announce-v6.syncthing.net:22026" {
			cfg.Options.GlobalAnnServers[i] = "udp6://announce-v6.syncthing.net:22027"
		} else if addr == "udp4://194.126.249.5:22026" {
			cfg.Options.GlobalAnnServers[i] = "udp4://194.126.249.5:22027"
		} else if addr == "udp6://[2001:470:28:4d6::5]:22026" {
			cfg.Options.GlobalAnnServers[i] = "udp6://[2001:470:28:4d6::5]:22027"
		}
	}

	// Use new multicast group
	if cfg.Options.LocalAnnMCAddr == "[ff32::5222]:21026" {
		cfg.Options.LocalAnnMCAddr = "[ff12::8384]:21027"
	}

	// Use new local discovery port
	if cfg.Options.LocalAnnPort == 21025 {
		cfg.Options.LocalAnnPort = 21027
	}

	cfg.Version = 12
}

func convertV9V10(cfg *Configuration) {
	// Enable auto normalization on existing folders.
	for i := range cfg.Folders {
		cfg.Folders[i].AutoNormalize = true
	}
	cfg.Version = 10
}

func convertV8V9(cfg *Configuration) {
	// Compression is interpreted and serialized differently, but no enforced
	// changes. Still need a new version number since the compression stuff
	// isn't understandable by earlier versions.
	cfg.Version = 9
}

func convertV7V8(cfg *Configuration) {
	// Add IPv6 announce server
	if len(cfg.Options.GlobalAnnServers) == 1 && cfg.Options.GlobalAnnServers[0] == "udp4://announce.syncthing.net:22026" {
		cfg.Options.GlobalAnnServers = append(cfg.Options.GlobalAnnServers, "udp6://announce-v6.syncthing.net:22026")
	}

	cfg.Version = 8
}

func convertV6V7(cfg *Configuration) {
	// Migrate announce server addresses to the new URL based format
	for i := range cfg.Options.GlobalAnnServers {
		cfg.Options.GlobalAnnServers[i] = "udp4://" + cfg.Options.GlobalAnnServers[i]
	}

	cfg.Version = 7
}

func convertV5V6(cfg *Configuration) {
	// Added ".stfolder" file at folder roots to identify mount issues
	// Doesn't affect the config itself, but uses config migrations to identify
	// the migration point.
	for _, folder := range Wrap("", *cfg).Folders() {
		// Best attempt, if it fails, it fails, the user will have to fix
		// it up manually, as the repo will not get started.
		folder.CreateMarker()
	}

	cfg.Version = 6
}

func setDefaults(data interface{}) error {
	s := reflect.ValueOf(data).Elem()
	t := s.Type()

	for i := 0; i < s.NumField(); i++ {
		f := s.Field(i)
		tag := t.Field(i).Tag

		v := tag.Get("default")
		if len(v) > 0 {
			switch f.Interface().(type) {
			case string:
				f.SetString(v)

			case int:
				i, err := strconv.ParseInt(v, 10, 64)
				if err != nil {
					return err
				}
				f.SetInt(i)

			case bool:
				f.SetBool(v == "true")

			case []string:
				// We don't do anything with string slices here. Any default
				// we set will be appended to by the XML decoder, so we fill
				// those after decoding.

			default:
				panic(f.Type())
			}
		}
	}
	return nil
}

// fillNilSlices sets default value on slices that are still nil.
func fillNilSlices(data interface{}) error {
	s := reflect.ValueOf(data).Elem()
	t := s.Type()

	for i := 0; i < s.NumField(); i++ {
		f := s.Field(i)
		tag := t.Field(i).Tag

		v := tag.Get("default")
		if len(v) > 0 {
			switch f.Interface().(type) {
			case []string:
				if f.IsNil() {
					// Treat the default as a comma separated slice
					vs := strings.Split(v, ",")
					for i := range vs {
						vs[i] = strings.TrimSpace(vs[i])
					}

					rv := reflect.MakeSlice(reflect.TypeOf([]string{}), len(vs), len(vs))
					for i, v := range vs {
						rv.Index(i).SetString(v)
					}
					f.Set(rv)
				}
			}
		}
	}
	return nil
}

func uniqueStrings(ss []string) []string {
	var m = make(map[string]bool, len(ss))
	for _, s := range ss {
		m[strings.Trim(s, " ")] = true
	}

	var us = make([]string, 0, len(m))
	for k := range m {
		us = append(us, k)
	}

	sort.Strings(us)

	return us
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

func ensureNoDuplicates(devices []FolderDeviceConfiguration) []FolderDeviceConfiguration {
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

type FolderDeviceConfigurationList []FolderDeviceConfiguration

func (l FolderDeviceConfigurationList) Less(a, b int) bool {
	return l[a].DeviceID.Compare(l[b].DeviceID) == -1
}
func (l FolderDeviceConfigurationList) Swap(a, b int) {
	l[a], l[b] = l[b], l[a]
}
func (l FolderDeviceConfigurationList) Len() int {
	return len(l)
}

// randomCharset contains the characters that can make up a randomString().
const randomCharset = "01234567890abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ-"

// randomString returns a string of random characters (taken from
// randomCharset) of the specified length.
func randomString(l int) string {
	bs := make([]byte, l)
	for i := range bs {
		bs[i] = randomCharset[rand.Intn(len(randomCharset))]
	}
	return string(bs)
}

type PullOrder int

const (
	OrderRandom PullOrder = iota // default is random
	OrderAlphabetic
	OrderSmallestFirst
	OrderLargestFirst
	OrderOldestFirst
	OrderNewestFirst
)

func (o PullOrder) String() string {
	switch o {
	case OrderRandom:
		return "random"
	case OrderAlphabetic:
		return "alphabetic"
	case OrderSmallestFirst:
		return "smallestFirst"
	case OrderLargestFirst:
		return "largestFirst"
	case OrderOldestFirst:
		return "oldestFirst"
	case OrderNewestFirst:
		return "newestFirst"
	default:
		return "unknown"
	}
}

func (o PullOrder) MarshalText() ([]byte, error) {
	return []byte(o.String()), nil
}

func (o *PullOrder) UnmarshalText(bs []byte) error {
	switch string(bs) {
	case "random":
		*o = OrderRandom
	case "alphabetic":
		*o = OrderAlphabetic
	case "smallestFirst":
		*o = OrderSmallestFirst
	case "largestFirst":
		*o = OrderLargestFirst
	case "oldestFirst":
		*o = OrderOldestFirst
	case "newestFirst":
		*o = OrderNewestFirst
	default:
		*o = OrderRandom
	}
	return nil
}
