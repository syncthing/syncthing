// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

// Package config implements reading and writing of the syncthing configuration file.
package config

import (
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"reflect"
	"sort"
	"strconv"

	"code.google.com/p/go.crypto/bcrypt"
	"github.com/syncthing/syncthing/internal/logger"
	"github.com/syncthing/syncthing/internal/protocol"
)

var l = logger.DefaultLogger

const CurrentVersion = 5

type Configuration struct {
	Version int                   `xml:"version,attr"`
	Folders []FolderConfiguration `xml:"folder"`
	Devices []DeviceConfiguration `xml:"device"`
	GUI     GUIConfiguration      `xml:"gui"`
	Options OptionsConfiguration  `xml:"options"`
	XMLName xml.Name              `xml:"configuration" json:"-"`

	OriginalVersion         int                   `xml:"-" json:"-"` // The version we read from disk, before any conversion
	Deprecated_Repositories []FolderConfiguration `xml:"repository" json:"-"`
	Deprecated_Nodes        []DeviceConfiguration `xml:"node" json:"-"`
}

type FolderConfiguration struct {
	ID              string                      `xml:"id,attr"`
	Path            string                      `xml:"path,attr"`
	Devices         []FolderDeviceConfiguration `xml:"device"`
	ReadOnly        bool                        `xml:"ro,attr"`
	RescanIntervalS int                         `xml:"rescanIntervalS,attr" default:"60"`
	IgnorePerms     bool                        `xml:"ignorePerms,attr"`
	Versioning      VersioningConfiguration     `xml:"versioning"`

	Invalid string `xml:"-"` // Set at runtime when there is an error, not saved

	deviceIDs []protocol.DeviceID

	Deprecated_Directory string                      `xml:"directory,omitempty,attr" json:"-"`
	Deprecated_Nodes     []FolderDeviceConfiguration `xml:"node" json:"-"`
}

func (r *FolderConfiguration) DeviceIDs() []protocol.DeviceID {
	if r.deviceIDs == nil {
		for _, n := range r.Devices {
			r.deviceIDs = append(r.deviceIDs, n.DeviceID)
		}
	}
	return r.deviceIDs
}

type VersioningConfiguration struct {
	Type   string `xml:"type,attr"`
	Params map[string]string
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
	DeviceID    protocol.DeviceID `xml:"id,attr"`
	Name        string            `xml:"name,attr,omitempty"`
	Addresses   []string          `xml:"address,omitempty"`
	Compression bool              `xml:"compression,attr"`
	CertName    string            `xml:"certName,attr,omitempty"`
	Introducer  bool              `xml:"introducer,attr"`
}

type FolderDeviceConfiguration struct {
	DeviceID protocol.DeviceID `xml:"id,attr"`

	Deprecated_Name      string   `xml:"name,attr,omitempty" json:"-"`
	Deprecated_Addresses []string `xml:"address,omitempty" json:"-"`
}

type OptionsConfiguration struct {
	ListenAddress        []string `xml:"listenAddress" default:"0.0.0.0:22000"`
	GlobalAnnServer      string   `xml:"globalAnnounceServer" default:"announce.syncthing.net:22026"`
	GlobalAnnEnabled     bool     `xml:"globalAnnounceEnabled" default:"true"`
	LocalAnnEnabled      bool     `xml:"localAnnounceEnabled" default:"true"`
	LocalAnnPort         int      `xml:"localAnnouncePort" default:"21025"`
	LocalAnnMCAddr       string   `xml:"localAnnounceMCAddr" default:"[ff32::5222]:21026"`
	MaxSendKbps          int      `xml:"maxSendKbps"`
	MaxRecvKbps          int      `xml:"maxRecvKbps"`
	ReconnectIntervalS   int      `xml:"reconnectionIntervalS" default:"60"`
	StartBrowser         bool     `xml:"startBrowser" default:"true"`
	UPnPEnabled          bool     `xml:"upnpEnabled" default:"true"`
	UPnPLease            int      `xml:"upnpLeaseMinutes" default:"0"`
	UPnPRenewal          int      `xml:"upnpRenewalMinutes" default:"30"`
	URAccepted           int      `xml:"urAccepted"` // Accepted usage reporting version; 0 for off (undecided), -1 for off (permanently)
	RestartOnWakeup      bool     `xml:"restartOnWakeup" default:"true"`
	AutoUpgradeIntervalH int      `xml:"autoUpgradeIntervalH" default:"12"` // 0 for off
	KeepTemporariesH     int      `xml:"keepTemporariesH" default:"24"`     // 0 for off

	Deprecated_RescanIntervalS int    `xml:"rescanIntervalS,omitempty" json:"-"`
	Deprecated_UREnabled       bool   `xml:"urEnabled,omitempty" json:"-"`
	Deprecated_URDeclined      bool   `xml:"urDeclined,omitempty" json:"-"`
	Deprecated_ReadOnly        bool   `xml:"readOnly,omitempty" json:"-"`
	Deprecated_GUIEnabled      bool   `xml:"guiEnabled,omitempty" json:"-"`
	Deprecated_GUIAddress      string `xml:"guiAddress,omitempty" json:"-"`
}

type GUIConfiguration struct {
	Enabled  bool   `xml:"enabled,attr" default:"true"`
	Address  string `xml:"address" default:"127.0.0.1:8080"`
	User     string `xml:"user,omitempty"`
	Password string `xml:"password,omitempty"`
	UseTLS   bool   `xml:"tls,attr"`
	APIKey   string `xml:"apikey,omitempty"`
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

	cfg.Options.ListenAddress = uniqueStrings(cfg.Options.ListenAddress)

	// Initialize an empty slice for folders if the config has none
	if cfg.Folders == nil {
		cfg.Folders = []FolderConfiguration{}
	}

	// Check for missing, bad or duplicate folder ID:s
	var seenFolders = map[string]*FolderConfiguration{}
	var uniqueCounter int
	for i := range cfg.Folders {
		folder := &cfg.Folders[i]

		if len(folder.Path) == 0 {
			folder.Invalid = "no directory configured"
			continue
		}

		if folder.ID == "" {
			folder.ID = "default"
		}

		if seen, ok := seenFolders[folder.ID]; ok {
			l.Warnf("Multiple folders with ID %q; disabling", folder.ID)

			seen.Invalid = "duplicate folder ID"
			if seen.ID == folder.ID {
				uniqueCounter++
				seen.ID = fmt.Sprintf("%s~%d", folder.ID, uniqueCounter)
			}
			folder.Invalid = "duplicate folder ID"
			uniqueCounter++
			folder.ID = fmt.Sprintf("%s~%d", folder.ID, uniqueCounter)
		} else {
			seenFolders[folder.ID] = folder
		}
	}

	if cfg.Options.Deprecated_URDeclined {
		cfg.Options.URAccepted = -1
	}
	cfg.Options.Deprecated_URDeclined = false
	cfg.Options.Deprecated_UREnabled = false

	// Upgrade to v1 configuration if appropriate
	if cfg.Version == 1 {
		convertV1V2(cfg)
	}

	// Upgrade to v3 configuration if appropriate
	if cfg.Version == 2 {
		convertV2V3(cfg)
	}

	// Upgrade to v4 configuration if appropriate
	if cfg.Version == 3 {
		convertV3V4(cfg)
	}

	// Upgrade to v5 configuration if appropriate
	if cfg.Version == 4 {
		convertV4V5(cfg)
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

	// All of the generic options require restart
	if !reflect.DeepEqual(from.Options, to.Options) || !reflect.DeepEqual(from.GUI, to.GUI) {
		return true
	}

	return false
}

func convertV4V5(cfg *Configuration) {
	// Renamed a bunch of fields in the structs.
	if cfg.Deprecated_Nodes == nil {
		cfg.Deprecated_Nodes = []DeviceConfiguration{}
	}

	if cfg.Deprecated_Repositories == nil {
		cfg.Deprecated_Repositories = []FolderConfiguration{}
	}

	cfg.Devices = cfg.Deprecated_Nodes
	cfg.Folders = cfg.Deprecated_Repositories

	for i := range cfg.Folders {
		cfg.Folders[i].Path = cfg.Folders[i].Deprecated_Directory
		cfg.Folders[i].Deprecated_Directory = ""
		cfg.Folders[i].Devices = cfg.Folders[i].Deprecated_Nodes
		cfg.Folders[i].Deprecated_Nodes = nil
	}

	cfg.Deprecated_Nodes = nil
	cfg.Deprecated_Repositories = nil

	cfg.Version = 5
}

func convertV3V4(cfg *Configuration) {
	// In previous versions, rescan interval was common for each folder.
	// From now, it can be set independently. We have to make sure, that after upgrade
	// the individual rescan interval will be defined for every existing folder.
	for i := range cfg.Deprecated_Repositories {
		cfg.Deprecated_Repositories[i].RescanIntervalS = cfg.Options.Deprecated_RescanIntervalS
	}

	cfg.Options.Deprecated_RescanIntervalS = 0

	// In previous versions, folders held full device configurations.
	// Since that's the only place where device configs were in V1, we still have
	// to define the deprecated fields to be able to upgrade from V1 to V4.
	for i, folder := range cfg.Deprecated_Repositories {

		for j := range folder.Deprecated_Nodes {
			rncfg := cfg.Deprecated_Repositories[i].Deprecated_Nodes[j]
			rncfg.Deprecated_Name = ""
			rncfg.Deprecated_Addresses = nil
		}
	}

	cfg.Version = 4
}

func convertV2V3(cfg *Configuration) {
	// In previous versions, compression was always on. When upgrading, enable
	// compression on all existing new. New devices will get compression on by
	// default by the GUI.
	for i := range cfg.Deprecated_Nodes {
		cfg.Deprecated_Nodes[i].Compression = true
	}

	// The global discovery format and port number changed in v0.9. Having the
	// default announce server but old port number is guaranteed to be legacy.
	if cfg.Options.GlobalAnnServer == "announce.syncthing.net:22025" {
		cfg.Options.GlobalAnnServer = "announce.syncthing.net:22026"
	}

	cfg.Version = 3
}

func convertV1V2(cfg *Configuration) {
	// Collect the list of devices.
	// Replace device configs inside folders with only a reference to the
	// device ID. Set all folders to read only if the global read only flag is
	// set.
	var devices = map[string]FolderDeviceConfiguration{}
	for i, folder := range cfg.Deprecated_Repositories {
		cfg.Deprecated_Repositories[i].ReadOnly = cfg.Options.Deprecated_ReadOnly
		for j, device := range folder.Deprecated_Nodes {
			id := device.DeviceID.String()
			if _, ok := devices[id]; !ok {
				devices[id] = device
			}
			cfg.Deprecated_Repositories[i].Deprecated_Nodes[j] = FolderDeviceConfiguration{DeviceID: device.DeviceID}
		}
	}
	cfg.Options.Deprecated_ReadOnly = false

	// Set and sort the list of devices.
	for _, device := range devices {
		cfg.Deprecated_Nodes = append(cfg.Deprecated_Nodes, DeviceConfiguration{
			DeviceID:  device.DeviceID,
			Name:      device.Deprecated_Name,
			Addresses: device.Deprecated_Addresses,
		})
	}
	sort.Sort(DeviceConfigurationList(cfg.Deprecated_Nodes))

	// GUI
	cfg.GUI.Address = cfg.Options.Deprecated_GUIAddress
	cfg.GUI.Enabled = cfg.Options.Deprecated_GUIEnabled
	cfg.Options.Deprecated_GUIEnabled = false
	cfg.Options.Deprecated_GUIAddress = ""

	cfg.Version = 2
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
					rv := reflect.MakeSlice(reflect.TypeOf([]string{}), 1, 1)
					rv.Index(0).SetString(v)
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
		m[s] = true
	}

	var us = make([]string, 0, len(m))
	for k := range m {
		us = append(us, k)
	}

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
