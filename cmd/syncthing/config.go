package main

import (
	"encoding/xml"
	"io"
	"os"
	"reflect"
	"sort"
	"strconv"

	"code.google.com/p/go.crypto/bcrypt"
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
	Invalid   string              `xml:"-"` // Set at runtime when there is an error, not saved
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
	UPnPEnabled        bool     `xml:"upnpEnabled" default:"true"`

	Deprecated_ReadOnly   bool   `xml:"readOnly,omitempty" json:"-"`
	Deprecated_GUIEnabled bool   `xml:"guiEnabled,omitempty" json:"-"`
	Deprecated_GUIAddress string `xml:"guiAddress,omitempty" json:"-"`
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

func readConfigXML(rd io.Reader, myID string) (Configuration, error) {
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

	// Check for missing, bad or duplicate repository ID:s
	var seenRepos = map[string]*RepositoryConfiguration{}
	for i := range cfg.Repositories {
		repo := &cfg.Repositories[i]

		if len(repo.Directory) == 0 {
			repo.Invalid = "empty directory"
			continue
		}

		if repo.ID == "" {
			repo.ID = "default"
		}

		if seen, ok := seenRepos[repo.ID]; ok {
			seen.Invalid = "duplicate repository ID"
			repo.Invalid = "duplicate repository ID"
			warnf("Multiple repositories with ID %q; disabling", repo.ID)
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
			warnln(err)
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

func invalidateRepo(repoID string, err error) {
	for i := range cfg.Repositories {
		repo := &cfg.Repositories[i]
		if repo.ID == repoID {
			repo.Invalid = err.Error()
			return
		}
	}
}
