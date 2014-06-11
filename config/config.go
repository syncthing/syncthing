// Copyright (C) 2014 Jakob Borg and other contributors. All rights reserved.
// Use of this source code is governed by an MIT-style license that can be
// found in the LICENSE file.

// Package config implements reading and writing of the syncthing configuration file.
package config

import (
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/calmh/syncthing/scanner"
	"code.google.com/p/go.crypto/bcrypt"
	"github.com/calmh/syncthing/logger"
)

var l = logger.DefaultLogger

type Configuration struct {
	Version      int                       `xml:"version,attr" default:"2"`
	Repositories []RepositoryConfiguration `xml:"repository"`
	Nodes        []NodeConfiguration       `xml:"node"`
	GUI          GUIConfiguration          `xml:"gui"`
	Options      OptionsConfiguration      `xml:"options"`
	XMLName      xml.Name                  `xml:"configuration" json:"-"`
}

// SyncOrderPattern allows a user to prioritize file downloading based on a
// regular expression.  If a file matches the Pattern the Priority will be
// assigned to the file.  If a file matches more than one Pattern the
// Priorities are summed.  This allows a user to, for example, prioritize files
// in a directory, as well as prioritize based on file type.  The higher the
// priority the "sooner" a file will be downloaded.  Files can be deprioritized
// by giving them a negative priority.  While Priority is represented as an
// integer, the expected range is something like -1000 to 1000.
type SyncOrderPattern struct {
	Pattern         string `xml:"pattern,attr"`
	Priority        int    `xml:"priority,attr"`
	compiledPattern *regexp.Regexp
}

func (s *SyncOrderPattern) CompiledPattern() *regexp.Regexp {
	if s.compiledPattern == nil {
		re, err := regexp.Compile(s.Pattern)
		if err != nil {
			l.Warnln("Could not compile regexp (" + s.Pattern + "): " + err.Error())
			s.compiledPattern = regexp.MustCompile("^\\0$")
		} else {
			s.compiledPattern = re
		}
	}
	return s.compiledPattern
}

type RepositoryConfiguration struct {
	ID                string                  `xml:"id,attr"`
	Directory         string                  `xml:"directory,attr"`
	Nodes             []NodeConfiguration     `xml:"node"`
	ReadOnly          bool                    `xml:"ro,attr"`
	IgnorePerms       bool                    `xml:"ignorePerms,attr"`
	Invalid           string                  `xml:"-"` // Set at runtime when there is an error, not saved
	Versioning        VersioningConfiguration `xml:"versioning"`
	SyncOrderPatterns []SyncOrderPattern      `xml:"syncorder>pattern"`

	nodeIDs []string
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

func (r *RepositoryConfiguration) NodeIDs() []string {
	if r.nodeIDs == nil {
		for _, n := range r.Nodes {
			r.nodeIDs = append(r.nodeIDs, n.NodeID)
		}
	}
	return r.nodeIDs
}

func (r RepositoryConfiguration) FileRanker() func(scanner.File) int {
	if len(r.SyncOrderPatterns) <= 0 {
		return nil
	}
	return func(f scanner.File) int {
		ret := 0
		for _, v := range r.SyncOrderPatterns {
			if v.CompiledPattern().MatchString(f.Name) {
				ret += v.Priority
			}
		}
		return ret
	}
}

type NodeConfiguration struct {
	NodeID    string   `xml:"id,attr"`
	Name      string   `xml:"name,attr,omitempty"`
	Addresses []string `xml:"address,omitempty"`
}

type OptionsConfiguration struct {
	ListenAddress      []string `xml:"listenAddress" default:"0.0.0.0:22000"`
	GlobalAnnServer    string   `xml:"globalAnnounceServer" default:"announce.syncthing.net:22025"`
	GlobalAnnEnabled   bool     `xml:"globalAnnounceEnabled" default:"true"`
	LocalAnnEnabled    bool     `xml:"localAnnounceEnabled" default:"true"`
	LocalAnnPort       int      `xml:"localAnnouncePort" default:"21025"`
	ParallelRequests   int      `xml:"parallelRequests" default:"16"`
	MaxSendKbps        int      `xml:"maxSendKbps"`
	RescanIntervalS    int      `xml:"rescanIntervalS" default:"60"`
	ReconnectIntervalS int      `xml:"reconnectionIntervalS" default:"60"`
	MaxChangeKbps      int      `xml:"maxChangeKbps" default:"10000"`
	StartBrowser       bool     `xml:"startBrowser" default:"true"`
	UPnPEnabled        bool     `xml:"upnpEnabled" default:"true"`

	UREnabled  bool `xml:"urEnabled"`  // If true, send usage reporting data
	URDeclined bool `xml:"urDeclined"` // If true, don't ask again
	URAccepted int  `xml:"urAccepted"` // Accepted usage reporting version

	Deprecated_ReadOnly   bool   `xml:"readOnly,omitempty" json:"-"`
	Deprecated_GUIEnabled bool   `xml:"guiEnabled,omitempty" json:"-"`
	Deprecated_GUIAddress string `xml:"guiAddress,omitempty" json:"-"`
}

type GUIConfiguration struct {
	Enabled  bool   `xml:"enabled,attr" default:"true"`
	Address  string `xml:"address" default:"127.0.0.1:8080"`
	User     string `xml:"user,omitempty"`
	Password string `xml:"password,omitempty"`
	UseTLS   bool   `xml:"tls,attr"`
	APIKey   string `xml:"apikey,omitempty"`
}

func (cfg *Configuration) NodeMap() map[string]NodeConfiguration {
	m := make(map[string]NodeConfiguration, len(cfg.Nodes))
	for _, n := range cfg.Nodes {
		m[n.NodeID] = n
	}
	return m
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

func Save(wr io.Writer, cfg Configuration) error {
	e := xml.NewEncoder(wr)
	e.Indent("", "    ")
	err := e.Encode(cfg)
	if err != nil {
		return err
	}
	_, err = wr.Write([]byte("\n"))
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

func Load(rd io.Reader, myID string) (Configuration, error) {
	var cfg Configuration

	setDefaults(&cfg)
	setDefaults(&cfg.Options)
	setDefaults(&cfg.GUI)

	var err error
	if rd != nil {
		err = xml.NewDecoder(rd).Decode(&cfg)
	}

	fillNilSlices(&cfg.Options)

	cfg.Options.ListenAddress = uniqueStrings(cfg.Options.ListenAddress)

	// Initialize an empty slice for repositories if the config has none
	if cfg.Repositories == nil {
		cfg.Repositories = []RepositoryConfiguration{}
	}

	// Sanitize node IDs
	for i := range cfg.Nodes {
		node := &cfg.Nodes[i]
		// Strip spaces and dashes
		node.NodeID = strings.Replace(node.NodeID, "-", "", -1)
		node.NodeID = strings.Replace(node.NodeID, " ", "", -1)
		node.NodeID = strings.ToUpper(node.NodeID)
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

		for i := range repo.Nodes {
			node := &repo.Nodes[i]
			// Strip spaces and dashes
			node.NodeID = strings.Replace(node.NodeID, "-", "", -1)
			node.NodeID = strings.Replace(node.NodeID, " ", "", -1)
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

	// Upgrade to v2 configuration if appropriate
	if cfg.Version == 1 {
		convertV1V2(&cfg)
	}

	// Hash old cleartext passwords
	if len(cfg.GUI.Password) > 0 && cfg.GUI.Password[0] != '$' {
		hash, err := bcrypt.GenerateFromPassword([]byte(cfg.GUI.Password), 0)
		if err != nil {
			l.Warnln(err)
		} else {
			cfg.GUI.Password = string(hash)
		}
	}

	// Ensure this node is present in all relevant places
	cfg.Nodes = ensureNodePresent(cfg.Nodes, myID)
	for i := range cfg.Repositories {
		cfg.Repositories[i].Nodes = ensureNodePresent(cfg.Repositories[i].Nodes, myID)
	}

	// An empty address list is equivalent to a single "dynamic" entry
	for i := range cfg.Nodes {
		n := &cfg.Nodes[i]
		if len(n.Addresses) == 0 || len(n.Addresses) == 1 && n.Addresses[0] == "" {
			n.Addresses = []string{"dynamic"}
		}
	}

	return cfg, err
}

func convertV1V2(cfg *Configuration) {
	// Collect the list of nodes.
	// Replace node configs inside repositories with only a reference to the nide ID.
	// Set all repositories to read only if the global read only flag is set.
	var nodes = map[string]NodeConfiguration{}
	for i, repo := range cfg.Repositories {
		cfg.Repositories[i].ReadOnly = cfg.Options.Deprecated_ReadOnly
		for j, node := range repo.Nodes {
			if _, ok := nodes[node.NodeID]; !ok {
				nodes[node.NodeID] = node
			}
			cfg.Repositories[i].Nodes[j] = NodeConfiguration{NodeID: node.NodeID}
		}
	}
	cfg.Options.Deprecated_ReadOnly = false

	// Set and sort the list of nodes.
	for _, node := range nodes {
		cfg.Nodes = append(cfg.Nodes, node)
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
	return l[a].NodeID < l[b].NodeID
}
func (l NodeConfigurationList) Swap(a, b int) {
	l[a], l[b] = l[b], l[a]
}
func (l NodeConfigurationList) Len() int {
	return len(l)
}

func ensureNodePresent(nodes []NodeConfiguration, myID string) []NodeConfiguration {
	var myIDExists bool
	for _, node := range nodes {
		if node.NodeID == myID {
			myIDExists = true
			break
		}
	}

	if !myIDExists {
		name, _ := os.Hostname()
		nodes = append(nodes, NodeConfiguration{
			NodeID: myID,
			Name:   name,
		})
	}

	sort.Sort(NodeConfigurationList(nodes))

	return nodes
}
