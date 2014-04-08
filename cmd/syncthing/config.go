package main

import (
	"crypto/sha256"
	"encoding/xml"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strconv"
)

type Configuration struct {
	Version      int                       `xml:"version,attr" default:"2"`
	Repositories []RepositoryConfiguration `xml:"repository"`
	Nodes        []NodeConfiguration       `xml:"node"`
	GUI          GUIConfiguration          `xml:"gui"`
	Options      OptionsConfiguration      `xml:"options"`
	XMLName      xml.Name                  `xml:"configuration" json:"-"`
}

type RepositoryConfiguration struct {
	ID        string              `xml:"id,attr"`
	Directory string              `xml:"directory,attr"`
	Nodes     []NodeConfiguration `xml:"node"`
	ReadOnly  bool                `xml:"ro,attr"`
	nodeIDs   []string
}

func (r *RepositoryConfiguration) NodeIDs() []string {
	if r.nodeIDs == nil {
		for _, n := range r.Nodes {
			r.nodeIDs = append(r.nodeIDs, n.NodeID)
		}
	}
	return r.nodeIDs
}

type NodeConfiguration struct {
	NodeID    string   `xml:"id,attr"`
	Name      string   `xml:"name,attr,omitempty"`
	Addresses []string `xml:"address,omitempty"`
}

type OptionsConfiguration struct {
	ListenAddress      []string `xml:"listenAddress" default:":22000"`
	GlobalAnnServer    string   `xml:"globalAnnounceServer" default:"announce.syncthing.net:22025"`
	GlobalAnnEnabled   bool     `xml:"globalAnnounceEnabled" default:"true"`
	LocalAnnEnabled    bool     `xml:"localAnnounceEnabled" default:"true"`
	ParallelRequests   int      `xml:"parallelRequests" default:"16"`
	MaxSendKbps        int      `xml:"maxSendKbps"`
	RescanIntervalS    int      `xml:"rescanIntervalS" default:"60"`
	ReconnectIntervalS int      `xml:"reconnectionIntervalS" default:"60"`
	MaxChangeKbps      int      `xml:"maxChangeKbps" default:"1000"`
	StartBrowser       bool     `xml:"startBrowser" default:"true"`

	Deprecated_ReadOnly   bool   `xml:"readOnly,omitempty"`
	Deprecated_GUIEnabled bool   `xml:"guiEnabled,omitempty"`
	Deprecated_GUIAddress string `xml:"guiAddress,omitempty"`
}

type GUIConfiguration struct {
	Enabled  bool   `xml:"enabled,attr" default:"true"`
	Address  string `xml:"address" default:"127.0.0.1:8080"`
	User     string `xml:"user,omitempty"`
	Password string `xml:"password,omitempty"`
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

func writeConfigXML(wr io.Writer, cfg Configuration) error {
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

func readConfigXML(rd io.Reader) (Configuration, error) {
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

	var seenRepos = map[string]bool{}
	for i := range cfg.Repositories {
		if cfg.Repositories[i].ID == "" {
			cfg.Repositories[i].ID = "default"
		}

		id := cfg.Repositories[i].ID
		if seenRepos[id] {
			panic("duplicate repository ID " + id)
		}
		seenRepos[id] = true
	}

	if cfg.Version == 1 {
		convertV1V2(&cfg)
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

func clusterHash(nodes []NodeConfiguration) string {
	sort.Sort(NodeConfigurationList(nodes))
	h := sha256.New()
	for _, n := range nodes {
		h.Write([]byte(n.NodeID))
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

func cleanNodeList(nodes []NodeConfiguration, myID string) []NodeConfiguration {
	var myIDExists bool
	for _, node := range nodes {
		if node.NodeID == myID {
			myIDExists = true
			break
		}
	}

	if !myIDExists {
		nodes = append(nodes, NodeConfiguration{
			NodeID:    myID,
			Addresses: []string{"dynamic"},
			Name:      "",
		})
	}

	sort.Sort(NodeConfigurationList(nodes))

	return nodes
}
