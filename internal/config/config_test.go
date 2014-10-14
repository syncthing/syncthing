// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

package config

import (
	"fmt"
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
		ListenAddress:        []string{"0.0.0.0:22000"},
		GlobalAnnServer:      "announce.syncthing.net:22026",
		GlobalAnnEnabled:     true,
		LocalAnnEnabled:      true,
		LocalAnnPort:         21025,
		LocalAnnMCAddr:       "[ff32::5222]:21026",
		MaxSendKbps:          0,
		MaxRecvKbps:          0,
		ReconnectIntervalS:   60,
		StartBrowser:         true,
		UPnPEnabled:          true,
		UPnPLease:            0,
		UPnPRenewal:          30,
		RestartOnWakeup:      true,
		AutoUpgradeIntervalH: 12,
		KeepTemporariesH:     24,
		CacheIgnoredFiles:    true,
	}

	cfg := New(device1)

	if !reflect.DeepEqual(cfg.Options, expected) {
		t.Errorf("Default config differs;\n  E: %#v\n  A: %#v", expected, cfg.Options)
	}
}

func TestDeviceConfig(t *testing.T) {
	for i := 1; i <= CurrentVersion; i++ {
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
				Path:            "testdata/",
				Devices:         []FolderDeviceConfiguration{{DeviceID: device1}, {DeviceID: device4}},
				ReadOnly:        true,
				RescanIntervalS: 600,
			},
		}
		expectedDevices := []DeviceConfiguration{
			{
				DeviceID:    device1,
				Name:        "node one",
				Addresses:   []string{"a"},
				Compression: true,
			},
			{
				DeviceID:    device4,
				Name:        "node two",
				Addresses:   []string{"b"},
				Compression: true,
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
		ListenAddress:        []string{":23000"},
		GlobalAnnServer:      "syncthing.nym.se:22026",
		GlobalAnnEnabled:     false,
		LocalAnnEnabled:      false,
		LocalAnnPort:         42123,
		LocalAnnMCAddr:       "quux:3232",
		MaxSendKbps:          1234,
		MaxRecvKbps:          2341,
		ReconnectIntervalS:   6000,
		StartBrowser:         false,
		UPnPEnabled:          false,
		UPnPLease:            60,
		UPnPRenewal:          15,
		RestartOnWakeup:      false,
		AutoUpgradeIntervalH: 24,
		KeepTemporariesH:     48,
		CacheIgnoredFiles:    false,
	}

	cfg, err := Load("testdata/overridenvalues.xml", device1)
	if err != nil {
		t.Error(err)
	}

	if !reflect.DeepEqual(cfg.Options(), expected) {
		t.Errorf("Overridden config differs;\n  E: %#v\n  A: %#v", expected, cfg.Options)
	}
}

func TestDeviceAddressesDynamic(t *testing.T) {
	name, _ := os.Hostname()
	expected := map[protocol.DeviceID]DeviceConfiguration{
		device1: {
			DeviceID:    device1,
			Addresses:   []string{"dynamic"},
			Compression: true,
		},
		device2: {
			DeviceID:    device2,
			Addresses:   []string{"dynamic"},
			Compression: true,
		},
		device3: {
			DeviceID:    device3,
			Addresses:   []string{"dynamic"},
			Compression: true,
		},
		device4: {
			DeviceID:  device4,
			Name:      name, // Set when auto created
			Addresses: []string{"dynamic"},
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

func TestDeviceAddressesStatic(t *testing.T) {
	name, _ := os.Hostname()
	expected := map[protocol.DeviceID]DeviceConfiguration{
		device1: {
			DeviceID:  device1,
			Addresses: []string{"192.0.2.1", "192.0.2.2"},
		},
		device2: {
			DeviceID:  device2,
			Addresses: []string{"192.0.2.3:6070", "[2001:db8::42]:4242"},
		},
		device3: {
			DeviceID:  device3,
			Addresses: []string{"[2001:db8::44]:4444", "192.0.2.4:6090"},
		},
		device4: {
			DeviceID:  device4,
			Name:      name, // Set when auto created
			Addresses: []string{"dynamic"},
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
		ID:   "t1",
		Path: "t1",
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
	newCfg.Folders[0].Path = "different"
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
