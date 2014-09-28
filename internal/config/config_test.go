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

var device1, device2, device3, device4 protocol.DeviceID

func init() {
	device1, _ = protocol.DeviceIDFromString("AIR6LPZ7K4PTTUXQSMUUCPQ5YWOEDFIIQJUG7772YQXXR5YD6AWQ")
	device2, _ = protocol.DeviceIDFromString("GYRZZQB-IRNPV4Z-T7TC52W-EQYJ3TT-FDQW6MW-DFLMU42-SSSU6EM-FBK2VAY")
	device3, _ = protocol.DeviceIDFromString("LGFPDIT-7SKNNJL-VJZA4FC-7QNCRKA-CE753K7-2BW5QDK-2FOZ7FR-FEP57QJ")
	device4, _ = protocol.DeviceIDFromString("P56IOI7-MZJNU2Y-IQGDREY-DM2MGTI-MGL3BXN-PQ6W5BM-TBBZ4TJ-XZWICQ2")
}

func TestDefaultValues(t *testing.T) {
	expected := OptionsConfiguration{
		ListenAddress:      []string{"0.0.0.0:22000"},
		GlobalAnnServer:    "announce.syncthing.net:22026",
		GlobalAnnEnabled:   true,
		LocalAnnEnabled:    true,
		LocalAnnPort:       21025,
		LocalAnnMCAddr:     "[ff32::5222]:21026",
		MaxSendKbps:        0,
		MaxRecvKbps:        0,
		ReconnectIntervalS: 60,
		StartBrowser:       true,
		UPnPEnabled:        true,
		UPnPLease:          0,
		UPnPRenewal:        30,
		RestartOnWakeup:    true,
	}

	cfg := New("test", device1)

	if !reflect.DeepEqual(cfg.Options, expected) {
		t.Errorf("Default config differs;\n  E: %#v\n  A: %#v", expected, cfg.Options)
	}
}

func TestDeviceConfig(t *testing.T) {
	for i, ver := range []string{"v1", "v2", "v3", "v4"} {
		cfg, err := Load("testdata/"+ver+".xml", device1)
		if err != nil {
			t.Error(err)
		}

		expectedFolders := []FolderConfiguration{
			{
				ID:              "test",
				Directory:       "~/Sync",
				Devices:           []FolderDeviceConfiguration{{DeviceID: device1}, {DeviceID: device4}},
				ReadOnly:        true,
				RescanIntervalS: 600,
			},
		}
		expectedDevices := []DeviceConfiguration{
			{
				DeviceID:      device1,
				Name:        "device one",
				Addresses:   []string{"a"},
				Compression: true,
			},
			{
				DeviceID:      device4,
				Name:        "device two",
				Addresses:   []string{"b"},
				Compression: true,
			},
		}
		expectedDeviceIDs := []protocol.DeviceID{device1, device4}

		if cfg.Version != 4 {
			t.Errorf("%d: Incorrect version %d != 3", i, cfg.Version)
		}
		if !reflect.DeepEqual(cfg.Folders, expectedFolders) {
			t.Errorf("%d: Incorrect Folders\n  A: %#v\n  E: %#v", i, cfg.Folders, expectedFolders)
		}
		if !reflect.DeepEqual(cfg.Devices, expectedDevices) {
			t.Errorf("%d: Incorrect Devices\n  A: %#v\n  E: %#v", i, cfg.Devices, expectedDevices)
		}
		if !reflect.DeepEqual(cfg.Folders[0].DeviceIDs(), expectedDeviceIDs) {
			t.Errorf("%d: Incorrect DeviceIDs\n  A: %#v\n  E: %#v", i, cfg.Folders[0].DeviceIDs(), expectedDeviceIDs)
		}

		if len(cfg.DeviceMap()) != len(expectedDevices) {
			t.Errorf("Unexpected number of DeviceMap() entries")
		}
		if len(cfg.FolderMap()) != len(expectedFolders) {
			t.Errorf("Unexpected number of FolderMap() entries")
		}
	}
}

func TestNoListenAddress(t *testing.T) {
	cfg, err := Load("testdata/nolistenaddress.xml", device1)
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
		ListenAddress:      []string{":23000"},
		GlobalAnnServer:    "syncthing.nym.se:22026",
		GlobalAnnEnabled:   false,
		LocalAnnEnabled:    false,
		LocalAnnPort:       42123,
		LocalAnnMCAddr:     "quux:3232",
		MaxSendKbps:        1234,
		MaxRecvKbps:        2341,
		ReconnectIntervalS: 6000,
		StartBrowser:       false,
		UPnPEnabled:        false,
		UPnPLease:          60,
		UPnPRenewal:        15,
		RestartOnWakeup:    false,
	}

	cfg, err := Load("testdata/overridenvalues.xml", device1)
	if err != nil {
		t.Error(err)
	}

	if !reflect.DeepEqual(cfg.Options, expected) {
		t.Errorf("Overridden config differs;\n  E: %#v\n  A: %#v", expected, cfg.Options)
	}
}

func TestDeviceAddressesDynamic(t *testing.T) {
	name, _ := os.Hostname()
	expected := []DeviceConfiguration{
		{
			DeviceID:      device1,
			Addresses:   []string{"dynamic"},
			Compression: true,
		},
		{
			DeviceID:      device2,
			Addresses:   []string{"dynamic"},
			Compression: true,
		},
		{
			DeviceID:      device3,
			Addresses:   []string{"dynamic"},
			Compression: true,
		},
		{
			DeviceID:    device4,
			Name:      name, // Set when auto created
			Addresses: []string{"dynamic"},
		},
	}

	cfg, err := Load("testdata/deviceaddressesdynamic.xml", device4)
	if err != nil {
		t.Error(err)
	}

	if !reflect.DeepEqual(cfg.Devices, expected) {
		t.Errorf("Devices differ;\n  E: %#v\n  A: %#v", expected, cfg.Devices)
	}
}

func TestDeviceAddressesStatic(t *testing.T) {
	name, _ := os.Hostname()
	expected := []DeviceConfiguration{
		{
			DeviceID:    device1,
			Addresses: []string{"192.0.2.1", "192.0.2.2"},
		},
		{
			DeviceID:    device2,
			Addresses: []string{"192.0.2.3:6070", "[2001:db8::42]:4242"},
		},
		{
			DeviceID:    device3,
			Addresses: []string{"[2001:db8::44]:4444", "192.0.2.4:6090"},
		},
		{
			DeviceID:    device4,
			Name:      name, // Set when auto created
			Addresses: []string{"dynamic"},
		},
	}

	cfg, err := Load("testdata/deviceaddressesstatic.xml", device4)
	if err != nil {
		t.Error(err)
	}

	if !reflect.DeepEqual(cfg.Devices, expected) {
		t.Errorf("Devices differ;\n  E: %#v\n  A: %#v", expected, cfg.Devices)
	}
}

func TestVersioningConfig(t *testing.T) {
	cfg, err := Load("testdata/versioningconfig.xml", device4)
	if err != nil {
		t.Error(err)
	}

	vc := cfg.Folders[0].Versioning
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

	cfg := New(path, device1)

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

	cfg2, err := Load(path, device1)
	if err != nil {
		t.Error(err)
	}

	if !reflect.DeepEqual(cfg, cfg2) {
		t.Errorf("Configs are not equal;\n  E:  %#v\n  A:  %#v", cfg, cfg2)
	}

	cfg.GUI.User = "test"
	cfg.Save()

	cfg2, err = Load(path, device1)
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

	if cfg.Folders != nil || cfg.Devices != nil || cfg.Options.ListenAddress != nil {
		t.Error("Expected nil")
	}

	cfg.prepare(device1)

	if cfg.Folders == nil || cfg.Devices == nil || cfg.Options.ListenAddress == nil {
		t.Error("Unexpected nil")
	}
}
