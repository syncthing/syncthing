// Copyright (C) 2014 Jakob Borg and other contributors. All rights reserved.
// Use of this source code is governed by an MIT-style license that can be
// found in the LICENSE file.

package config

import (
	"bytes"
	"io"
	"os"
	"reflect"
	"testing"

	"github.com/calmh/syncthing/files"
	"github.com/calmh/syncthing/scanner"
)

func TestDefaultValues(t *testing.T) {
	expected := OptionsConfiguration{
		ListenAddress:      []string{"0.0.0.0:22000"},
		GlobalAnnServer:    "announce.syncthing.net:22025",
		GlobalAnnEnabled:   true,
		LocalAnnEnabled:    true,
		LocalAnnPort:       21025,
		ParallelRequests:   16,
		MaxSendKbps:        0,
		RescanIntervalS:    60,
		ReconnectIntervalS: 60,
		MaxChangeKbps:      10000,
		StartBrowser:       true,
		UPnPEnabled:        true,
	}

	cfg, err := Load(bytes.NewReader(nil), "nodeID")
	if err != io.EOF {
		t.Error(err)
	}

	if !reflect.DeepEqual(cfg.Options, expected) {
		t.Errorf("Default config differs;\n  E: %#v\n  A: %#v", expected, cfg.Options)
	}
}

func TestNodeConfig(t *testing.T) {
	v1data := []byte(`
<configuration version="1">
    <repository id="test" directory="~/Sync">
        <node id="NODE1" name="node one">
            <address>a</address>
        </node>
        <node id="NODE2" name="node two">
            <address>b</address>
        </node>
    </repository>
    <options>
        <readOnly>true</readOnly>
    </options>
</configuration>
`)

	v2data := []byte(`
<configuration version="2">
    <repository id="test" directory="~/Sync" ro="true">
        <node id="NODE1"/>
        <node id="NODE2"/>
    </repository>
    <node id="NODE1" name="node one">
        <address>a</address>
    </node>
    <node id="NODE2" name="node two">
        <address>b</address>
    </node>
</configuration>
`)

	for i, data := range [][]byte{v1data, v2data} {
		cfg, err := Load(bytes.NewReader(data), "NODE1")
		if err != nil {
			t.Error(err)
		}

		expectedRepos := []RepositoryConfiguration{
			{
				ID:        "test",
				Directory: "~/Sync",
				Nodes:     []NodeConfiguration{{NodeID: "NODE1"}, {NodeID: "NODE2"}},
				ReadOnly:  true,
			},
		}
		expectedNodes := []NodeConfiguration{
			{
				NodeID:    "NODE1",
				Name:      "node one",
				Addresses: []string{"a"},
			},
			{
				NodeID:    "NODE2",
				Name:      "node two",
				Addresses: []string{"b"},
			},
		}
		expectedNodeIDs := []string{"NODE1", "NODE2"}

		if cfg.Version != 2 {
			t.Errorf("%d: Incorrect version %d != 2", i, cfg.Version)
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
	}
}

func TestNoListenAddress(t *testing.T) {
	data := []byte(`<configuration version="1">
    <repository directory="~/Sync">
        <node id="..." name="...">
            <address>dynamic</address>
        </node>
    </repository>
    <options>
        <listenAddress></listenAddress>
    </options>
</configuration>
`)

	cfg, err := Load(bytes.NewReader(data), "nodeID")
	if err != nil {
		t.Error(err)
	}

	expected := []string{""}
	if !reflect.DeepEqual(cfg.Options.ListenAddress, expected) {
		t.Errorf("Unexpected ListenAddress %#v", cfg.Options.ListenAddress)
	}
}

func TestOverriddenValues(t *testing.T) {
	data := []byte(`<configuration version="2">
    <repository directory="~/Sync">
        <node id="..." name="...">
            <address>dynamic</address>
        </node>
    </repository>
    <options>
       <listenAddress>:23000</listenAddress>
        <allowDelete>false</allowDelete>
        <globalAnnounceServer>syncthing.nym.se:22025</globalAnnounceServer>
        <globalAnnounceEnabled>false</globalAnnounceEnabled>
        <localAnnounceEnabled>false</localAnnounceEnabled>
        <localAnnouncePort>42123</localAnnouncePort>
        <parallelRequests>32</parallelRequests>
        <maxSendKbps>1234</maxSendKbps>
        <rescanIntervalS>600</rescanIntervalS>
        <reconnectionIntervalS>6000</reconnectionIntervalS>
        <maxChangeKbps>2345</maxChangeKbps>
        <startBrowser>false</startBrowser>
        <upnpEnabled>false</upnpEnabled>
    </options>
</configuration>
`)

	expected := OptionsConfiguration{
		ListenAddress:      []string{":23000"},
		GlobalAnnServer:    "syncthing.nym.se:22025",
		GlobalAnnEnabled:   false,
		LocalAnnEnabled:    false,
		LocalAnnPort:       42123,
		ParallelRequests:   32,
		MaxSendKbps:        1234,
		RescanIntervalS:    600,
		ReconnectIntervalS: 6000,
		MaxChangeKbps:      2345,
		StartBrowser:       false,
		UPnPEnabled:        false,
	}

	cfg, err := Load(bytes.NewReader(data), "nodeID")
	if err != nil {
		t.Error(err)
	}

	if !reflect.DeepEqual(cfg.Options, expected) {
		t.Errorf("Overridden config differs;\n  E: %#v\n  A: %#v", expected, cfg.Options)
	}
}

func TestNodeAddresses(t *testing.T) {
	data := []byte(`
<configuration version="2">
    <node id="n1">
        <address>dynamic</address>
    </node>
    <node id="n2">
        <address></address>
    </node>
    <node id="n3">
    </node>
</configuration>
`)

	name, _ := os.Hostname()
	expected := []NodeConfiguration{
		{
			NodeID:    "N1",
			Addresses: []string{"dynamic"},
		},
		{
			NodeID:    "N2",
			Addresses: []string{"dynamic"},
		},
		{
			NodeID:    "N3",
			Addresses: []string{"dynamic"},
		},
		{
			NodeID:    "N4",
			Name:      name, // Set when auto created
			Addresses: []string{"dynamic"},
		},
	}

	cfg, err := Load(bytes.NewReader(data), "N4")
	if err != nil {
		t.Error(err)
	}

	if !reflect.DeepEqual(cfg.Nodes, expected) {
		t.Errorf("Nodes differ;\n  E: %#v\n  A: %#v", expected, cfg.Nodes)
	}
}

func TestStripNodeIs(t *testing.T) {
	data := []byte(`
<configuration version="2">
    <node id="AAAA-BBBB-CCCC">
        <address>dynamic</address>
    </node>
    <node id="AAAA BBBB DDDD">
        <address></address>
    </node>
    <node id="AAAABBBBEEEE">
        <address></address>
    </node>
    <repository directory="~/Sync">
        <node id="AAA ABBB-BCC CC" name=""></node>
        <node id="AA-AAB BBBD-DDD" name=""></node>
        <node id="AAA AB-BBB EEE-E" name=""></node>
    </repository>
</configuration>
`)

	expected := []NodeConfiguration{
		{
			NodeID:    "AAAABBBBCCCC",
			Addresses: []string{"dynamic"},
		},
		{
			NodeID:    "AAAABBBBDDDD",
			Addresses: []string{"dynamic"},
		},
		{
			NodeID:    "AAAABBBBEEEE",
			Addresses: []string{"dynamic"},
		},
	}

	cfg, err := Load(bytes.NewReader(data), "n4")
	if err != nil {
		t.Error(err)
	}

	for i := range expected {
		if !reflect.DeepEqual(cfg.Nodes[i], expected[i]) {
			t.Errorf("Nodes[%d] differ;\n  E: %#v\n  A: %#v", i, expected[i], cfg.Nodes[i])
		}
		if cfg.Repositories[0].Nodes[i].NodeID != expected[i].NodeID {
			t.Errorf("Repo nodes[%d] differ;\n  E: %#v\n  A: %#v", i, expected[i].NodeID, cfg.Repositories[0].Nodes[i].NodeID)
		}
	}
}

func TestSyncOrders(t *testing.T) {
	data := []byte(`
<configuration version="2">
    <node id="AAAA-BBBB-CCCC">
        <address>dynamic</address>
    </node>
    <repository directory="~/Sync">
        <syncorder>
            <pattern pattern="\.jpg$" priority="1" />
        </syncorder>
        <node id="AAAA-BBBB-CCCC" name=""></node>
    </repository>
</configuration>
`)

	expected := []SyncOrderPattern{
		{
			Pattern: "\\.jpg$",
			Priority:  1,
		},
	}

	cfg, err := Load(bytes.NewReader(data), "n4")
	if err != nil {
		t.Error(err)
	}

	for i := range expected {
		if !reflect.DeepEqual(cfg.Repositories[0].SyncOrderPatterns[i], expected[i]) {
			t.Errorf("Nodes[%d] differ;\n  E: %#v\n  A: %#v", i, expected[i], cfg.Repositories[0].SyncOrderPatterns[i])
		}
	}
}

func TestFileSorter(t *testing.T) {
	rcfg := RepositoryConfiguration{
		SyncOrderPatterns: []SyncOrderPattern{
			{"\\.jpg$", 10, nil},
			{"\\.mov$", 5, nil},
			{"^camera-uploads", 100, nil},
		},
	}

	f := []scanner.File{
		{Name: "bar.mov"},
		{Name: "baz.txt"},
		{Name: "foo.jpg"},
		{Name: "frew/foo.jpg"},
		{Name: "frew/lol.go"},
		{Name: "frew/rofl.copter"},
		{Name: "frew/bar.mov"},
		{Name: "camera-uploads/foo.jpg"},
		{Name: "camera-uploads/hurr.pl"},
		{Name: "camera-uploads/herp.mov"},
		{Name: "camera-uploads/wee.txt"},
	}

	files.SortBy(rcfg.FileRanker()).Sort(f)

	expected := []scanner.File{
		{Name: "camera-uploads/foo.jpg"},
		{Name: "camera-uploads/herp.mov"},
		{Name: "camera-uploads/hurr.pl"},
		{Name: "camera-uploads/wee.txt"},
		{Name: "foo.jpg"},
		{Name: "frew/foo.jpg"},
		{Name: "bar.mov"},
		{Name: "frew/bar.mov"},
		{Name: "frew/lol.go"},
		{Name: "baz.txt"},
		{Name: "frew/rofl.copter"},
	}

	if !reflect.DeepEqual(f, expected) {
		t.Errorf(
			"\n\nexpected:\n" +
			formatFiles(expected) + "\n" +
			"got:\n" +
			formatFiles(f) + "\n\n",
		)
	}
}

func formatFiles(f []scanner.File) string {
	ret := ""

	for _, v := range f {
		ret += "   " + v.Name + "\n"
	}

	return ret
}
