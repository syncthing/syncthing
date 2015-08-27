// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/syncthing/protocol"
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
		ListenAddress:           []string{"tcp://0.0.0.0:22000"},
		GlobalAnnServers:        []string{"udp4://announce.syncthing.net:22027", "udp6://announce-v6.syncthing.net:22027"},
		GlobalAnnEnabled:        true,
		LocalAnnEnabled:         true,
		LocalAnnPort:            21027,
		LocalAnnMCAddr:          "[ff12::8384]:21027",
		RelayServers:            []string{"dynamic+https://relays.syncthing.net"},
		MaxSendKbps:             0,
		MaxRecvKbps:             0,
		ReconnectIntervalS:      60,
		RelaysEnabled:           true,
		RelayReconnectIntervalM: 10,
		RelayWithoutGlobalAnn:   false,
		StartBrowser:            true,
		UPnPEnabled:             true,
		UPnPLeaseM:              60,
		UPnPRenewalM:            30,
		UPnPTimeoutS:            10,
		RestartOnWakeup:         true,
		AutoUpgradeIntervalH:    12,
		KeepTemporariesH:        24,
		CacheIgnoredFiles:       true,
		ProgressUpdateIntervalS: 5,
		SymlinksEnabled:         true,
		LimitBandwidthInLan:     false,
		DatabaseBlockCacheMiB:   0,
		PingTimeoutS:            30,
		PingIdleTimeS:           60,
		MinHomeDiskFreePct:      1,
	}

	cfg := New(device1)

	if !reflect.DeepEqual(cfg.Options, expected) {
		t.Errorf("Default config differs;\n  E: %#v\n  A: %#v", expected, cfg.Options)
	}
}

func TestDeviceConfig(t *testing.T) {
	for i := OldestHandledVersion; i <= CurrentVersion; i++ {
		os.Remove("testdata/.stfolder")
		wr, err := Load(fmt.Sprintf("testdata/v%d.xml", i), device1)
		if err != nil {
			t.Fatal(err)
		}

		_, err = os.Stat("testdata/.stfolder")
		if i < 6 && err != nil {
			t.Fatal(err)
		} else if i >= 6 && err == nil {
			t.Fatal("Unexpected file")
		}

		cfg := wr.cfg

		expectedFolders := []FolderConfiguration{
			{
				ID:              "test",
				RawPath:         "testdata",
				Devices:         []FolderDeviceConfiguration{{DeviceID: device1}, {DeviceID: device4}},
				ReadOnly:        true,
				RescanIntervalS: 600,
				Copiers:         0,
				Pullers:         0,
				Hashers:         0,
				AutoNormalize:   true,
				MinDiskFreePct:  1,
			},
		}
		expectedDevices := []DeviceConfiguration{
			{
				DeviceID:    device1,
				Name:        "node one",
				Addresses:   []string{"tcp://a"},
				Compression: protocol.CompressMetadata,
			},
			{
				DeviceID:    device4,
				Name:        "node two",
				Addresses:   []string{"tcp://b"},
				Compression: protocol.CompressMetadata,
			},
		}
		expectedDeviceIDs := []protocol.DeviceID{device1, device4}

		if cfg.Version != CurrentVersion {
			t.Errorf("%d: Incorrect version %d != %d", i, cfg.Version, CurrentVersion)
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
	}
}

func TestNoListenAddress(t *testing.T) {
	cfg, err := Load("testdata/nolistenaddress.xml", device1)
	if err != nil {
		t.Error(err)
	}

	expected := []string{""}
	actual := cfg.Options().ListenAddress
	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("Unexpected ListenAddress %#v", actual)
	}
}

func TestOverriddenValues(t *testing.T) {
	expected := OptionsConfiguration{
		ListenAddress:           []string{"tcp://:23000"},
		GlobalAnnServers:        []string{"udp4://syncthing.nym.se:22026"},
		GlobalAnnEnabled:        false,
		LocalAnnEnabled:         false,
		LocalAnnPort:            42123,
		LocalAnnMCAddr:          "quux:3232",
		RelayServers:            []string{"relay://123.123.123.123:1234", "relay://125.125.125.125:1255"},
		MaxSendKbps:             1234,
		MaxRecvKbps:             2341,
		ReconnectIntervalS:      6000,
		RelaysEnabled:           false,
		RelayReconnectIntervalM: 20,
		RelayWithoutGlobalAnn:   true,
		StartBrowser:            false,
		UPnPEnabled:             false,
		UPnPLeaseM:              90,
		UPnPRenewalM:            15,
		UPnPTimeoutS:            15,
		RestartOnWakeup:         false,
		AutoUpgradeIntervalH:    24,
		KeepTemporariesH:        48,
		CacheIgnoredFiles:       false,
		ProgressUpdateIntervalS: 10,
		SymlinksEnabled:         false,
		LimitBandwidthInLan:     true,
		DatabaseBlockCacheMiB:   42,
		PingTimeoutS:            60,
		PingIdleTimeS:           120,
		MinHomeDiskFreePct:      5,
	}

	cfg, err := Load("testdata/overridenvalues.xml", device1)
	if err != nil {
		t.Error(err)
	}

	if !reflect.DeepEqual(cfg.Options(), expected) {
		t.Errorf("Overridden config differs;\n  E: %#v\n  A: %#v", expected, cfg.Options())
	}
}

func TestDeviceAddressesDynamic(t *testing.T) {
	name, _ := os.Hostname()
	expected := map[protocol.DeviceID]DeviceConfiguration{
		device1: {
			DeviceID:  device1,
			Addresses: []string{"dynamic"},
		},
		device2: {
			DeviceID:  device2,
			Addresses: []string{"dynamic"},
		},
		device3: {
			DeviceID:  device3,
			Addresses: []string{"dynamic"},
		},
		device4: {
			DeviceID:    device4,
			Name:        name, // Set when auto created
			Addresses:   []string{"dynamic"},
			Compression: protocol.CompressMetadata,
		},
	}

	cfg, err := Load("testdata/deviceaddressesdynamic.xml", device4)
	if err != nil {
		t.Error(err)
	}

	actual := cfg.Devices()
	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("Devices differ;\n  E: %#v\n  A: %#v", expected, actual)
	}
}

func TestDeviceCompression(t *testing.T) {
	name, _ := os.Hostname()
	expected := map[protocol.DeviceID]DeviceConfiguration{
		device1: {
			DeviceID:    device1,
			Addresses:   []string{"dynamic"},
			Compression: protocol.CompressMetadata,
		},
		device2: {
			DeviceID:    device2,
			Addresses:   []string{"dynamic"},
			Compression: protocol.CompressMetadata,
		},
		device3: {
			DeviceID:    device3,
			Addresses:   []string{"dynamic"},
			Compression: protocol.CompressNever,
		},
		device4: {
			DeviceID:    device4,
			Name:        name, // Set when auto created
			Addresses:   []string{"dynamic"},
			Compression: protocol.CompressMetadata,
		},
	}

	cfg, err := Load("testdata/devicecompression.xml", device4)
	if err != nil {
		t.Error(err)
	}

	actual := cfg.Devices()
	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("Devices differ;\n  E: %#v\n  A: %#v", expected, actual)
	}
}

func TestDeviceAddressesStatic(t *testing.T) {
	name, _ := os.Hostname()
	expected := map[protocol.DeviceID]DeviceConfiguration{
		device1: {
			DeviceID:  device1,
			Addresses: []string{"tcp://192.0.2.1", "tcp://192.0.2.2"},
		},
		device2: {
			DeviceID:  device2,
			Addresses: []string{"tcp://192.0.2.3:6070", "tcp://[2001:db8::42]:4242"},
		},
		device3: {
			DeviceID:  device3,
			Addresses: []string{"tcp://[2001:db8::44]:4444", "tcp://192.0.2.4:6090"},
		},
		device4: {
			DeviceID:    device4,
			Name:        name, // Set when auto created
			Addresses:   []string{"dynamic"},
			Compression: protocol.CompressMetadata,
		},
	}

	cfg, err := Load("testdata/deviceaddressesstatic.xml", device4)
	if err != nil {
		t.Error(err)
	}

	actual := cfg.Devices()
	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("Devices differ;\n  E: %#v\n  A: %#v", expected, actual)
	}
}

func TestVersioningConfig(t *testing.T) {
	cfg, err := Load("testdata/versioningconfig.xml", device4)
	if err != nil {
		t.Error(err)
	}

	vc := cfg.Folders()["test"].Versioning
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

func TestIssue1262(t *testing.T) {
	cfg, err := Load("testdata/issue-1262.xml", device4)
	if err != nil {
		t.Fatal(err)
	}

	actual := cfg.Folders()["test"].RawPath
	expected := "e:"
	if runtime.GOOS == "windows" {
		expected = `e:\`
	}

	if actual != expected {
		t.Errorf("%q != %q", actual, expected)
	}
}

func TestIssue1750(t *testing.T) {
	cfg, err := Load("testdata/issue-1750.xml", device4)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Options().ListenAddress[0] != "tcp://:23000" {
		t.Errorf("%q != %q", cfg.Options().ListenAddress[0], "tcp://:23000")
	}

	if cfg.Options().ListenAddress[1] != "tcp://:23001" {
		t.Errorf("%q != %q", cfg.Options().ListenAddress[1], "tcp://:23001")
	}

	if cfg.Options().GlobalAnnServers[0] != "udp4://syncthing.nym.se:22026" {
		t.Errorf("%q != %q", cfg.Options().GlobalAnnServers[0], "udp4://syncthing.nym.se:22026")
	}

	if cfg.Options().GlobalAnnServers[1] != "udp4://syncthing.nym.se:22027" {
		t.Errorf("%q != %q", cfg.Options().GlobalAnnServers[1], "udp4://syncthing.nym.se:22027")
	}
}

func TestWindowsPaths(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Not useful on non-Windows")
		return
	}

	folder := FolderConfiguration{
		RawPath: `e:\`,
	}

	expected := `\\?\e:\`
	actual := folder.Path()
	if actual != expected {
		t.Errorf("%q != %q", actual, expected)
	}

	folder.RawPath = `\\192.0.2.22\network\share`
	expected = folder.RawPath
	actual = folder.Path()
	if actual != expected {
		t.Errorf("%q != %q", actual, expected)
	}

	folder.RawPath = `relative\path`
	expected = folder.RawPath
	actual = folder.Path()
	if actual == expected || !strings.HasPrefix(actual, "\\\\?\\") {
		t.Errorf("%q == %q, expected absolutification", actual, expected)
	}
}

func TestFolderPath(t *testing.T) {
	folder := FolderConfiguration{
		RawPath: "~/tmp",
	}

	realPath := folder.Path()
	if !filepath.IsAbs(realPath) {
		t.Error(realPath, "should be absolute")
	}
	if strings.Contains(realPath, "~") {
		t.Error(realPath, "should not contain ~")
	}
}

func TestNewSaveLoad(t *testing.T) {
	path := "testdata/temp.xml"
	os.Remove(path)

	exists := func(path string) bool {
		_, err := os.Stat(path)
		return err == nil
	}

	intCfg := New(device1)
	cfg := Wrap(path, intCfg)

	// To make the equality pass later
	cfg.cfg.XMLName.Local = "configuration"

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

	if !reflect.DeepEqual(cfg.Raw(), cfg2.Raw()) {
		t.Errorf("Configs are not equal;\n  E:  %#v\n  A:  %#v", cfg.Raw(), cfg2.Raw())
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

func TestRequiresRestart(t *testing.T) {
	wr, err := Load("testdata/v6.xml", device1)
	if err != nil {
		t.Fatal(err)
	}
	cfg := wr.cfg

	if ChangeRequiresRestart(cfg, cfg) {
		t.Error("No change does not require restart")
	}

	newCfg := cfg
	newCfg.Devices = append(newCfg.Devices, DeviceConfiguration{
		DeviceID: device3,
	})
	if ChangeRequiresRestart(cfg, newCfg) {
		t.Error("Adding a device does not require restart")
	}

	newCfg = cfg
	newCfg.Devices = newCfg.Devices[:len(newCfg.Devices)-1]
	if !ChangeRequiresRestart(cfg, newCfg) {
		t.Error("Removing a device requires restart")
	}

	newCfg = cfg
	newCfg.Folders = append(newCfg.Folders, FolderConfiguration{
		ID:      "t1",
		RawPath: "t1",
	})
	if !ChangeRequiresRestart(cfg, newCfg) {
		t.Error("Adding a folder requires restart")
	}

	newCfg = cfg
	newCfg.Folders = newCfg.Folders[:len(newCfg.Folders)-1]
	if !ChangeRequiresRestart(cfg, newCfg) {
		t.Error("Removing a folder requires restart")
	}

	newCfg = cfg
	newFolders := make([]FolderConfiguration, len(cfg.Folders))
	copy(newFolders, cfg.Folders)
	newCfg.Folders = newFolders
	if ChangeRequiresRestart(cfg, newCfg) {
		t.Error("No changes done yet")
	}
	newCfg.Folders[0].RawPath = "different"
	if !ChangeRequiresRestart(cfg, newCfg) {
		t.Error("Changing a folder requires restart")
	}

	newCfg = cfg
	newDevices := make([]DeviceConfiguration, len(cfg.Devices))
	copy(newDevices, cfg.Devices)
	newCfg.Devices = newDevices
	if ChangeRequiresRestart(cfg, newCfg) {
		t.Error("No changes done yet")
	}
	newCfg.Devices[0].Name = "different"
	if ChangeRequiresRestart(cfg, newCfg) {
		t.Error("Changing a device does not require restart")
	}

	newCfg = cfg
	newCfg.Options.GlobalAnnEnabled = !cfg.Options.GlobalAnnEnabled
	if !ChangeRequiresRestart(cfg, newCfg) {
		t.Error("Changing general options requires restart")
	}

	newCfg = cfg
	newCfg.GUI.UseTLS = !cfg.GUI.UseTLS
	if !ChangeRequiresRestart(cfg, newCfg) {
		t.Error("Changing GUI options requires restart")
	}
}

func TestCopy(t *testing.T) {
	wrapper, err := Load("testdata/example.xml", device1)
	if err != nil {
		t.Fatal(err)
	}
	cfg := wrapper.Raw()

	bsOrig, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	copy := cfg.Copy()

	cfg.Devices[0].Addresses[0] = "wrong"
	cfg.Folders[0].Devices[0].DeviceID = protocol.DeviceID{0, 1, 2, 3}
	cfg.Options.ListenAddress[0] = "wrong"
	cfg.GUI.APIKey = "wrong"

	bsChanged, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	bsCopy, err := json.MarshalIndent(copy, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	if bytes.Compare(bsOrig, bsChanged) == 0 {
		t.Error("Config should have changed")
	}
	if bytes.Compare(bsOrig, bsCopy) != 0 {
		//ioutil.WriteFile("a", bsOrig, 0644)
		//ioutil.WriteFile("b", bsCopy, 0644)
		t.Error("Copy should be unchanged")
	}
}

func TestPullOrder(t *testing.T) {
	wrapper, err := Load("testdata/pullorder.xml", device1)
	if err != nil {
		t.Fatal(err)
	}
	folders := wrapper.Folders()

	expected := []struct {
		name  string
		order PullOrder
	}{
		{"f1", OrderRandom},        // empty value, default
		{"f2", OrderRandom},        // explicit
		{"f3", OrderAlphabetic},    // explicit
		{"f4", OrderRandom},        // unknown value, default
		{"f5", OrderSmallestFirst}, // explicit
		{"f6", OrderLargestFirst},  // explicit
		{"f7", OrderOldestFirst},   // explicit
		{"f8", OrderNewestFirst},   // explicit
	}

	// Verify values are deserialized correctly

	for _, tc := range expected {
		if actual := folders[tc.name].Order; actual != tc.order {
			t.Errorf("Incorrect pull order for %q: %v != %v", tc.name, actual, tc.order)
		}
	}

	// Serialize and deserialize again to verify it survives the transformation

	buf := new(bytes.Buffer)
	cfg := wrapper.Raw()
	cfg.WriteXML(buf)

	t.Logf("%s", buf.Bytes())

	cfg, err = ReadXML(buf, device1)
	wrapper = Wrap("testdata/pullorder.xml", cfg)
	folders = wrapper.Folders()

	for _, tc := range expected {
		if actual := folders[tc.name].Order; actual != tc.order {
			t.Errorf("Incorrect pull order for %q: %v != %v", tc.name, actual, tc.order)
		}
	}
}

func TestLargeRescanInterval(t *testing.T) {
	wrapper, err := Load("testdata/largeinterval.xml", device1)
	if err != nil {
		t.Fatal(err)
	}

	if wrapper.Folders()["l1"].RescanIntervalS != MaxRescanIntervalS {
		t.Error("too large rescan interval should be maxed out")
	}
	if wrapper.Folders()["l2"].RescanIntervalS != 0 {
		t.Error("negative rescan interval should become zero")
	}
}
