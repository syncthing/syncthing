// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package config

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/d4l3k/messagediff"
	"golang.org/x/crypto/bcrypt"

	"github.com/syncthing/syncthing/lib/build"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/rand"
)

var device1, device2, device3, device4 protocol.DeviceID

var testFs fs.Filesystem

func init() {
	device1, _ = protocol.DeviceIDFromString("AIR6LPZ7K4PTTUXQSMUUCPQ5YWOEDFIIQJUG7772YQXXR5YD6AWQ")
	device2, _ = protocol.DeviceIDFromString("GYRZZQB-IRNPV4Z-T7TC52W-EQYJ3TT-FDQW6MW-DFLMU42-SSSU6EM-FBK2VAY")
	device3, _ = protocol.DeviceIDFromString("LGFPDIT-7SKNNJL-VJZA4FC-7QNCRKA-CE753K7-2BW5QDK-2FOZ7FR-FEP57QJ")
	device4, _ = protocol.DeviceIDFromString("P56IOI7-MZJNU2Y-IQGDREY-DM2MGTI-MGL3BXN-PQ6W5BM-TBBZ4TJ-XZWICQ2")

	testFs = fs.NewFilesystem(fs.FilesystemTypeFake, rand.String(32)+"?content=true")
	loadTestFiles()
}

func TestDefaultValues(t *testing.T) {
	size, err := ParseSize("1%")
	if err != nil {
		t.Fatal(err)
	}
	expected := Configuration{
		Version: CurrentVersion,
		Folders: []FolderConfiguration{},
		Options: OptionsConfiguration{
			RawListenAddresses:        []string{"default"},
			RawGlobalAnnServers:       []string{"default"},
			GlobalAnnEnabled:          true,
			LocalAnnEnabled:           true,
			LocalAnnPort:              21027,
			LocalAnnMCAddr:            "[ff12::8384]:21027",
			MaxSendKbps:               0,
			MaxRecvKbps:               0,
			ReconnectIntervalS:        60,
			RelaysEnabled:             true,
			RelayReconnectIntervalM:   10,
			StartBrowser:              true,
			NATEnabled:                true,
			NATLeaseM:                 60,
			NATRenewalM:               30,
			NATTimeoutS:               10,
			AutoUpgradeIntervalH:      12,
			KeepTemporariesH:          24,
			CacheIgnoredFiles:         false,
			ProgressUpdateIntervalS:   5,
			LimitBandwidthInLan:       false,
			MinHomeDiskFree:           Size{1, "%"},
			URURL:                     "https://data.syncthing.net/newdata",
			URInitialDelayS:           1800,
			URPostInsecurely:          false,
			ReleasesURL:               "https://upgrades.syncthing.net/meta.json",
			AlwaysLocalNets:           []string{},
			OverwriteRemoteDevNames:   false,
			TempIndexMinBlocks:        10,
			UnackedNotificationIDs:    []string{"authenticationUserAndPassword"},
			SetLowPriority:            true,
			CRURL:                     "https://crash.syncthing.net/newcrash",
			CREnabled:                 true,
			StunKeepaliveStartS:       180,
			StunKeepaliveMinS:         20,
			RawStunServers:            []string{"default"},
			AnnounceLANAddresses:      true,
			FeatureFlags:              []string{},
			ConnectionPriorityTCPLAN:  10,
			ConnectionPriorityQUICLAN: 20,
			ConnectionPriorityTCPWAN:  30,
			ConnectionPriorityQUICWAN: 40,
			ConnectionPriorityRelay:   50,
		},
		Defaults: Defaults{
			Folder: FolderConfiguration{
				FilesystemType:   FilesystemTypeBasic,
				Path:             "~",
				Type:             FolderTypeSendReceive,
				Devices:          []FolderDeviceConfiguration{{DeviceID: device1}},
				RescanIntervalS:  3600,
				FSWatcherEnabled: true,
				FSWatcherDelayS:  10,
				IgnorePerms:      false,
				AutoNormalize:    true,
				MinDiskFree:      size,
				Versioning: VersioningConfiguration{
					CleanupIntervalS: 3600,
					Params:           map[string]string{},
				},
				MaxConflicts:         10,
				WeakHashThresholdPct: 25,
				MarkerName:           ".stfolder",
				MaxConcurrentWrites:  2,
				XattrFilter: XattrFilter{
					Entries:            []XattrFilterEntry{},
					MaxSingleEntrySize: 1024,
					MaxTotalSize:       4096,
				},
			},
			Device: DeviceConfiguration{
				Addresses:       []string{"dynamic"},
				AllowedNetworks: []string{},
				Compression:     CompressionMetadata,
				IgnoredFolders:  []ObservedFolder{},
			},
			Ignores: Ignores{
				Lines: []string{},
			},
		},
		IgnoredDevices: []ObservedDevice{},
	}
	expected.Devices = []DeviceConfiguration{expected.Defaults.Device.Copy()}
	expected.Devices[0].DeviceID = device1
	expected.Devices[0].Name, _ = os.Hostname()

	cfg := New(device1)
	cfg.GUI = GUIConfiguration{}
	cfg.LDAP = LDAPConfiguration{}

	if diff, equal := messagediff.PrettyDiff(expected, cfg); !equal {
		t.Errorf("Default config differs. Diff:\n%s", diff)
	}
}

func TestDeviceConfig(t *testing.T) {
	for i := OldestHandledVersion; i <= CurrentVersion; i++ {
		cfgFile := fmt.Sprintf("v%d.xml", i)
		if _, err := testFs.Stat(cfgFile); os.IsNotExist(err) {
			continue
		}

		testFs.RemoveAll(DefaultMarkerName)
		wr, wrCancel, err := copyAndLoad(testFs, cfgFile, device1)
		defer wrCancel()
		if err != nil {
			t.Fatal(err)
		}

		_, err = testFs.Stat(DefaultMarkerName)
		if i < 6 && err != nil {
			t.Fatal(err)
		} else if i >= 6 && err == nil {
			t.Fatal("Unexpected file")
		}

		cfg := wr.Wrapper.(*wrapper).cfg

		expectedFolders := []FolderConfiguration{
			{
				ID:               "test",
				FilesystemType:   FilesystemTypeBasic,
				Path:             "testdata",
				Devices:          []FolderDeviceConfiguration{{DeviceID: device1}, {DeviceID: device4}},
				Type:             FolderTypeSendOnly,
				RescanIntervalS:  600,
				FSWatcherEnabled: false,
				FSWatcherDelayS:  10,
				Copiers:          0,
				Hashers:          0,
				AutoNormalize:    true,
				MinDiskFree:      Size{1, "%"},
				MaxConflicts:     -1,
				Versioning: VersioningConfiguration{
					Params: map[string]string{},
				},
				WeakHashThresholdPct: 25,
				MarkerName:           DefaultMarkerName,
				JunctionsAsDirs:      true,
				MaxConcurrentWrites:  maxConcurrentWritesDefault,
				XattrFilter: XattrFilter{
					Entries: []XattrFilterEntry{},
				},
			},
		}

		expectedDevices := []DeviceConfiguration{
			{
				DeviceID:        device1,
				Name:            "node one",
				Addresses:       []string{"tcp://a"},
				Compression:     CompressionMetadata,
				AllowedNetworks: []string{},
				IgnoredFolders:  []ObservedFolder{},
			},
			{
				DeviceID:        device4,
				Name:            "node two",
				Addresses:       []string{"tcp://b"},
				Compression:     CompressionMetadata,
				AllowedNetworks: []string{},
				IgnoredFolders:  []ObservedFolder{},
			},
		}
		expectedDeviceIDs := []protocol.DeviceID{device1, device4}

		if cfg.Version != CurrentVersion {
			t.Errorf("%d: Incorrect version %d != %d", i, cfg.Version, CurrentVersion)
		}
		if diff, equal := messagediff.PrettyDiff(expectedFolders, cfg.Folders); !equal {
			t.Errorf("%d: Incorrect Folders. Diff:\n%s\n%v", i, diff, cfg)
		}
		if diff, equal := messagediff.PrettyDiff(expectedDevices, cfg.Devices); !equal {
			t.Errorf("%d: Incorrect Devices. Diff:\n%s", i, diff)
		}
		if diff, equal := messagediff.PrettyDiff(expectedDeviceIDs, cfg.Folders[0].DeviceIDs()); !equal {
			t.Errorf("%d: Incorrect DeviceIDs. Diff:\n%s", i, diff)
		}
	}
}

func TestNoListenAddresses(t *testing.T) {
	cfg, cfgCancel, err := copyAndLoad(testFs, "nolistenaddress.xml", device1)
	defer cfgCancel()
	if err != nil {
		t.Fatal(err)
	}

	expected := []string{""}
	actual := cfg.Options().RawListenAddresses
	if diff, equal := messagediff.PrettyDiff(expected, actual); !equal {
		t.Errorf("Unexpected RawListenAddresses. Diff:\n%s", diff)
	}
}

func TestOverriddenValues(t *testing.T) {
	expected := OptionsConfiguration{
		RawListenAddresses:        []string{"tcp://:23000"},
		RawGlobalAnnServers:       []string{"udp4://syncthing.nym.se:22026"},
		GlobalAnnEnabled:          false,
		LocalAnnEnabled:           false,
		LocalAnnPort:              42123,
		LocalAnnMCAddr:            "quux:3232",
		MaxSendKbps:               1234,
		MaxRecvKbps:               2341,
		ReconnectIntervalS:        6000,
		RelaysEnabled:             false,
		RelayReconnectIntervalM:   20,
		StartBrowser:              false,
		NATEnabled:                false,
		NATLeaseM:                 90,
		NATRenewalM:               15,
		NATTimeoutS:               15,
		AutoUpgradeIntervalH:      24,
		KeepTemporariesH:          48,
		CacheIgnoredFiles:         true,
		ProgressUpdateIntervalS:   10,
		LimitBandwidthInLan:       true,
		MinHomeDiskFree:           Size{5.2, "%"},
		URSeen:                    8,
		URAccepted:                4,
		URURL:                     "https://localhost/newdata",
		URInitialDelayS:           800,
		URPostInsecurely:          true,
		ReleasesURL:               "https://localhost/releases",
		AlwaysLocalNets:           []string{},
		OverwriteRemoteDevNames:   true,
		TempIndexMinBlocks:        100,
		UnackedNotificationIDs:    []string{"asdfasdf"},
		SetLowPriority:            false,
		CRURL:                     "https://localhost/newcrash",
		CREnabled:                 false,
		StunKeepaliveStartS:       9000,
		StunKeepaliveMinS:         900,
		RawStunServers:            []string{"foo"},
		FeatureFlags:              []string{"feature"},
		ConnectionPriorityTCPLAN:  40,
		ConnectionPriorityQUICLAN: 45,
		ConnectionPriorityTCPWAN:  50,
		ConnectionPriorityQUICWAN: 55,
		ConnectionPriorityRelay:   9000,
	}
	expectedPath := "/media/syncthing"

	os.Unsetenv("STNOUPGRADE")
	cfg, cfgCancel, err := copyAndLoad(testFs, "overridenvalues.xml", device1)
	defer cfgCancel()
	if err != nil {
		t.Error(err)
	}

	if cfg.Options().URUniqueID == "" {
		t.Error("expected usage reporting unique ID to be set since usage reporting is enabled")
	}
	expected.URUniqueID = cfg.Options().URUniqueID // it's random

	if diff, equal := messagediff.PrettyDiff(expected, cfg.Options()); !equal {
		t.Errorf("Overridden config differs. Diff:\n%s", diff)
	}

	if path := cfg.DefaultFolder().Path; path != expectedPath {
		t.Errorf("Default folder path is %v, expected %v", path, expectedPath)
	}
}

func TestDeviceAddressesDynamic(t *testing.T) {
	name, _ := os.Hostname()
	expected := map[protocol.DeviceID]DeviceConfiguration{
		device1: {
			DeviceID:        device1,
			Addresses:       []string{"dynamic"},
			AllowedNetworks: []string{},
			IgnoredFolders:  []ObservedFolder{},
		},
		device2: {
			DeviceID:        device2,
			Addresses:       []string{"dynamic"},
			AllowedNetworks: []string{},
			IgnoredFolders:  []ObservedFolder{},
		},
		device3: {
			DeviceID:        device3,
			Addresses:       []string{"dynamic"},
			AllowedNetworks: []string{},
			IgnoredFolders:  []ObservedFolder{},
		},
		device4: {
			DeviceID:        device4,
			Name:            name, // Set when auto created
			Addresses:       []string{"dynamic"},
			Compression:     CompressionMetadata,
			AllowedNetworks: []string{},
			IgnoredFolders:  []ObservedFolder{},
		},
	}

	cfg, cfgCancel, err := copyAndLoad(testFs, "deviceaddressesdynamic.xml", device4)
	defer cfgCancel()
	if err != nil {
		t.Error(err)
	}

	actual := cfg.Devices()
	if diff, equal := messagediff.PrettyDiff(expected, actual); !equal {
		t.Errorf("Devices differ. Diff:\n%s", diff)
	}
}

func TestDeviceCompression(t *testing.T) {
	name, _ := os.Hostname()
	expected := map[protocol.DeviceID]DeviceConfiguration{
		device1: {
			DeviceID:        device1,
			Addresses:       []string{"dynamic"},
			Compression:     CompressionMetadata,
			AllowedNetworks: []string{},
			IgnoredFolders:  []ObservedFolder{},
		},
		device2: {
			DeviceID:        device2,
			Addresses:       []string{"dynamic"},
			Compression:     CompressionMetadata,
			AllowedNetworks: []string{},
			IgnoredFolders:  []ObservedFolder{},
		},
		device3: {
			DeviceID:        device3,
			Addresses:       []string{"dynamic"},
			Compression:     CompressionNever,
			AllowedNetworks: []string{},
			IgnoredFolders:  []ObservedFolder{},
		},
		device4: {
			DeviceID:        device4,
			Name:            name, // Set when auto created
			Addresses:       []string{"dynamic"},
			Compression:     CompressionMetadata,
			AllowedNetworks: []string{},
			IgnoredFolders:  []ObservedFolder{},
		},
	}

	cfg, cfgCancel, err := copyAndLoad(testFs, "devicecompression.xml", device4)
	defer cfgCancel()
	if err != nil {
		t.Error(err)
	}

	actual := cfg.Devices()
	if diff, equal := messagediff.PrettyDiff(expected, actual); !equal {
		t.Errorf("Devices differ. Diff:\n%s", diff)
	}
}

func TestDeviceAddressesStatic(t *testing.T) {
	name, _ := os.Hostname()
	expected := map[protocol.DeviceID]DeviceConfiguration{
		device1: {
			DeviceID:        device1,
			Addresses:       []string{"tcp://192.0.2.1", "tcp://192.0.2.2"},
			AllowedNetworks: []string{},
			IgnoredFolders:  []ObservedFolder{},
		},
		device2: {
			DeviceID:        device2,
			Addresses:       []string{"tcp://192.0.2.3:6070", "tcp://[2001:db8::42]:4242"},
			AllowedNetworks: []string{},
			IgnoredFolders:  []ObservedFolder{},
		},
		device3: {
			DeviceID:        device3,
			Addresses:       []string{"tcp://[2001:db8::44]:4444", "tcp://192.0.2.4:6090"},
			AllowedNetworks: []string{},
			IgnoredFolders:  []ObservedFolder{},
		},
		device4: {
			DeviceID:        device4,
			Name:            name, // Set when auto created
			Addresses:       []string{"dynamic"},
			Compression:     CompressionMetadata,
			AllowedNetworks: []string{},
			IgnoredFolders:  []ObservedFolder{},
		},
	}

	cfg, cfgCancel, err := copyAndLoad(testFs, "deviceaddressesstatic.xml", device4)
	defer cfgCancel()
	if err != nil {
		t.Error(err)
	}

	actual := cfg.Devices()
	if diff, equal := messagediff.PrettyDiff(expected, actual); !equal {
		t.Errorf("Devices differ. Diff:\n%s", diff)
	}
}

func TestVersioningConfig(t *testing.T) {
	cfg, cfgCancel, err := copyAndLoad(testFs, "versioningconfig.xml", device4)
	defer cfgCancel()
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
	if diff, equal := messagediff.PrettyDiff(expected, vc.Params); !equal {
		t.Errorf("vc.Params differ. Diff:\n%s", diff)
	}
}

func TestIssue1262(t *testing.T) {
	if !build.IsWindows {
		t.Skipf("path gets converted to absolute as part of the filesystem initialization on linux")
	}

	cfg, cfgCancel, err := copyAndLoad(testFs, "issue-1262.xml", device4)
	defer cfgCancel()
	if err != nil {
		t.Fatal(err)
	}

	actual := cfg.Folders()["test"].Filesystem(nil).URI()
	expected := `e:\`

	if actual != expected {
		t.Errorf("%q != %q", actual, expected)
	}
}

func TestIssue1750(t *testing.T) {
	cfg, cfgCancel, err := copyAndLoad(testFs, "issue-1750.xml", device4)
	defer cfgCancel()
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Options().RawListenAddresses[0] != "tcp://:23000" {
		t.Errorf("%q != %q", cfg.Options().RawListenAddresses[0], "tcp://:23000")
	}

	if cfg.Options().RawListenAddresses[1] != "tcp://:23001" {
		t.Errorf("%q != %q", cfg.Options().RawListenAddresses[1], "tcp://:23001")
	}

	if cfg.Options().RawGlobalAnnServers[0] != "udp4://syncthing.nym.se:22026" {
		t.Errorf("%q != %q", cfg.Options().RawGlobalAnnServers[0], "udp4://syncthing.nym.se:22026")
	}

	if cfg.Options().RawGlobalAnnServers[1] != "udp4://syncthing.nym.se:22027" {
		t.Errorf("%q != %q", cfg.Options().RawGlobalAnnServers[1], "udp4://syncthing.nym.se:22027")
	}
}

func TestFolderPath(t *testing.T) {
	folder := FolderConfiguration{
		Path: "~/tmp",
	}

	realPath := folder.Filesystem(nil).URI()
	if !filepath.IsAbs(realPath) {
		t.Error(realPath, "should be absolute")
	}
	if strings.Contains(realPath, "~") {
		t.Error(realPath, "should not contain ~")
	}
}

func TestFolderCheckPath(t *testing.T) {
	tmpFs := fs.NewFilesystem(fs.FilesystemTypeFake, rand.String(16)+"?nostfolder=true")
	_ = tmpFs.MkdirAll(filepath.Join("dir", ".stfolder"), 0o777)

	testcases := []struct {
		path string
		err  error
	}{
		{
			path: ".",
			err:  ErrMarkerMissing,
		},
		{
			path: "does not exist",
			err:  ErrPathMissing,
		},
		{
			path: "dir",
			err:  nil,
		},
	}

	for _, testcase := range testcases {
		cfg := FolderConfiguration{
			FilesystemType: FilesystemTypeFake,
			MarkerName:     DefaultMarkerName,
		}

		if err := cfg.checkFilesystemPath(tmpFs, testcase.path); testcase.err != err {
			t.Errorf("unexpected error in case; path: [%s] err [%s] expected err [%v]", testcase.path, err, testcase.err)
		}
	}
}

func TestNewSaveLoad(t *testing.T) {
	path := "temp.xml"
	os.Remove(path)
	defer os.Remove(path)

	exists := func(path string) bool {
		_, err := os.Stat(path)
		return err == nil
	}

	intCfg := New(device1)
	cfg := wrap(path, intCfg, device1)
	defer cfg.stop()

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

	cfg2, err := load(path, device1)
	defer cfg2.stop()
	if err != nil {
		t.Error(err)
	}

	if diff, equal := messagediff.PrettyDiff(cfg.RawCopy(), cfg2.RawCopy()); !equal {
		t.Errorf("Configs are not equal. Diff:\n%s", diff)
	}
}

func TestWindowsLineEndings(t *testing.T) {
	if !build.IsWindows {
		t.Skip("Windows specific")
	}

	dir := t.TempDir()

	path := filepath.Join(dir, "config.xml")
	os.Remove(path)
	defer os.Remove(path)

	intCfg := New(device1)
	cfg := wrap(path, intCfg, device1)
	defer cfg.stop()

	if err := cfg.Save(); err != nil {
		t.Error(err)
	}

	bs, err := os.ReadFile(path)
	if err != nil {
		t.Error(err)
	}

	unixLineEndings := bytes.Count(bs, []byte("\n"))
	windowsLineEndings := bytes.Count(bs, []byte("\r\n"))
	if unixLineEndings == 0 || windowsLineEndings != unixLineEndings {
		t.Error("expected there to be a non-zero number of Windows line endings")
	}
}

func TestPrepare(t *testing.T) {
	var cfg Configuration

	if cfg.Folders != nil || cfg.Devices != nil || cfg.Options.RawListenAddresses != nil {
		t.Error("Expected nil")
	}

	cfg.prepare(device1)

	if cfg.Folders == nil || cfg.Devices == nil || cfg.Options.RawListenAddresses == nil {
		t.Error("Unexpected nil")
	}
}

func TestCopy(t *testing.T) {
	wrapper, wrapperCancel, err := copyAndLoad(testFs, "example.xml", device1)
	defer wrapperCancel()
	if err != nil {
		t.Fatal(err)
	}
	cfg := wrapper.RawCopy()

	bsOrig, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	copy := cfg.Copy()

	cfg.Devices[0].Addresses[0] = "wrong"
	cfg.Folders[0].Devices[0].DeviceID = protocol.DeviceID{0, 1, 2, 3}
	cfg.Options.RawListenAddresses[0] = "wrong"
	cfg.GUI.APIKey = "wrong"

	bsChanged, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	bsCopy, err := json.MarshalIndent(copy, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	if bytes.Equal(bsOrig, bsChanged) {
		t.Error("Config should have changed")
	}
	if !bytes.Equal(bsOrig, bsCopy) {
		t.Error("Copy should be unchanged")
	}
}

func TestPullOrder(t *testing.T) {
	wrapper, wrapperCleanup, err := copyAndLoad(testFs, "pullorder.xml", device1)
	defer wrapperCleanup()
	if err != nil {
		t.Fatal(err)
	}
	folders := wrapper.Folders()

	expected := []struct {
		name  string
		order PullOrder
	}{
		{"f1", PullOrderRandom},        // empty value, default
		{"f2", PullOrderRandom},        // explicit
		{"f3", PullOrderAlphabetic},    // explicit
		{"f4", PullOrderRandom},        // unknown value, default
		{"f5", PullOrderSmallestFirst}, // explicit
		{"f6", PullOrderLargestFirst},  // explicit
		{"f7", PullOrderOldestFirst},   // explicit
		{"f8", PullOrderNewestFirst},   // explicit
	}

	// Verify values are deserialized correctly

	for _, tc := range expected {
		if actual := folders[tc.name].Order; actual != tc.order {
			t.Errorf("Incorrect pull order for %q: %v != %v", tc.name, actual, tc.order)
		}
	}

	// Serialize and deserialize again to verify it survives the transformation

	buf := new(bytes.Buffer)
	cfg := wrapper.RawCopy()
	cfg.WriteXML(buf)

	t.Logf("%s", buf.Bytes())

	cfg, _, err = ReadXML(buf, device1)
	if err != nil {
		t.Fatal(err)
	}
	wrapper2 := wrap(wrapper.ConfigPath(), cfg, device1)
	defer wrapper2.stop()
	folders = wrapper2.Folders()

	for _, tc := range expected {
		if actual := folders[tc.name].Order; actual != tc.order {
			t.Errorf("Incorrect pull order for %q: %v != %v", tc.name, actual, tc.order)
		}
	}
}

func TestLargeRescanInterval(t *testing.T) {
	wrapper, wrapperCancel, err := copyAndLoad(testFs, "largeinterval.xml", device1)
	defer wrapperCancel()
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

func TestGUIConfigURL(t *testing.T) {
	testcases := [][2]string{
		{"192.0.2.42:8080", "http://192.0.2.42:8080/"},
		{":8080", "http://127.0.0.1:8080/"},
		{"0.0.0.0:8080", "http://127.0.0.1:8080/"},
		{"127.0.0.1:8080", "http://127.0.0.1:8080/"},
		{"127.0.0.2:8080", "http://127.0.0.2:8080/"},
		{"[::]:8080", "http://[::1]:8080/"},
		{"[2001::42]:8080", "http://[2001::42]:8080/"},
	}

	for _, tc := range testcases {
		c := GUIConfiguration{
			RawAddress: tc[0],
		}
		u := c.URL()
		if u != tc[1] {
			t.Errorf("Incorrect URL %s != %s for addr %s", u, tc[1], tc[0])
		}
	}
}

func TestGUIPasswordHash(t *testing.T) {
	var c GUIConfiguration

	// Setting a plaintext password should work
	testPass := "pass"
	if err := c.SetPassword(testPass); err != nil {
		t.Fatal(err)
	}
	if c.Password == testPass {
		t.Error("Password hashing resulted in plaintext")
	}

	if err := c.CompareHashedPassword(testPass); err != nil {
		t.Errorf("No match on same password: %v", err)
	}

	failPass := "different"
	if err := c.CompareHashedPassword(failPass); err == nil {
		t.Errorf("Match on different password: %v", err)
	}

	// Setting a bcrypt hash directly should also work
	hash, err := bcrypt.GenerateFromPassword([]byte("test"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	c.SetPassword(string(hash))
	if err := c.CompareHashedPassword("test"); err != nil {
		t.Errorf("No match on hashed password: %v", err)
	}
}

func TestDuplicateDevices(t *testing.T) {
	// Duplicate devices should be removed

	wrapper, wrapperCancel, err := copyAndLoad(testFs, "dupdevices.xml", device1)
	defer wrapperCancel()
	if err != nil {
		t.Fatal(err)
	}

	if l := len(wrapper.RawCopy().Devices); l != 3 {
		t.Errorf("Incorrect number of devices, %d != 3", l)
	}

	f := wrapper.Folders()["f2"]
	if l := len(f.Devices); l != 2 {
		t.Errorf("Incorrect number of folder devices, %d != 2", l)
	}
}

func TestDuplicateFolders(t *testing.T) {
	// Duplicate folders are a loading error

	_, _Cancel, err := copyAndLoad(testFs, "dupfolders.xml", device1)
	defer _Cancel()
	if err == nil || !strings.Contains(err.Error(), errFolderIDDuplicate.Error()) {
		t.Fatal(`Expected error to mention "duplicate folder ID":`, err)
	}
}

func TestEmptyFolderPaths(t *testing.T) {
	// Empty folder paths are not allowed at the loading stage, and should not
	// get messed up by the prepare steps (e.g., become the current dir or
	// get a slash added so that it becomes the root directory or similar).

	_, _Cancel, err := copyAndLoad(testFs, "nopath.xml", device1)
	defer _Cancel()
	if err == nil || !strings.Contains(err.Error(), errFolderPathEmpty.Error()) {
		t.Fatal("Expected error due to empty folder path, got", err)
	}
}

func TestV14ListenAddressesMigration(t *testing.T) {
	tcs := [][3][]string{
		// Default listen plus default relays is now "default"
		{
			{"tcp://0.0.0.0:22000"},
			{"dynamic+https://relays.syncthing.net/endpoint"},
			{"default"},
		},
		// Default listen address without any relay addresses gets converted
		// to just the listen address. It's easier this way, and frankly the
		// user has gone to some trouble to get the empty string in the
		// config to start with...
		{
			{"tcp://0.0.0.0:22000"}, // old listen addrs
			{""},                    // old relay addrs
			{"tcp://0.0.0.0:22000"}, // new listen addrs
		},
		// Default listen plus non-default relays gets copied verbatim
		{
			{"tcp://0.0.0.0:22000"},
			{"dynamic+https://other.example.com"},
			{"tcp://0.0.0.0:22000", "dynamic+https://other.example.com"},
		},
		// Non-default listen plus default relays gets copied verbatim
		{
			{"tcp://1.2.3.4:22000"},
			{"dynamic+https://relays.syncthing.net/endpoint"},
			{"tcp://1.2.3.4:22000", "dynamic+https://relays.syncthing.net/endpoint"},
		},
		// Default stuff gets sucked into "default", the rest gets copied
		{
			{"tcp://0.0.0.0:22000", "tcp://1.2.3.4:22000"},
			{"dynamic+https://relays.syncthing.net/endpoint", "relay://other.example.com"},
			{"default", "tcp://1.2.3.4:22000", "relay://other.example.com"},
		},
	}

	m := migration{14, migrateToConfigV14}

	for _, tc := range tcs {
		cfg := Configuration{
			Version: 13,
			Options: OptionsConfiguration{
				RawListenAddresses:     tc[0],
				DeprecatedRelayServers: tc[1],
			},
		}
		m.apply(&cfg)
		if cfg.Version != 14 {
			t.Error("Configuration was not converted")
		}

		sort.Strings(tc[2])
		if !reflect.DeepEqual(cfg.Options.RawListenAddresses, tc[2]) {
			t.Errorf("Migration error; actual %#v != expected %#v", cfg.Options.RawListenAddresses, tc[2])
		}
	}
}

func TestIgnoredDevices(t *testing.T) {
	// Verify that ignored devices that are also present in the
	// configuration are not in fact ignored.

	wrapper, wrapperCancel, err := copyAndLoad(testFs, "ignoreddevices.xml", device1)
	defer wrapperCancel()
	if err != nil {
		t.Fatal(err)
	}

	if wrapper.IgnoredDevice(device1) {
		t.Errorf("Device %v should not be ignored", device1)
	}
	if !wrapper.IgnoredDevice(device3) {
		t.Errorf("Device %v should be ignored", device3)
	}
}

func TestIgnoredFolders(t *testing.T) {
	// Verify that ignored folder that are also present in the
	// configuration are not in fact ignored.
	// Also, verify that folders that are shared with a device are not ignored.

	wrapper, wrapperCancel, err := copyAndLoad(testFs, "ignoredfolders.xml", device1)
	defer wrapperCancel()
	if err != nil {
		t.Fatal(err)
	}

	if wrapper.IgnoredFolder(device2, "folder1") {
		t.Errorf("Device %v should not be ignored", device2)
	}
	if !wrapper.IgnoredFolder(device3, "folder1") {
		t.Errorf("Device %v should be ignored", device3)
	}
	// Should be removed, hence not ignored.
	if wrapper.IgnoredFolder(device4, "folder1") {
		t.Errorf("Device %v should not be ignored", device4)
	}

	if !wrapper.IgnoredFolder(device2, "folder2") {
		t.Errorf("Device %v should not be ignored", device2)
	}
	if !wrapper.IgnoredFolder(device3, "folder2") {
		t.Errorf("Device %v should be ignored", device3)
	}

	// 2 for folder2, 1 for folder1, as non-existing device and device the folder is shared with is removed.
	expectedIgnoredFolders := 3
	for _, dev := range wrapper.Devices() {
		expectedIgnoredFolders -= len(dev.IgnoredFolders)
	}
	if expectedIgnoredFolders != 0 {
		t.Errorf("Left with %d ignored folders", expectedIgnoredFolders)
	}
}

func TestGetDevice(t *testing.T) {
	// Verify that the Device() call does the right thing

	wrapper, wrapperCancel, err := copyAndLoad(testFs, "ignoreddevices.xml", device1)
	defer wrapperCancel()
	if err != nil {
		t.Fatal(err)
	}

	// device1 is mentioned in the config

	device, ok := wrapper.Device(device1)
	if !ok {
		t.Error(device1, "should exist")
	}
	if device.DeviceID != device1 {
		t.Error("Should have returned", device1, "not", device.DeviceID)
	}

	// device3 is not

	device, ok = wrapper.Device(device3)
	if ok {
		t.Error(device3, "should not exist")
	}
	if device.DeviceID == device3 {
		t.Error("Should not returned ID", device3)
	}
}

func TestSharesRemovedOnDeviceRemoval(t *testing.T) {
	t.Skip("to fix: test hangs")
	wrapper, wrapperCancel, err := copyAndLoad(testFs, "example.xml", device1)
	defer wrapperCancel()
	if err != nil {
		t.Errorf("Failed: %s", err)
	}

	raw := wrapper.RawCopy()
	raw.Devices = raw.Devices[:len(raw.Devices)-1]

	if len(raw.Folders[0].Devices) <= len(raw.Devices) {
		t.Error("Should have less devices")
	}

	replace(t, wrapper, raw)

	raw = wrapper.RawCopy()
	if len(raw.Folders[0].Devices) > len(raw.Devices) {
		t.Error("Unexpected extra device")
	}
}

func TestIssue4219(t *testing.T) {
	// Adding a folder that was previously ignored should make it unignored.

	r := bytes.NewReader([]byte(`{
		"devices": [
			{
				"deviceID": "GYRZZQB-IRNPV4Z-T7TC52W-EQYJ3TT-FDQW6MW-DFLMU42-SSSU6EM-FBK2VAY",
				"ignoredFolders": [
					{
						"id": "t1"
					},
					{
						"id": "abcd123"
					}
				]
			},
			{
				"deviceID": "LGFPDIT-7SKNNJL-VJZA4FC-7QNCRKA-CE753K7-2BW5QDK-2FOZ7FR-FEP57QJ",
				"ignoredFolders": [
					{
						"id": "t1"
					},
					{
						"id": "abcd123"
					}
				]
			}
		],
		"folders": [
			{
				"id": "abcd123",
				"path": "testdata",
				"devices":[
					{"deviceID": "GYRZZQB-IRNPV4Z-T7TC52W-EQYJ3TT-FDQW6MW-DFLMU42-SSSU6EM-FBK2VAY"}
				]
			}
		],
		"remoteIgnoredDevices": [
			{
				"deviceID": "GYRZZQB-IRNPV4Z-T7TC52W-EQYJ3TT-FDQW6MW-DFLMU42-SSSU6EM-FBK2VAY"
			}
		]
	}`))

	myID := protocol.LocalDeviceID
	cfg, err := ReadJSON(r, myID)
	if err != nil {
		t.Fatal(err)
	}

	if len(cfg.IgnoredDevices) != 0 { // 1 gets removed
		t.Errorf("There should be zero ignored devices, not %d", len(cfg.IgnoredDevices))
	}

	ignoredFolders := 0
	for _, dev := range cfg.Devices {
		ignoredFolders += len(dev.IgnoredFolders)
	}

	if ignoredFolders != 3 { // 1 gets removed
		t.Errorf("There should be three ignored folders, not %d", ignoredFolders)
	}

	w := wrap("/tmp/cfg", cfg, myID)
	defer w.stop()
	if !w.IgnoredFolder(device2, "t1") {
		t.Error("Folder device2 t1 should be ignored")
	}
	if !w.IgnoredFolder(device3, "t1") {
		t.Error("Folder device3 t1 should be ignored")
	}
	if w.IgnoredFolder(device2, "abcd123") {
		t.Error("Folder device2 abcd123 should not be ignored")
	}
	if !w.IgnoredFolder(device3, "abcd123") {
		t.Error("Folder device3 abcd123 should be ignored")
	}
}

func TestInvalidDeviceIDRejected(t *testing.T) {
	// This test verifies that we properly reject invalid device IDs when
	// deserializing a JSON config.

	cases := []struct {
		id string
		ok bool
	}{
		// a genuine device ID
		{"GYRZZQB-IRNPV4Z-T7TC52W-EQYJ3TT-FDQW6MW-DFLMU42-SSSU6EM-FBK2VAY", true},
		// incorrect check digit
		{"GYRZZQB-IRNPV4A-T7TC52W-EQYJ3TT-FDQW6MW-DFLMU42-SSSU6EM-FBK2VAY", false},
		// missing digit
		{"GYRZZQB-IRNPV4Z-T7TC52W-EQYJ3TT-FDQW6MW-DFLMU42-SSSU6EM-FBK2VA", false},
		// clearly broken
		{"invalid", false},
		// accepted as the empty device ID for historical reasons...
		{"", true},
	}

	for _, tc := range cases {
		cfg := defaultConfigAsMap()

		// Change the device ID of the first device to "invalid". Fast and loose
		// with the type assertions as we know what the JSON decoder returns.
		devs := cfg["devices"].([]interface{})
		dev0 := devs[0].(map[string]interface{})
		dev0["deviceID"] = tc.id
		devs[0] = dev0

		invalidJSON, err := json.Marshal(cfg)
		if err != nil {
			t.Fatal(err)
		}

		_, err = ReadJSON(bytes.NewReader(invalidJSON), device1)
		if tc.ok && err != nil {
			t.Errorf("unexpected error for device ID %q: %v", tc.id, err)
		} else if !tc.ok && err == nil {
			t.Errorf("device ID %q, expected error but got nil", tc.id)
		}
	}
}

func TestInvalidFolderIDRejected(t *testing.T) {
	// This test verifies that we properly reject invalid folder IDs when
	// deserializing a JSON config.

	cases := []struct {
		id string
		ok bool
	}{
		// a reasonable folder ID
		{"foo", true},
		// empty is not OK
		{"", false},
	}

	for _, tc := range cases {
		cfg := defaultConfigAsMap()

		// Change the folder ID of the first folder to the empty string.
		// Fast and loose with the type assertions as we know what the JSON
		// decoder returns.
		devs := cfg["folders"].([]interface{})
		dev0 := devs[0].(map[string]interface{})
		dev0["id"] = tc.id
		devs[0] = dev0

		invalidJSON, err := json.Marshal(cfg)
		if err != nil {
			t.Fatal(err)
		}

		_, err = ReadJSON(bytes.NewReader(invalidJSON), device1)
		if tc.ok && err != nil {
			t.Errorf("unexpected error for folder ID %q: %v", tc.id, err)
		} else if !tc.ok && err == nil {
			t.Errorf("folder ID %q, expected error but got nil", tc.id)
		}
	}
}

func TestFilterURLSchemePrefix(t *testing.T) {
	cases := []struct {
		before []string
		prefix string
		after  []string
	}{
		{[]string{}, "kcp", []string{}},
		{[]string{"tcp://foo"}, "kcp", []string{"tcp://foo"}},
		{[]string{"kcp://foo"}, "kcp", []string{}},
		{[]string{"tcp6://foo", "kcp6://foo"}, "kcp", []string{"tcp6://foo"}},
		{[]string{"kcp6://foo", "tcp6://foo"}, "kcp", []string{"tcp6://foo"}},
		{
			[]string{"tcp://foo", "tcp4://foo", "kcp://foo", "kcp4://foo", "banana://foo", "banana4://foo", "banananas!"},
			"kcp",
			[]string{"tcp://foo", "tcp4://foo", "banana://foo", "banana4://foo", "banananas!"},
		},
	}

	for _, tc := range cases {
		res := filterURLSchemePrefix(tc.before, tc.prefix)
		if !reflect.DeepEqual(res, tc.after) {
			t.Errorf("filterURLSchemePrefix => %q, expected %q", res, tc.after)
		}
	}
}

func TestDeviceConfigObservedNotNil(t *testing.T) {
	cfg := Configuration{
		Devices: []DeviceConfiguration{
			{},
		},
	}

	cfg.prepare(device1)

	for _, dev := range cfg.Devices {
		if dev.IgnoredFolders == nil {
			t.Errorf("Ignored folders nil")
		}
	}
}

func TestRemoveDeviceWithEmptyID(t *testing.T) {
	cfg := Configuration{
		Devices: []DeviceConfiguration{
			{
				Name: "foo",
			},
		},
		Folders: []FolderConfiguration{
			{
				ID:      "foo",
				Path:    "testdata",
				Devices: []FolderDeviceConfiguration{{}},
			},
		},
	}

	cfg.prepare(device1)

	if len(cfg.Devices) != 1 {
		t.Error("Expected one device")
	} else if cfg.Devices[0].DeviceID != device1 {
		t.Error("Expected device with empty ID to be removed from config:", cfg.Devices)
	}
	if len(cfg.Folders[0].Devices) != 1 {
		t.Error("Expected one device in folder")
	} else if cfg.Folders[0].Devices[0].DeviceID != device1 {
		t.Error("Expected device with empty ID to be removed from folder")
	}
}

func TestMaxConcurrentFolders(t *testing.T) {
	cases := []struct {
		input  int
		output int
	}{
		{input: -42, output: 0},
		{input: -1, output: 0},
		{input: 0, output: runtime.GOMAXPROCS(-1)},
		{input: 1, output: 1},
		{input: 42, output: 42},
	}

	for _, tc := range cases {
		opts := OptionsConfiguration{RawMaxFolderConcurrency: tc.input}
		res := opts.MaxFolderConcurrency()
		if res != tc.output {
			t.Errorf("Wrong MaxFolderConcurrency, %d => %d, expected %d", tc.input, res, tc.output)
		}
	}
}

func adjustDeviceConfiguration(cfg *DeviceConfiguration, id protocol.DeviceID, name string) {
	cfg.DeviceID = id
	cfg.Name = name
}

func adjustFolderConfiguration(cfg *FolderConfiguration, id, label string, fsType fs.FilesystemType, path string) {
	cfg.ID = id
	cfg.Label = label
	cfg.FilesystemType = FilesystemType(fsType)
	cfg.Path = path
}

// defaultConfigAsMap returns a valid default config as a JSON-decoded
// map[string]interface{}. This is useful to override random elements and
// re-encode into JSON.
func defaultConfigAsMap() map[string]interface{} {
	cfg := New(device1)
	dev := cfg.Defaults.Device.Copy()
	adjustDeviceConfiguration(&dev, device2, "name")
	cfg.Devices = append(cfg.Devices, dev)
	folder := cfg.Defaults.Folder.Copy()
	adjustFolderConfiguration(&folder, "default", "default", fs.FilesystemTypeBasic, "/tmp")
	cfg.Folders = append(cfg.Folders, folder)
	bs, err := json.Marshal(cfg)
	if err != nil {
		// can't happen
		panic(err)
	}
	var tmp map[string]interface{}
	if err := json.Unmarshal(bs, &tmp); err != nil {
		// can't happen
		panic(err)
	}
	return tmp
}

func copyToTmp(fs fs.Filesystem, path string) (string, error) {
	orig, err := fs.Open(path)
	if err != nil {
		return "", err
	}
	defer orig.Close()
	temp, err := fs.Create("syncthing-configTest-" + rand.String(6))
	if err != nil {
		return "", err
	}
	defer temp.Close()

	if _, err := io.Copy(temp, orig); err != nil {
		return "", err
	}
	return temp.Name(), nil
}

func copyAndLoad(fs fs.Filesystem, path string, myID protocol.DeviceID) (*testWrapper, func(), error) {
	temp, err := copyToTmp(fs, path)
	if err != nil {
		return nil, func() {}, err
	}
	wrapper, err := loadTest(fs, temp, myID)
	if err != nil {
		return nil, func() {}, err
	}
	return wrapper, func() {
		fs.Remove(temp)
		wrapper.stop()
	}, nil
}

func loadTest(fs fs.Filesystem, path string, myID protocol.DeviceID) (*testWrapper, error) {
	cfg, _, err := loadWrapTest(fs, path, myID, events.NoopLogger)
	if err != nil {
		return nil, err
	}

	return startWrapper(cfg), nil
}

func loadWrapTest(fs fs.Filesystem, path string, myID protocol.DeviceID, evLogger events.Logger) (Wrapper, int, error) {
	fd, err := fs.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer fd.Close()

	cfg, originalVersion, err := ReadXML(fd, myID)
	if err != nil {
		return nil, 0, err
	}

	return Wrap(filepath.Join(testFs.URI(), path), cfg, myID, evLogger), originalVersion, nil
}

func load(path string, myID protocol.DeviceID) (*testWrapper, error) {
	cfg, _, err := Load(path, myID, events.NoopLogger)
	if err != nil {
		return nil, err
	}
	return startWrapper(cfg), nil
}

func wrap(path string, cfg Configuration, myID protocol.DeviceID) *testWrapper {
	wrapper := Wrap(path, cfg, myID, events.NoopLogger)
	return startWrapper(wrapper)
}

type testWrapper struct {
	Wrapper
	cancel context.CancelFunc
	done   chan struct{}
}

func (w *testWrapper) stop() {
	w.cancel()
	<-w.done
}

func startWrapper(wrapper Wrapper) *testWrapper {
	tw := &testWrapper{
		Wrapper: wrapper,
		done:    make(chan struct{}),
	}
	ctx, cancel := context.WithCancel(context.Background())
	tw.cancel = cancel
	go func() {
		wrapper.Serve(ctx)
		close(tw.done)
	}()
	return tw
}

func TestInternalVersioningConfiguration(t *testing.T) {
	// Verify that the versioning configuration XML serializes to something
	// reasonable.

	cfg := New(device1)
	folder := cfg.Defaults.Folder.Copy()
	adjustFolderConfiguration(&folder, "default", "default", fs.FilesystemTypeBasic, "/tmp")
	cfg.Folders = append(cfg.Folders, folder)
	cfg.Folders[0].Versioning = VersioningConfiguration{
		Type:             "foo",
		Params:           map[string]string{"bar": "baz"},
		CleanupIntervalS: 42,
	}

	// These things should all be present in the serialized version.
	expected := []string{
		`<versioning type="foo">`,
		`<param key="bar" val="baz"`,
		`<cleanupIntervalS>42<`,
		`</versioning>`,
	}

	bs, err := xml.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	for _, exp := range expected {
		if !strings.Contains(string(bs), exp) {
			t.Logf("%s", bs)
			t.Fatal("bad serializion of versioning parameters")
		}
	}
}

func TestReceiveEncryptedFolderFixed(t *testing.T) {
	cfg := Configuration{
		Folders: []FolderConfiguration{
			{
				ID:                 "foo",
				Path:               "testdata",
				Type:               FolderTypeReceiveEncrypted,
				DisableTempIndexes: false,
				IgnorePerms:        false,
			},
		},
	}

	cfg.prepare(device1)

	if len(cfg.Folders) != 1 {
		t.Fatal("Expected one folder")
	}
	f := cfg.Folders[0]
	if !f.DisableTempIndexes {
		t.Error("DisableTempIndexes should be true")
	}
	if !f.IgnorePerms {
		t.Error("IgnorePerms should be true")
	}
}

func TestXattrFilter(t *testing.T) {
	cases := []struct {
		in     []string
		filter []XattrFilterEntry
		out    []string
	}{
		{in: nil, filter: nil, out: nil},
		{in: []string{"foo", "bar", "baz"}, filter: nil, out: []string{"foo", "bar", "baz"}},
		{
			in:     []string{"foo", "bar", "baz"},
			filter: []XattrFilterEntry{{Match: "b*", Permit: true}},
			out:    []string{"bar", "baz"},
		},
		{
			in:     []string{"foo", "bar", "baz"},
			filter: []XattrFilterEntry{{Match: "b*", Permit: false}, {Match: "*", Permit: true}},
			out:    []string{"foo"},
		},
		{
			in:     []string{"foo", "bar", "baz"},
			filter: []XattrFilterEntry{{Match: "yoink", Permit: true}},
			out:    []string{},
		},
	}

	for _, tc := range cases {
		f := XattrFilter{Entries: tc.filter}
		var out []string
		for _, s := range tc.in {
			if f.Permit(s) {
				out = append(out, s)
			}
		}

		if fmt.Sprint(out) != fmt.Sprint(tc.out) {
			t.Errorf("Filter.Apply(%v, %v) == %v, expected %v", tc.in, tc.filter, out, tc.out)
		}
	}
}

func TestUntrustedIntroducer(t *testing.T) {
	fd, err := os.Open("testdata/untrustedintroducer.xml")
	if err != nil {
		t.Fatal(err)
	}
	cfg, _, err := ReadXML(fd, device1)
	if err != nil {
		t.Fatal(err)
	}

	if len(cfg.Devices) != 2 {
		// ourselves and the remote in the config
		t.Fatal("Expected two devices")
	}

	// Check that the introducer and auto-accept flags were negated
	var foundUntrusted protocol.DeviceID
	for _, d := range cfg.Devices {
		if d.Name == "untrusted" {
			foundUntrusted = d.DeviceID
			if !d.Untrusted {
				t.Error("untrusted device should be untrusted")
			}
			if d.Introducer {
				t.Error("untrusted device should not be an introducer")
			}
			if d.AutoAcceptFolders {
				t.Error("untrusted device should not auto-accept folders")
			}
		}
	}
	if foundUntrusted.Equals(protocol.EmptyDeviceID) {
		t.Error("untrusted device not found")
	}

	// Folder A has the device added without a password, which is not permitted
	folderA := cfg.FolderMap()["a"]
	for _, dev := range folderA.Devices {
		if dev.DeviceID == foundUntrusted {
			t.Error("untrusted device should not be in folder A")
		}
	}

	// Folder B has the device added with a password, this is just a sanity check
	folderB := cfg.FolderMap()["b"]
	found := false
	for _, dev := range folderB.Devices {
		if dev.DeviceID == foundUntrusted {
			found = true
			if dev.EncryptionPassword == "" {
				t.Error("untrusted device should have a password in folder B")
			}
		}
	}
	if !found {
		t.Error("untrusted device not found in folder B")
	}
}

// Verify that opening a config with myID == protocol.EmptyDeviceID doesn't add that ID to the config.
// Done in various places where config is needed, but the device ID isn't known.
func TestLoadEmptyDeviceID(t *testing.T) {
	temp, err := copyToTmp(testFs, "example.xml")
	if err != nil {
		t.Fatal(err)
	}
	fd, err := testFs.Open(temp)
	if err != nil {
		t.Fatal(err)
	}
	defer fd.Close()
	cfg, _, err := ReadXML(fd, protocol.EmptyDeviceID)
	if err != nil {
		t.Fatal(err)
	}
	for _, devCfg := range cfg.Devices {
		if devCfg.DeviceID == protocol.EmptyDeviceID {
			t.Fatal("Device with empty ID")
		}
	}
	for _, folderCfg := range cfg.Folders {
		for _, devCfg := range folderCfg.Devices {
			if devCfg.DeviceID == protocol.EmptyDeviceID {
				t.Fatalf("Device with empty ID in folder %v", folderCfg.Description())
			}
		}
	}
}

func loadTestFiles() {
	entries, err := os.ReadDir("testdata")
	if err != nil {
		return
	}
	for _, e := range entries {
		handleFile(e.Name())
	}
}

func handleFile(name string) {
	fd, err := testFs.Create(name)
	if err != nil {
		return
	}
	origin, _ := os.ReadFile(filepath.Join("testdata", name))
	fd.Write(origin)
	fd.Close()
}

func TestCopyMatching(t *testing.T) {
	type Nested struct {
		A int
	}
	type Test struct {
		CopyA  int
		CopyB  []string
		CopyC  Nested
		CopyD  *Nested
		NoCopy int `restart:"true"`
	}

	from := Test{
		CopyA: 1,
		CopyB: []string{"friend", "foe"},
		CopyC: Nested{
			A: 2,
		},
		CopyD: &Nested{
			A: 3,
		},
		NoCopy: 4,
	}

	to := Test{
		CopyA: 11,
		CopyB: []string{"foot", "toe"},
		CopyC: Nested{
			A: 22,
		},
		CopyD: &Nested{
			A: 33,
		},
		NoCopy: 44,
	}

	// Copy empty fields
	copyMatchingTag(&from, &to, "restart", func(v string) bool {
		return v != "true"
	})

	if to.CopyA != 1 {
		t.Error("CopyA")
	}
	if len(to.CopyB) != 2 || to.CopyB[0] != "friend" || to.CopyB[1] != "foe" {
		t.Error("CopyB")
	}
	if to.CopyC.A != 2 {
		t.Error("CopyC")
	}
	if to.CopyD.A != 3 {
		t.Error("CopyC")
	}
	if to.NoCopy != 44 {
		t.Error("NoCopy")
	}
}
