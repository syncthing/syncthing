package main

import (
	"crypto/sha256"
	"encoding/xml"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

type Configuration struct {
	Version      int                       `xml:"version,attr" default:"1"`
	Repositories []RepositoryConfiguration `xml:"repository"`
	Options      OptionsConfiguration      `xml:"options"`
	XMLName      xml.Name                  `xml:"configuration" json:"-"`
}

type RepositoryConfiguration struct {
	Directory string              `xml:"directory,attr"`
	Nodes     []NodeConfiguration `xml:"node"`
}

type NodeConfiguration struct {
	NodeID    string   `xml:"id,attr"`
	Name      string   `xml:"name,attr"`
	Addresses []string `xml:"address"`
}

type OptionsConfiguration struct {
	ListenAddress      []string `xml:"listenAddress" default:":22000" ini:"listen-address"`
	ReadOnly           bool     `xml:"readOnly" ini:"read-only"`
	FollowSymlinks     bool     `xml:"followSymlinks" default:"true" ini:"follow-symlinks"`
	GUIEnabled         bool     `xml:"guiEnabled" default:"true" ini:"gui-enabled"`
	GUIAddress         string   `xml:"guiAddress" default:"127.0.0.1:8080" ini:"gui-address"`
	GlobalAnnServer    string   `xml:"globalAnnounceServer" default:"announce.syncthing.net:22025" ini:"global-announce-server"`
	GlobalAnnEnabled   bool     `xml:"globalAnnounceEnabled" default:"true" ini:"global-announce-enabled"`
	LocalAnnEnabled    bool     `xml:"localAnnounceEnabled" default:"true" ini:"local-announce-enabled"`
	ParallelRequests   int      `xml:"parallelRequests" default:"16" ini:"parallel-requests"`
	MaxSendKbps        int      `xml:"maxSendKbps" ini:"max-send-kbps"`
	RescanIntervalS    int      `xml:"rescanIntervalS" default:"60" ini:"rescan-interval"`
	ReconnectIntervalS int      `xml:"reconnectionIntervalS" default:"60" ini:"reconnection-interval"`
	MaxChangeKbps      int      `xml:"maxChangeKbps" default:"1000" ini:"max-change-bw"`
	StartBrowser       bool     `xml:"startBrowser" default:"true"`
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

func readConfigINI(m map[string]string, data interface{}) error {
	s := reflect.ValueOf(data).Elem()
	t := s.Type()

	for i := 0; i < s.NumField(); i++ {
		f := s.Field(i)
		tag := t.Field(i).Tag

		name := tag.Get("ini")
		if len(name) == 0 {
			name = strings.ToLower(t.Field(i).Name)
		}

		if v, ok := m[name]; ok {
			switch f.Interface().(type) {
			case string:
				f.SetString(v)

			case int:
				i, err := strconv.ParseInt(v, 10, 64)
				if err == nil {
					f.SetInt(i)
				}

			case bool:
				f.SetBool(v == "true")

			default:
				panic(f.Type())
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

	var err error
	if rd != nil {
		err = xml.NewDecoder(rd).Decode(&cfg)
	}

	fillNilSlices(&cfg.Options)

	cfg.Options.ListenAddress = uniqueStrings(cfg.Options.ListenAddress)
	return cfg, err
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
