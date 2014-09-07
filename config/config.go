// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// Package config implements reading and writing of the syncthing configuration file.
package config

import (
	"encoding/xml"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strconv"

	"code.google.com/p/go.crypto/bcrypt"
	"github.com/syncthing/syncthing/events"
	"github.com/syncthing/syncthing/logger"
	"github.com/syncthing/syncthing/osutil"
	"github.com/syncthing/syncthing/protocol"
)

var l = logger.DefaultLogger

type Configuration struct {
	Location     string                    `xml:"-" json:"-"`
	Version      int                       `xml:"version,attr" default:"3"`
	Repositories []RepositoryConfiguration `xml:"repository"`
	Nodes        []NodeConfiguration       `xml:"node"`
	GUI          GUIConfiguration          `xml:"gui"`
	Options      OptionsConfiguration      `xml:"options"`
	XMLName      xml.Name                  `xml:"configuration" json:"-"`
}

type RepositoryConfiguration struct {
	ID              string                        `xml:"id,attr"`
	Directory       string                        `xml:"directory,attr"`
	Nodes           []RepositoryNodeConfiguration `xml:"node"`
	ReadOnly        bool                          `xml:"ro,attr"`
	RescanIntervalS int                           `xml:"rescanIntervalS,attr" default:"60"`
	IgnorePerms     bool                          `xml:"ignorePerms,attr"`
	Invalid         string                        `xml:"-"` // Set at runtime when there is an error, not saved
	Versioning      VersioningConfiguration       `xml:"versioning"`

	nodeIDs []protocol.NodeID
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

func (r *RepositoryConfiguration) NodeIDs() []protocol.NodeID {
	if r.nodeIDs == nil {
		for _, n := range r.Nodes {
			r.nodeIDs = append(r.nodeIDs, n.NodeID)
		}
	}
	return r.nodeIDs
}

type NodeConfiguration struct {
	NodeID      protocol.NodeID `xml:"id,attr"`
	Name        string          `xml:"name,attr,omitempty"`
	Addresses   []string        `xml:"address,omitempty"`
	Compression bool            `xml:"compression,attr"`
	CertName    string          `xml:"certName,attr,omitempty"`
}

type RepositoryNodeConfiguration struct {
	NodeID protocol.NodeID `xml:"id,attr"`

	Deprecated_Name      string   `xml:"name,attr,omitempty" json:"-"`
	Deprecated_Addresses []string `xml:"address,omitempty" json:"-"`
}

type OptionsConfiguration struct {
	ListenAddress      []string `xml:"listenAddress" default:"0.0.0.0:22000"`
	GlobalAnnServer    string   `xml:"globalAnnounceServer" default:"announce.syncthing.net:22026"`
	GlobalAnnEnabled   bool     `xml:"globalAnnounceEnabled" default:"true"`
	LocalAnnEnabled    bool     `xml:"localAnnounceEnabled" default:"true"`
	LocalAnnPort       int      `xml:"localAnnouncePort" default:"21025"`
	LocalAnnMCAddr     string   `xml:"localAnnounceMCAddr" default:"[ff32::5222]:21026"`
	ParallelRequests   int      `xml:"parallelRequests" default:"16"`
	MaxSendKbps        int      `xml:"maxSendKbps"`
	ReconnectIntervalS int      `xml:"reconnectionIntervalS" default:"60"`
	StartBrowser       bool     `xml:"startBrowser" default:"true"`
	UPnPEnabled        bool     `xml:"upnpEnabled" default:"true"`
	UPnPLease          int      `xml:"upnpLeaseMinutes" default:"0"`
	UPnPRenewal        int      `xml:"upnpRenewalMinutes" default:"30"`
	URAccepted         int      `xml:"urAccepted"` // Accepted usage reporting version; 0 for off (undecided), -1 for off (permanently)

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

func (cfg *Configuration) NodeMap() map[protocol.NodeID]NodeConfiguration {
	m := make(map[protocol.NodeID]NodeConfiguration, len(cfg.Nodes))
	for _, n := range cfg.Nodes {
		m[n.NodeID] = n
	}
	return m
}

func (cfg *Configuration) GetNodeConfiguration(nodeid protocol.NodeID) *NodeConfiguration {
	for i, node := range cfg.Nodes {
		if node.NodeID == nodeid {
			return &cfg.Nodes[i]
		}
	}
	return nil
}

func (cfg *Configuration) RepoMap() map[string]RepositoryConfiguration {
	m := make(map[string]RepositoryConfiguration, len(cfg.Repositories))
	for _, r := range cfg.Repositories {
		m[r.ID] = r
	}
	return m
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

func (cfg *Configuration) Save() error {
	fd, err := os.Create(cfg.Location + ".tmp")
	if err != nil {
		l.Warnln("Saving config:", err)
		return err
	}

	e := xml.NewEncoder(fd)
	e.Indent("", "    ")
	err = e.Encode(cfg)
	if err != nil {
		fd.Close()
		return err
	}
	_, err = fd.Write([]byte("\n"))

	if err != nil {
		l.Warnln("Saving config:", err)
		fd.Close()
		return err
	}

	err = fd.Close()
	if err != nil {
		l.Warnln("Saving config:", err)
		return err
	}

	err = osutil.Rename(cfg.Location+".tmp", cfg.Location)
	if err != nil {
		l.Warnln("Saving config:", err)
	}
	events.Default.Log(events.ConfigSaved, cfg)
	return err
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

func (cfg *Configuration) prepare(myID protocol.NodeID) {
	fillNilSlices(&cfg.Options)

	cfg.Options.ListenAddress = uniqueStrings(cfg.Options.ListenAddress)

	// Initialize an empty slice for repositories if the config has none
	if cfg.Repositories == nil {
		cfg.Repositories = []RepositoryConfiguration{}
	}

	// Check for missing, bad or duplicate repository ID:s
	var seenRepos = map[string]*RepositoryConfiguration{}
	var uniqueCounter int
	for i := range cfg.Repositories {
		repo := &cfg.Repositories[i]

		if len(repo.Directory) == 0 {
			repo.Invalid = "no directory configured"
			continue
		}

		if repo.ID == "" {
			repo.ID = "default"
		}

		if seen, ok := seenRepos[repo.ID]; ok {
			l.Warnf("Multiple repositories with ID %q; disabling", repo.ID)

			seen.Invalid = "duplicate repository ID"
			if seen.ID == repo.ID {
				uniqueCounter++
				seen.ID = fmt.Sprintf("%s~%d", repo.ID, uniqueCounter)
			}
			repo.Invalid = "duplicate repository ID"
			uniqueCounter++
			repo.ID = fmt.Sprintf("%s~%d", repo.ID, uniqueCounter)
		} else {
			seenRepos[repo.ID] = repo
		}
	}

	if cfg.Options.Deprecated_URDeclined {
		cfg.Options.URAccepted = -1
	}
	cfg.Options.Deprecated_URDeclined = false
	cfg.Options.Deprecated_UREnabled = false

	// Upgrade to v2 configuration if appropriate
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

	// Hash old cleartext passwords
	if len(cfg.GUI.Password) > 0 && cfg.GUI.Password[0] != '$' {
		hash, err := bcrypt.GenerateFromPassword([]byte(cfg.GUI.Password), 0)
		if err != nil {
			l.Warnln("bcrypting password:", err)
		} else {
			cfg.GUI.Password = string(hash)
		}
	}

	// Build a list of available nodes
	existingNodes := make(map[protocol.NodeID]bool)
	existingNodes[myID] = true
	for _, node := range cfg.Nodes {
		existingNodes[node.NodeID] = true
	}

	// Ensure this node is present in all relevant places
	me := cfg.GetNodeConfiguration(myID)
	if me == nil {
		myName, _ := os.Hostname()
		cfg.Nodes = append(cfg.Nodes, NodeConfiguration{
			NodeID: myID,
			Name:   myName,
		})
	}
	sort.Sort(NodeConfigurationList(cfg.Nodes))
	// Ensure that any loose nodes are not present in the wrong places
	// Ensure that there are no duplicate nodes
	for i := range cfg.Repositories {
		cfg.Repositories[i].Nodes = ensureNodePresent(cfg.Repositories[i].Nodes, myID)
		cfg.Repositories[i].Nodes = ensureExistingNodes(cfg.Repositories[i].Nodes, existingNodes)
		cfg.Repositories[i].Nodes = ensureNoDuplicates(cfg.Repositories[i].Nodes)
		sort.Sort(RepositoryNodeConfigurationList(cfg.Repositories[i].Nodes))
	}

	// An empty address list is equivalent to a single "dynamic" entry
	for i := range cfg.Nodes {
		n := &cfg.Nodes[i]
		if len(n.Addresses) == 0 || len(n.Addresses) == 1 && n.Addresses[0] == "" {
			n.Addresses = []string{"dynamic"}
		}
	}
}

func New(location string, myID protocol.NodeID) Configuration {
	var cfg Configuration

	cfg.Location = location

	setDefaults(&cfg)
	setDefaults(&cfg.Options)
	setDefaults(&cfg.GUI)

	cfg.prepare(myID)

	return cfg
}

func Load(location string, myID protocol.NodeID) (Configuration, error) {
	var cfg Configuration

	cfg.Location = location

	setDefaults(&cfg)
	setDefaults(&cfg.Options)
	setDefaults(&cfg.GUI)

	fd, err := os.Open(location)
	if err != nil {
		return Configuration{}, err
	}
	err = xml.NewDecoder(fd).Decode(&cfg)
	fd.Close()

	cfg.prepare(myID)

	return cfg, err
}

func convertV3V4(cfg *Configuration) {
	// In previous versions, rescan interval was common for each repository.
	// From now, it can be set independently. We have to make sure, that after upgrade
	// the individual rescan interval will be defined for every existing repository.
	for i := range cfg.Repositories {
		cfg.Repositories[i].RescanIntervalS = cfg.Options.Deprecated_RescanIntervalS
	}

	cfg.Options.Deprecated_RescanIntervalS = 0

	// In previous versions, repositories held full node configurations.
	// Since that's the only place where node configs were in V1, we still have
	// to define the deprecated fields to be able to upgrade from V1 to V4.
	for i, repo := range cfg.Repositories {

		for j := range repo.Nodes {
			rncfg := cfg.Repositories[i].Nodes[j]
			rncfg.Deprecated_Name = ""
			rncfg.Deprecated_Addresses = nil
		}
	}

	cfg.Version = 4
}

func convertV2V3(cfg *Configuration) {
	// In previous versions, compression was always on. When upgrading, enable
	// compression on all existing new. New nodes will get compression on by
	// default by the GUI.
	for i := range cfg.Nodes {
		cfg.Nodes[i].Compression = true
	}

	// The global discovery format and port number changed in v0.9. Having the
	// default announce server but old port number is guaranteed to be legacy.
	if cfg.Options.GlobalAnnServer == "announce.syncthing.net:22025" {
		cfg.Options.GlobalAnnServer = "announce.syncthing.net:22026"
	}

	cfg.Version = 3
}

func convertV1V2(cfg *Configuration) {
	// Collect the list of nodes.
	// Replace node configs inside repositories with only a reference to the nide ID.
	// Set all repositories to read only if the global read only flag is set.
	var nodes = map[string]RepositoryNodeConfiguration{}
	for i, repo := range cfg.Repositories {
		cfg.Repositories[i].ReadOnly = cfg.Options.Deprecated_ReadOnly
		for j, node := range repo.Nodes {
			id := node.NodeID.String()
			if _, ok := nodes[id]; !ok {
				nodes[id] = node
			}
			cfg.Repositories[i].Nodes[j] = RepositoryNodeConfiguration{NodeID: node.NodeID}
		}
	}
	cfg.Options.Deprecated_ReadOnly = false

	// Set and sort the list of nodes.
	for _, node := range nodes {
		cfg.Nodes = append(cfg.Nodes, NodeConfiguration{
			NodeID:    node.NodeID,
			Name:      node.Deprecated_Name,
			Addresses: node.Deprecated_Addresses,
		})
	}
	sort.Sort(NodeConfigurationList(cfg.Nodes))

	// GUI
	cfg.GUI.Address = cfg.Options.Deprecated_GUIAddress
	cfg.GUI.Enabled = cfg.Options.Deprecated_GUIEnabled
	cfg.Options.Deprecated_GUIEnabled = false
	cfg.Options.Deprecated_GUIAddress = ""

	cfg.Version = 2
}

type NodeConfigurationList []NodeConfiguration

func (l NodeConfigurationList) Less(a, b int) bool {
	return l[a].NodeID.Compare(l[b].NodeID) == -1
}
func (l NodeConfigurationList) Swap(a, b int) {
	l[a], l[b] = l[b], l[a]
}
func (l NodeConfigurationList) Len() int {
	return len(l)
}

type RepositoryNodeConfigurationList []RepositoryNodeConfiguration

func (l RepositoryNodeConfigurationList) Less(a, b int) bool {
	return l[a].NodeID.Compare(l[b].NodeID) == -1
}
func (l RepositoryNodeConfigurationList) Swap(a, b int) {
	l[a], l[b] = l[b], l[a]
}
func (l RepositoryNodeConfigurationList) Len() int {
	return len(l)
}

func ensureNodePresent(nodes []RepositoryNodeConfiguration, myID protocol.NodeID) []RepositoryNodeConfiguration {
	for _, node := range nodes {
		if node.NodeID.Equals(myID) {
			return nodes
		}
	}

	nodes = append(nodes, RepositoryNodeConfiguration{
		NodeID: myID,
	})

	return nodes
}

func ensureExistingNodes(nodes []RepositoryNodeConfiguration, existingNodes map[protocol.NodeID]bool) []RepositoryNodeConfiguration {
	count := len(nodes)
	i := 0
loop:
	for i < count {
		if _, ok := existingNodes[nodes[i].NodeID]; !ok {
			nodes[i] = nodes[count-1]
			count--
			continue loop
		}
		i++
	}
	return nodes[0:count]
}

func ensureNoDuplicates(nodes []RepositoryNodeConfiguration) []RepositoryNodeConfiguration {
	count := len(nodes)
	i := 0
	seenNodes := make(map[protocol.NodeID]bool)
loop:
	for i < count {
		id := nodes[i].NodeID
		if _, ok := seenNodes[id]; ok {
			nodes[i] = nodes[count-1]
			count--
			continue loop
		}
		seenNodes[id] = true
		i++
	}
	return nodes[0:count]
}
