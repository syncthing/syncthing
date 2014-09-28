// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package config

import (
	"os"
	"reflect"
	"testing"

	"github.com/syncthing/syncthing/internal/protocol"
)

var node1, node2, node3, node4 protocol.NodeID

func init() {
	node1, _ = protocol.NodeIDFromString("AIR6LPZ7K4PTTUXQSMUUCPQ5YWOEDFIIQJUG7772YQXXR5YD6AWQ")
	node2, _ = protocol.NodeIDFromString("GYRZZQB-IRNPV4Z-T7TC52W-EQYJ3TT-FDQW6MW-DFLMU42-SSSU6EM-FBK2VAY")
	node3, _ = protocol.NodeIDFromString("LGFPDIT-7SKNNJL-VJZA4FC-7QNCRKA-CE753K7-2BW5QDK-2FOZ7FR-FEP57QJ")
	node4, _ = protocol.NodeIDFromString("P56IOI7-MZJNU2Y-IQGDREY-DM2MGTI-MGL3BXN-PQ6W5BM-TBBZ4TJ-XZWICQ2")
}

func TestDefaultValues(t *testing.T) {
	expected := OptionsConfiguration{
		ListenAddress:        []string{"0.0.0.0:22000"},
		GlobalAnnServer:      "announce.syncthing.net:22026",
		GlobalAnnEnabled:     true,
		LocalAnnEnabled:      true,
		LocalAnnPort:         21025,
		LocalAnnMCAddr:       "[ff32::5222]:21026",
		ParallelRequests:     16,
		MaxSendKbps:          0,
		MaxRecvKbps:          0,
		ReconnectIntervalS:   60,
		StartBrowser:         true,
		UPnPEnabled:          true,
		UPnPLease:            0,
		UPnPRenewal:          30,
		RestartOnWakeup:      true,
		AutoUpgradeIntervalH: 12,
	}

	cfg := New("test", node1)

	if !reflect.DeepEqual(cfg.Options, expected) {
		t.Errorf("Default config differs;\n  E: %#v\n  A: %#v", expected, cfg.Options)
	}
}

func TestNodeConfig(t *testing.T) {
	for i, ver := range []string{"v1", "v2", "v3", "v4"} {
		cfg, err := Load("testdata/"+ver+".xml", node1)
		if err != nil {
			t.Error(err)
		}

		expectedRepos := []RepositoryConfiguration{
			{
				ID:              "test",
				Directory:       "~/Sync",
				Nodes:           []RepositoryNodeConfiguration{{NodeID: node1}, {NodeID: node4}},
				ReadOnly:        true,
				RescanIntervalS: 600,
			},
		}
		expectedNodes := []NodeConfiguration{
			{
				NodeID:      node1,
				Name:        "node one",
				Addresses:   []string{"a"},
				Compression: true,
			},
			{
				NodeID:      node4,
				Name:        "node two",
				Addresses:   []string{"b"},
				Compression: true,
			},
		}
		expectedNodeIDs := []protocol.NodeID{node1, node4}

		if cfg.Version != 4 {
			t.Errorf("%d: Incorrect version %d != 3", i, cfg.Version)
		}
		if !reflect.DeepEqual(cfg.Repositories, expectedRepos) {
			t.Errorf("%d: Incorrect Repositories\n  A: %#v\n  E: %#v", i, cfg.Repositories, expectedRepos)
		}
		if !reflect.DeepEqual(cfg.Nodes, expectedNodes) {
			t.Errorf("%d: Incorrect Nodes\n  A: %#v\n  E: %#v", i, cfg.Nodes, expectedNodes)
		}
		if !reflect.DeepEqual(cfg.Repositories[0].NodeIDs(), expectedNodeIDs) {
			t.Errorf("%d: Incorrect NodeIDs\n  A: %#v\n  E: %#v", i, cfg.Repositories[0].NodeIDs(), expectedNodeIDs)
		}

		if len(cfg.NodeMap()) != len(expectedNodes) {
			t.Errorf("Unexpected number of NodeMap() entries")
		}
		if len(cfg.RepoMap()) != len(expectedRepos) {
			t.Errorf("Unexpected number of RepoMap() entries")
		}
	}
}

func TestNoListenAddress(t *testing.T) {
	cfg, err := Load("testdata/nolistenaddress.xml", node1)
	if err != nil {
		t.Error(err)
	}

	expected := []string{""}
	if !reflect.DeepEqual(cfg.Options.ListenAddress, expected) {
		t.Errorf("Unexpected ListenAddress %#v", cfg.Options.ListenAddress)
	}
}

func TestOverriddenValues(t *testing.T) {
	expected := OptionsConfiguration{
		ListenAddress:        []string{":23000"},
		GlobalAnnServer:      "syncthing.nym.se:22026",
		GlobalAnnEnabled:     false,
		LocalAnnEnabled:      false,
		LocalAnnPort:         42123,
		LocalAnnMCAddr:       "quux:3232",
		ParallelRequests:     32,
		MaxSendKbps:          1234,
		MaxRecvKbps:          2341,
		ReconnectIntervalS:   6000,
		StartBrowser:         false,
		UPnPEnabled:          false,
		UPnPLease:            60,
		UPnPRenewal:          15,
		RestartOnWakeup:      false,
		AutoUpgradeIntervalH: 24,
	}

	cfg, err := Load("testdata/overridenvalues.xml", node1)
	if err != nil {
		t.Error(err)
	}

	if !reflect.DeepEqual(cfg.Options, expected) {
		t.Errorf("Overridden config differs;\n  E: %#v\n  A: %#v", expected, cfg.Options)
	}
}

func TestNodeAddressesDynamic(t *testing.T) {
	name, _ := os.Hostname()
	expected := []NodeConfiguration{
		{
			NodeID:      node1,
			Addresses:   []string{"dynamic"},
			Compression: true,
		},
		{
			NodeID:      node2,
			Addresses:   []string{"dynamic"},
			Compression: true,
		},
		{
			NodeID:      node3,
			Addresses:   []string{"dynamic"},
			Compression: true,
		},
		{
			NodeID:    node4,
			Name:      name, // Set when auto created
			Addresses: []string{"dynamic"},
		},
	}

	cfg, err := Load("testdata/nodeaddressesdynamic.xml", node4)
	if err != nil {
		t.Error(err)
	}

	if !reflect.DeepEqual(cfg.Nodes, expected) {
		t.Errorf("Nodes differ;\n  E: %#v\n  A: %#v", expected, cfg.Nodes)
	}
}

func TestNodeAddressesStatic(t *testing.T) {
	name, _ := os.Hostname()
	expected := []NodeConfiguration{
		{
			NodeID:    node1,
			Addresses: []string{"192.0.2.1", "192.0.2.2"},
		},
		{
			NodeID:    node2,
			Addresses: []string{"192.0.2.3:6070", "[2001:db8::42]:4242"},
		},
		{
			NodeID:    node3,
			Addresses: []string{"[2001:db8::44]:4444", "192.0.2.4:6090"},
		},
		{
			NodeID:    node4,
			Name:      name, // Set when auto created
			Addresses: []string{"dynamic"},
		},
	}

	cfg, err := Load("testdata/nodeaddressesstatic.xml", node4)
	if err != nil {
		t.Error(err)
	}

	if !reflect.DeepEqual(cfg.Nodes, expected) {
		t.Errorf("Nodes differ;\n  E: %#v\n  A: %#v", expected, cfg.Nodes)
	}
}

func TestVersioningConfig(t *testing.T) {
	cfg, err := Load("testdata/versioningconfig.xml", node4)
	if err != nil {
		t.Error(err)
	}

	vc := cfg.Repositories[0].Versioning
	if vc.Type != "simple" {
		t.Errorf(`vc.Type %q != "simple"`, vc.Type)
	}
	if l := len(vc.Params); l != 2 {
		t.Errorf("len(vc.Params) %d != 2", l)
	}

	expected := map[string]string{
		"foo": "bar",
		"baz": "quux",
	}
	if !reflect.DeepEqual(vc.Params, expected) {
		t.Errorf("vc.Params differ;\n  E: %#v\n  A: %#v", expected, vc.Params)
	}
}

func TestNewSaveLoad(t *testing.T) {
	path := "testdata/temp.xml"
	os.Remove(path)

	exists := func(path string) bool {
		_, err := os.Stat(path)
		return err == nil
	}

	cfg := New(path, node1)

	// To make the equality pass later
	cfg.XMLName.Local = "configuration"

	if exists(path) {
		t.Error(path, "exists")
	}

	err := cfg.Save()
	if err != nil {
		t.Error(err)
	}
	if !exists(path) {
		t.Error(path, "does not exist")
	}

	cfg2, err := Load(path, node1)
	if err != nil {
		t.Error(err)
	}

	if !reflect.DeepEqual(cfg, cfg2) {
		t.Errorf("Configs are not equal;\n  E:  %#v\n  A:  %#v", cfg, cfg2)
	}

	cfg.GUI.User = "test"
	cfg.Save()

	cfg2, err = Load(path, node1)
	if err != nil {
		t.Error(err)
	}

	if cfg2.GUI.User != "test" || !reflect.DeepEqual(cfg, cfg2) {
		t.Errorf("Configs are not equal;\n  E:  %#v\n  A:  %#v", cfg, cfg2)
	}

	os.Remove(path)
}

func TestPrepare(t *testing.T) {
	var cfg Configuration

	if cfg.Repositories != nil || cfg.Nodes != nil || cfg.Options.ListenAddress != nil {
		t.Error("Expected nil")
	}

	cfg.prepare(node1)

	if cfg.Repositories == nil || cfg.Nodes == nil || cfg.Options.ListenAddress == nil {
		t.Error("Unexpected nil")
	}
}
