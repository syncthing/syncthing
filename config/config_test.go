// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package config

import (
	"bytes"
	"io"
	"os"
	"reflect"
	"testing"

	"github.com/calmh/syncthing/protocol"
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
		ListenAddress:      []string{"0.0.0.0:22000"},
		GlobalAnnServer:    "announce.syncthing.net:22026",
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

	cfg, err := Load(bytes.NewReader(nil), node1)
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
        <node id="AIR6LPZ7K4PTTUXQSMUUCPQ5YWOEDFIIQJUG7772YQXXR5YD6AWQ" name="node one">
            <address>a</address>
        </node>
        <node id="P56IOI7MZJNU2IQGDREYDM2MGTMGL3BXNPQ6W5BTBBZ4TJXZWICQ" name="node two">
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
        <node id="AIR6LPZ7K4PTTUXQSMUUCPQ5YWOEDFIIQJUG7772YQXXR5YD6AWQ"/>
        <node id="P56IOI7MZJNU2IQGDREYDM2MGTMGL3BXNPQ6W5BTBBZ4TJXZWICQ"/>
    </repository>
    <node id="AIR6LPZ7K4PTTUXQSMUUCPQ5YWOEDFIIQJUG7772YQXXR5YD6AWQ" name="node one">
        <address>a</address>
    </node>
    <node id="P56IOI7MZJNU2IQGDREYDM2MGTMGL3BXNPQ6W5BTBBZ4TJXZWICQ" name="node two">
        <address>b</address>
    </node>
</configuration>
`)

	for i, data := range [][]byte{v1data, v2data} {
		cfg, err := Load(bytes.NewReader(data), node1)
		if err != nil {
			t.Error(err)
		}

		expectedRepos := []RepositoryConfiguration{
			{
				ID:        "test",
				Directory: "~/Sync",
				Nodes:     []NodeConfiguration{{NodeID: node1}, {NodeID: node4}},
				ReadOnly:  true,
			},
		}
		expectedNodes := []NodeConfiguration{
			{
				NodeID:    node1,
				Name:      "node one",
				Addresses: []string{"a"},
			},
			{
				NodeID:    node4,
				Name:      "node two",
				Addresses: []string{"b"},
			},
		}
		expectedNodeIDs := []protocol.NodeID{node1, node4}

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
    <options>
        <listenAddress></listenAddress>
    </options>
</configuration>
`)

	cfg, err := Load(bytes.NewReader(data), node1)
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
    <options>
       <listenAddress>:23000</listenAddress>
        <allowDelete>false</allowDelete>
        <globalAnnounceServer>syncthing.nym.se:22026</globalAnnounceServer>
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
		GlobalAnnServer:    "syncthing.nym.se:22026",
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

	cfg, err := Load(bytes.NewReader(data), node1)
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
    <node id="AIR6LPZ7K4PTTUXQSMUUCPQ5YWOEDFIIQJUG7772YQXXR5YD6AWQ">
        <address></address>
    </node>
    <node id="GYRZZQBIRNPV4T7TC52WEQYJ3TFDQW6MWDFLMU4SSSU6EMFBK2VA">
    </node>
    <node id="LGFPDIT7SKNNJVJZA4FC7QNCRKCE753K72BW5QD2FOZ7FRFEP57Q">
        <address>dynamic</address>
    </node>
</configuration>
`)

	name, _ := os.Hostname()
	expected := []NodeConfiguration{
		{
			NodeID:    node1,
			Addresses: []string{"dynamic"},
		},
		{
			NodeID:    node2,
			Addresses: []string{"dynamic"},
		},
		{
			NodeID:    node3,
			Addresses: []string{"dynamic"},
		},
		{
			NodeID:    node4,
			Name:      name, // Set when auto created
			Addresses: []string{"dynamic"},
		},
	}

	cfg, err := Load(bytes.NewReader(data), node4)
	if err != nil {
		t.Error(err)
	}

	if !reflect.DeepEqual(cfg.Nodes, expected) {
		t.Errorf("Nodes differ;\n  E: %#v\n  A: %#v", expected, cfg.Nodes)
	}
}
