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

	"github.com/syncthing/syncthing/protocol"
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
        <node id="P56IOI7MZJNU2IQGDREYDM2MGTMGL3BXNPQ6W5BTBBZ4TJXZWICQ"/>
        <node id="AIR6LPZ7K4PTTUXQSMUUCPQ5YWOEDFIIQJUG7772YQXXR5YD6AWQ"/>
        <node id="C4YBIESWDUAIGU62GOSRXCRAAJDWVE3TKCPMURZE2LH5QHAF576A"/>
        <node id="P56IOI7MZJNU2IQGDREYDM2MGTMGL3BXNPQ6W5BTBBZ4TJXZWICQ"/>
        <node id="AIR6LPZ7K4PTTUXQSMUUCPQ5YWOEDFIIQJUG7772YQXXR5YD6AWQ"/>
        <node id="C4YBIESWDUAIGU62GOSRXCRAAJDWVE3TKCPMURZE2LH5QHAF576A"/>
    </repository>
    <node id="AIR6LPZ7K4PTTUXQSMUUCPQ5YWOEDFIIQJUG7772YQXXR5YD6AWQ" name="node one">
        <address>a</address>
    </node>
    <node id="P56IOI7MZJNU2IQGDREYDM2MGTMGL3BXNPQ6W5BTBBZ4TJXZWICQ" name="node two">
        <address>b</address>
    </node>
</configuration>
`)

	v3data := []byte(`
<configuration version="3">
    <repository id="test" directory="~/Sync" ro="true" ignorePerms="false">
        <node id="AIR6LPZ-7K4PTTV-UXQSMUU-CPQ5YWH-OEDFIIQ-JUG777G-2YQXXR5-YD6AWQR" compression="false"></node>
        <node id="P56IOI7-MZJNU2Y-IQGDREY-DM2MGTI-MGL3BXN-PQ6W5BM-TBBZ4TJ-XZWICQ2" compression="false"></node>
    </repository>
    <node id="AIR6LPZ-7K4PTTV-UXQSMUU-CPQ5YWH-OEDFIIQ-JUG777G-2YQXXR5-YD6AWQR" name="node one" compression="true">
        <address>a</address>
    </node>
    <node id="P56IOI7-MZJNU2Y-IQGDREY-DM2MGTI-MGL3BXN-PQ6W5BM-TBBZ4TJ-XZWICQ2" name="node two" compression="true">
        <address>b</address>
    </node>
</configuration>`)

	for i, data := range [][]byte{v1data, v2data, v3data} {
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

		if cfg.Version != 3 {
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

func TestNodeAddressesDynamic(t *testing.T) {
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

	cfg, err := Load(bytes.NewReader(data), node4)
	if err != nil {
		t.Error(err)
	}

	if !reflect.DeepEqual(cfg.Nodes, expected) {
		t.Errorf("Nodes differ;\n  E: %#v\n  A: %#v", expected, cfg.Nodes)
	}
}

func TestNodeAddressesStatic(t *testing.T) {
	data := []byte(`
<configuration version="3">
    <node id="AIR6LPZ7K4PTTUXQSMUUCPQ5YWOEDFIIQJUG7772YQXXR5YD6AWQ">
        <address>192.0.2.1</address>
        <address>192.0.2.2</address>
    </node>
    <node id="GYRZZQBIRNPV4T7TC52WEQYJ3TFDQW6MWDFLMU4SSSU6EMFBK2VA">
        <address>192.0.2.3:6070</address>
        <address>[2001:db8::42]:4242</address>
    </node>
    <node id="LGFPDIT7SKNNJVJZA4FC7QNCRKCE753K72BW5QD2FOZ7FRFEP57Q">
        <address>[2001:db8::44]:4444</address>
        <address>192.0.2.4:6090</address>
    </node>
</configuration>
`)

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

	cfg, err := Load(bytes.NewReader(data), node4)
	if err != nil {
		t.Error(err)
	}

	if !reflect.DeepEqual(cfg.Nodes, expected) {
		t.Errorf("Nodes differ;\n  E: %#v\n  A: %#v", expected, cfg.Nodes)
	}
}

func TestVersioningConfig(t *testing.T) {
	data := []byte(`
		<configuration version="2">
			<repository id="test" directory="~/Sync" ro="true">
				<versioning type="simple">
					<param key="foo" val="bar"/>
					<param key="baz" val="quux"/>
				</versioning>
			</repository>
		</configuration>
		`)

	cfg, err := Load(bytes.NewReader(data), node4)
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
