package main

import (
	"bytes"
	"io"
	"reflect"
	"testing"
)

func TestDefaultValues(t *testing.T) {
	expected := OptionsConfiguration{
		ListenAddress:      []string{":22000"},
		ReadOnly:           false,
		FollowSymlinks:     true,
		GUIEnabled:         true,
		GUIAddress:         "127.0.0.1:8080",
		GlobalAnnServer:    "announce.syncthing.net:22025",
		GlobalAnnEnabled:   true,
		LocalAnnEnabled:    true,
		ParallelRequests:   16,
		MaxSendKbps:        0,
		RescanIntervalS:    60,
		ReconnectIntervalS: 60,
		MaxChangeKbps:      1000,
		StartBrowser:       true,
	}

	cfg, err := readConfigXML(bytes.NewReader(nil))
	if err != io.EOF {
		t.Error(err)
	}

	if !reflect.DeepEqual(cfg.Options, expected) {
		t.Errorf("Default config differs;\n  E: %#v\n  A: %#v", expected, cfg.Options)
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

	cfg, err := readConfigXML(bytes.NewReader(data))
	if err != nil {
		t.Error(err)
	}

	expected := []string{""}
	if !reflect.DeepEqual(cfg.Options.ListenAddress, expected) {
		t.Errorf("Unexpected ListenAddress %#v", cfg.Options.ListenAddress)
	}
}

func TestOverriddenValues(t *testing.T) {
	data := []byte(`<configuration version="1">
    <repository directory="~/Sync">
        <node id="..." name="...">
            <address>dynamic</address>
        </node>
    </repository>
    <options>
       <listenAddress>:23000</listenAddress>
        <readOnly>true</readOnly>
        <allowDelete>false</allowDelete>
        <followSymlinks>false</followSymlinks>
        <guiEnabled>false</guiEnabled>
        <guiAddress>125.2.2.2:8080</guiAddress>
        <globalAnnounceServer>syncthing.nym.se:22025</globalAnnounceServer>
        <globalAnnounceEnabled>false</globalAnnounceEnabled>
        <localAnnounceEnabled>false</localAnnounceEnabled>
        <parallelRequests>32</parallelRequests>
        <maxSendKbps>1234</maxSendKbps>
        <rescanIntervalS>600</rescanIntervalS>
        <reconnectionIntervalS>6000</reconnectionIntervalS>
        <maxChangeKbps>2345</maxChangeKbps>
        <startBrowser>false</startBrowser>
    </options>
</configuration>
`)

	expected := OptionsConfiguration{
		ListenAddress:      []string{":23000"},
		ReadOnly:           true,
		FollowSymlinks:     false,
		GUIEnabled:         false,
		GUIAddress:         "125.2.2.2:8080",
		GlobalAnnServer:    "syncthing.nym.se:22025",
		GlobalAnnEnabled:   false,
		LocalAnnEnabled:    false,
		ParallelRequests:   32,
		MaxSendKbps:        1234,
		RescanIntervalS:    600,
		ReconnectIntervalS: 6000,
		MaxChangeKbps:      2345,
		StartBrowser:       false,
	}

	cfg, err := readConfigXML(bytes.NewReader(data))
	if err != nil {
		t.Error(err)
	}

	if !reflect.DeepEqual(cfg.Options, expected) {
		t.Errorf("Overridden config differs;\n  E: %#v\n  A: %#v", expected, cfg.Options)
	}
}
