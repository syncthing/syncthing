// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package model

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/syncthing/protocol"
	"github.com/syncthing/syncthing/internal/config"
	"github.com/syncthing/syncthing/internal/db"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/storage"
)

var device1, device2 protocol.DeviceID
var defaultConfig *config.Wrapper
var defaultFolderConfig config.FolderConfiguration

func init() {
	device1, _ = protocol.DeviceIDFromString("AIR6LPZ-7K4PTTV-UXQSMUU-CPQ5YWH-OEDFIIQ-JUG777G-2YQXXR5-YD6AWQR")
	device2, _ = protocol.DeviceIDFromString("GYRZZQB-IRNPV4Z-T7TC52W-EQYJ3TT-FDQW6MW-DFLMU42-SSSU6EM-FBK2VAY")

	defaultFolderConfig = config.FolderConfiguration{
		ID:   "default",
		Path: "testdata",
		Devices: []config.FolderDeviceConfiguration{
			{
				DeviceID: device1,
			},
		},
	}
	_defaultConfig := config.Configuration{
		Folders: []config.FolderConfiguration{defaultFolderConfig},
		Devices: []config.DeviceConfiguration{
			{
				DeviceID: device1,
			},
		},
	}
	defaultConfig = config.Wrap("/tmp/test", _defaultConfig)
}

var testDataExpected = map[string]protocol.FileInfo{
	"foo": {
		Name:     "foo",
		Flags:    0,
		Modified: 0,
		Blocks:   []protocol.BlockInfo{{Offset: 0x0, Size: 0x7, Hash: []uint8{0xae, 0xc0, 0x70, 0x64, 0x5f, 0xe5, 0x3e, 0xe3, 0xb3, 0x76, 0x30, 0x59, 0x37, 0x61, 0x34, 0xf0, 0x58, 0xcc, 0x33, 0x72, 0x47, 0xc9, 0x78, 0xad, 0xd1, 0x78, 0xb6, 0xcc, 0xdf, 0xb0, 0x1, 0x9f}}},
	},
	"empty": {
		Name:     "empty",
		Flags:    0,
		Modified: 0,
		Blocks:   []protocol.BlockInfo{{Offset: 0x0, Size: 0x0, Hash: []uint8{0xe3, 0xb0, 0xc4, 0x42, 0x98, 0xfc, 0x1c, 0x14, 0x9a, 0xfb, 0xf4, 0xc8, 0x99, 0x6f, 0xb9, 0x24, 0x27, 0xae, 0x41, 0xe4, 0x64, 0x9b, 0x93, 0x4c, 0xa4, 0x95, 0x99, 0x1b, 0x78, 0x52, 0xb8, 0x55}}},
	},
	"bar": {
		Name:     "bar",
		Flags:    0,
		Modified: 0,
		Blocks:   []protocol.BlockInfo{{Offset: 0x0, Size: 0xa, Hash: []uint8{0x2f, 0x72, 0xcc, 0x11, 0xa6, 0xfc, 0xd0, 0x27, 0x1e, 0xce, 0xf8, 0xc6, 0x10, 0x56, 0xee, 0x1e, 0xb1, 0x24, 0x3b, 0xe3, 0x80, 0x5b, 0xf9, 0xa9, 0xdf, 0x98, 0xf9, 0x2f, 0x76, 0x36, 0xb0, 0x5c}}},
	},
}

func init() {
	// Fix expected test data to match reality
	for n, f := range testDataExpected {
		fi, _ := os.Stat("testdata/" + n)
		f.Flags = uint32(fi.Mode())
		f.Modified = fi.ModTime().Unix()
		testDataExpected[n] = f
	}
}

func TestRequest(t *testing.T) {
	db, _ := leveldb.Open(storage.NewMemStorage(), nil)

	m := NewModel(defaultConfig, protocol.LocalDeviceID, "device", "syncthing", "dev", db)

	// device1 shares default, but device2 doesn't
	m.AddFolder(defaultFolderConfig)
	m.StartFolderRO("default")
	m.ScanFolder("default")

	// Existing, shared file
	bs, err := m.Request(device1, "default", "foo", 0, 6, nil, 0, nil)
	if err != nil {
		t.Error(err)
	}
	if bytes.Compare(bs, []byte("foobar")) != 0 {
		t.Errorf("Incorrect data from request: %q", string(bs))
	}

	// Existing, nonshared file
	bs, err = m.Request(device2, "default", "foo", 0, 6, nil, 0, nil)
	if err == nil {
		t.Error("Unexpected nil error on insecure file read")
	}
	if bs != nil {
		t.Errorf("Unexpected non nil data on insecure file read: %q", string(bs))
	}

	// Nonexistent file
	bs, err = m.Request(device1, "default", "nonexistent", 0, 6, nil, 0, nil)
	if err == nil {
		t.Error("Unexpected nil error on insecure file read")
	}
	if bs != nil {
		t.Errorf("Unexpected non nil data on insecure file read: %q", string(bs))
	}

	// Shared folder, but disallowed file name
	bs, err = m.Request(device1, "default", "../walk.go", 0, 6, nil, 0, nil)
	if err == nil {
		t.Error("Unexpected nil error on insecure file read")
	}
	if bs != nil {
		t.Errorf("Unexpected non nil data on insecure file read: %q", string(bs))
	}

	// Larger block than available
	bs, err = m.Request(device1, "default", "foo", 0, 42, nil, 0, nil)
	if err == nil {
		t.Error("Unexpected nil error on insecure file read")
	}
	if bs != nil {
		t.Errorf("Unexpected non nil data on insecure file read: %q", string(bs))
	}

	// Negative offset
	bs, err = m.Request(device1, "default", "foo", -4, 6, nil, 0, nil)
	if err == nil {
		t.Error("Unexpected nil error on insecure file read")
	}
	if bs != nil {
		t.Errorf("Unexpected non nil data on insecure file read: %q", string(bs))
	}

	// Negative size
	bs, err = m.Request(device1, "default", "foo", 4, -4, nil, 0, nil)
	if err == nil {
		t.Error("Unexpected nil error on insecure file read")
	}
	if bs != nil {
		t.Errorf("Unexpected non nil data on insecure file read: %q", string(bs))
	}
}

func genFiles(n int) []protocol.FileInfo {
	files := make([]protocol.FileInfo, n)
	t := time.Now().Unix()
	for i := 0; i < n; i++ {
		files[i] = protocol.FileInfo{
			Name:     fmt.Sprintf("file%d", i),
			Modified: t,
			Blocks:   []protocol.BlockInfo{{0, 100, []byte("some hash bytes")}},
		}
	}

	return files
}

func BenchmarkIndex10000(b *testing.B) {
	db, _ := leveldb.Open(storage.NewMemStorage(), nil)
	m := NewModel(defaultConfig, protocol.LocalDeviceID, "device", "syncthing", "dev", db)
	m.AddFolder(defaultFolderConfig)
	m.ScanFolder("default")
	files := genFiles(10000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Index(device1, "default", files, 0, nil)
	}
}

func BenchmarkIndex00100(b *testing.B) {
	db, _ := leveldb.Open(storage.NewMemStorage(), nil)
	m := NewModel(defaultConfig, protocol.LocalDeviceID, "device", "syncthing", "dev", db)
	m.AddFolder(defaultFolderConfig)
	m.ScanFolder("default")
	files := genFiles(100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Index(device1, "default", files, 0, nil)
	}
}

func BenchmarkIndexUpdate10000f10000(b *testing.B) {
	db, _ := leveldb.Open(storage.NewMemStorage(), nil)
	m := NewModel(defaultConfig, protocol.LocalDeviceID, "device", "syncthing", "dev", db)
	m.AddFolder(defaultFolderConfig)
	m.ScanFolder("default")
	files := genFiles(10000)
	m.Index(device1, "default", files, 0, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.IndexUpdate(device1, "default", files, 0, nil)
	}
}

func BenchmarkIndexUpdate10000f00100(b *testing.B) {
	db, _ := leveldb.Open(storage.NewMemStorage(), nil)
	m := NewModel(defaultConfig, protocol.LocalDeviceID, "device", "syncthing", "dev", db)
	m.AddFolder(defaultFolderConfig)
	m.ScanFolder("default")
	files := genFiles(10000)
	m.Index(device1, "default", files, 0, nil)

	ufiles := genFiles(100)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.IndexUpdate(device1, "default", ufiles, 0, nil)
	}
}

func BenchmarkIndexUpdate10000f00001(b *testing.B) {
	db, _ := leveldb.Open(storage.NewMemStorage(), nil)
	m := NewModel(defaultConfig, protocol.LocalDeviceID, "device", "syncthing", "dev", db)
	m.AddFolder(defaultFolderConfig)
	m.ScanFolder("default")
	files := genFiles(10000)
	m.Index(device1, "default", files, 0, nil)

	ufiles := genFiles(1)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.IndexUpdate(device1, "default", ufiles, 0, nil)
	}
}

type FakeConnection struct {
	id          protocol.DeviceID
	requestData []byte
}

func (FakeConnection) Close() error {
	return nil
}

func (f FakeConnection) ID() protocol.DeviceID {
	return f.id
}

func (f FakeConnection) Name() string {
	return ""
}

func (f FakeConnection) Option(string) string {
	return ""
}

func (FakeConnection) Index(string, []protocol.FileInfo, uint32, []protocol.Option) error {
	return nil
}

func (FakeConnection) IndexUpdate(string, []protocol.FileInfo, uint32, []protocol.Option) error {
	return nil
}

func (f FakeConnection) Request(folder, name string, offset int64, size int, hash []byte, flags uint32, options []protocol.Option) ([]byte, error) {
	return f.requestData, nil
}

func (FakeConnection) ClusterConfig(protocol.ClusterConfigMessage) {}

func (FakeConnection) Ping() bool {
	return true
}

func (FakeConnection) Statistics() protocol.Statistics {
	return protocol.Statistics{}
}

func BenchmarkRequest(b *testing.B) {
	db, _ := leveldb.Open(storage.NewMemStorage(), nil)
	m := NewModel(defaultConfig, protocol.LocalDeviceID, "device", "syncthing", "dev", db)
	m.AddFolder(defaultFolderConfig)
	m.ScanFolder("default")

	const n = 1000
	files := make([]protocol.FileInfo, n)
	t := time.Now().Unix()
	for i := 0; i < n; i++ {
		files[i] = protocol.FileInfo{
			Name:     fmt.Sprintf("file%d", i),
			Modified: t,
			Blocks:   []protocol.BlockInfo{{0, 100, []byte("some hash bytes")}},
		}
	}

	fc := FakeConnection{
		id:          device1,
		requestData: []byte("some data to return"),
	}
	m.AddConnection(fc, fc)
	m.Index(device1, "default", files, 0, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		data, err := m.requestGlobal(device1, "default", files[i%n].Name, 0, 32, nil, 0, nil)
		if err != nil {
			b.Error(err)
		}
		if data == nil {
			b.Error("nil data")
		}
	}
}

func TestDeviceRename(t *testing.T) {
	ccm := protocol.ClusterConfigMessage{
		ClientName:    "syncthing",
		ClientVersion: "v0.9.4",
	}

	defer os.Remove("tmpconfig.xml")

	cfg := config.New(device1)
	cfg.Devices = []config.DeviceConfiguration{
		{
			DeviceID: device1,
		},
	}

	db, _ := leveldb.Open(storage.NewMemStorage(), nil)
	m := NewModel(config.Wrap("tmpconfig.xml", cfg), protocol.LocalDeviceID, "device", "syncthing", "dev", db)
	if cfg.Devices[0].Name != "" {
		t.Errorf("Device already has a name")
	}

	m.ClusterConfig(device1, ccm)
	if cfg.Devices[0].Name != "" {
		t.Errorf("Device already has a name")
	}

	ccm.Options = []protocol.Option{
		{
			Key:   "name",
			Value: "tester",
		},
	}
	m.ClusterConfig(device1, ccm)
	if cfg.Devices[0].Name != "tester" {
		t.Errorf("Device did not get a name")
	}

	ccm.Options[0].Value = "tester2"
	m.ClusterConfig(device1, ccm)
	if cfg.Devices[0].Name != "tester" {
		t.Errorf("Device name got overwritten")
	}

	cfgw, err := config.Load("tmpconfig.xml", protocol.LocalDeviceID)
	if err != nil {
		t.Error(err)
		return
	}
	if cfgw.Devices()[device1].Name != "tester" {
		t.Errorf("Device name not saved in config")
	}
}

func TestClusterConfig(t *testing.T) {
	cfg := config.New(device1)
	cfg.Devices = []config.DeviceConfiguration{
		{
			DeviceID:   device1,
			Introducer: true,
		},
		{
			DeviceID: device2,
		},
	}
	cfg.Folders = []config.FolderConfiguration{
		{
			ID: "folder1",
			Devices: []config.FolderDeviceConfiguration{
				{DeviceID: device1},
				{DeviceID: device2},
			},
		},
		{
			ID: "folder2",
			Devices: []config.FolderDeviceConfiguration{
				{DeviceID: device1},
				{DeviceID: device2},
			},
		},
	}

	db, _ := leveldb.Open(storage.NewMemStorage(), nil)

	m := NewModel(config.Wrap("/tmp/test", cfg), protocol.LocalDeviceID, "device", "syncthing", "dev", db)
	m.AddFolder(cfg.Folders[0])
	m.AddFolder(cfg.Folders[1])

	cm := m.clusterConfig(device2)

	if l := len(cm.Folders); l != 2 {
		t.Fatalf("Incorrect number of folders %d != 2", l)
	}

	r := cm.Folders[0]
	if r.ID != "folder1" {
		t.Errorf("Incorrect folder %q != folder1", r.ID)
	}
	if l := len(r.Devices); l != 2 {
		t.Errorf("Incorrect number of devices %d != 2", l)
	}
	if id := r.Devices[0].ID; bytes.Compare(id, device1[:]) != 0 {
		t.Errorf("Incorrect device ID %x != %x", id, device1)
	}
	if r.Devices[0].Flags&protocol.FlagIntroducer == 0 {
		t.Error("Device1 should be flagged as Introducer")
	}
	if id := r.Devices[1].ID; bytes.Compare(id, device2[:]) != 0 {
		t.Errorf("Incorrect device ID %x != %x", id, device2)
	}
	if r.Devices[1].Flags&protocol.FlagIntroducer != 0 {
		t.Error("Device2 should not be flagged as Introducer")
	}

	r = cm.Folders[1]
	if r.ID != "folder2" {
		t.Errorf("Incorrect folder %q != folder2", r.ID)
	}
	if l := len(r.Devices); l != 2 {
		t.Errorf("Incorrect number of devices %d != 2", l)
	}
	if id := r.Devices[0].ID; bytes.Compare(id, device1[:]) != 0 {
		t.Errorf("Incorrect device ID %x != %x", id, device1)
	}
	if r.Devices[0].Flags&protocol.FlagIntroducer == 0 {
		t.Error("Device1 should be flagged as Introducer")
	}
	if id := r.Devices[1].ID; bytes.Compare(id, device2[:]) != 0 {
		t.Errorf("Incorrect device ID %x != %x", id, device2)
	}
	if r.Devices[1].Flags&protocol.FlagIntroducer != 0 {
		t.Error("Device2 should not be flagged as Introducer")
	}
}

func TestIgnores(t *testing.T) {
	arrEqual := func(a, b []string) bool {
		if len(a) != len(b) {
			return false
		}

		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}

	db, _ := leveldb.Open(storage.NewMemStorage(), nil)
	m := NewModel(defaultConfig, protocol.LocalDeviceID, "device", "syncthing", "dev", db)
	m.AddFolder(defaultFolderConfig)
	m.StartFolderRO("default")

	expected := []string{
		".*",
		"quux",
	}

	ignores, _, err := m.GetIgnores("default")
	if err != nil {
		t.Error(err)
	}

	if !arrEqual(ignores, expected) {
		t.Errorf("Incorrect ignores: %v != %v", ignores, expected)
	}

	ignores = append(ignores, "pox")

	err = m.SetIgnores("default", ignores)
	if err != nil {
		t.Error(err)
	}

	ignores2, _, err := m.GetIgnores("default")
	if err != nil {
		t.Error(err)
	}

	if arrEqual(expected, ignores2) {
		t.Errorf("Incorrect ignores: %v == %v", ignores2, expected)
	}

	if !arrEqual(ignores, ignores2) {
		t.Errorf("Incorrect ignores: %v != %v", ignores2, ignores)
	}

	err = m.SetIgnores("default", expected)
	if err != nil {
		t.Error(err)
	}

	ignores, _, err = m.GetIgnores("default")
	if err != nil {
		t.Error(err)
	}

	if !arrEqual(ignores, expected) {
		t.Errorf("Incorrect ignores: %v != %v", ignores, expected)
	}

	ignores, _, err = m.GetIgnores("doesnotexist")
	if err == nil {
		t.Error("No error")
	}

	err = m.SetIgnores("doesnotexist", expected)
	if err == nil {
		t.Error("No error")
	}

	m.AddFolder(config.FolderConfiguration{ID: "fresh", Path: "XXX"})
	ignores, _, err = m.GetIgnores("fresh")
	if err != nil {
		t.Error(err)
	}
	if len(ignores) > 0 {
		t.Errorf("Expected no ignores, got: %v", ignores)
	}
}

func TestRefuseUnknownBits(t *testing.T) {
	db, _ := leveldb.Open(storage.NewMemStorage(), nil)
	m := NewModel(defaultConfig, protocol.LocalDeviceID, "device", "syncthing", "dev", db)
	m.AddFolder(defaultFolderConfig)

	m.ScanFolder("default")
	m.Index(device1, "default", []protocol.FileInfo{
		{
			Name:  "invalid1",
			Flags: (protocol.FlagsAll + 1) &^ protocol.FlagInvalid,
		},
		{
			Name:  "invalid2",
			Flags: (protocol.FlagsAll + 2) &^ protocol.FlagInvalid,
		},
		{
			Name:  "invalid3",
			Flags: (1 << 31) &^ protocol.FlagInvalid,
		},
		{
			Name:  "valid",
			Flags: protocol.FlagsAll &^ (protocol.FlagInvalid | protocol.FlagSymlink),
		},
	}, 0, nil)

	for _, name := range []string{"invalid1", "invalid2", "invalid3"} {
		f, ok := m.CurrentGlobalFile("default", name)
		if ok || f.Name == name {
			t.Error("Invalid file found or name match")
		}
	}
	f, ok := m.CurrentGlobalFile("default", "valid")
	if !ok || f.Name != "valid" {
		t.Error("Valid file not found or name mismatch", ok, f)
	}
}

func TestROScanRecovery(t *testing.T) {
	ldb, _ := leveldb.Open(storage.NewMemStorage(), nil)
	set := db.NewFileSet("default", ldb)
	set.Update(protocol.LocalDeviceID, []protocol.FileInfo{
		{Name: "dummyfile"},
	})

	fcfg := config.FolderConfiguration{
		ID:              "default",
		Path:            "testdata/rotestfolder",
		RescanIntervalS: 1,
	}
	cfg := config.Wrap("/tmp/test", config.Configuration{
		Folders: []config.FolderConfiguration{fcfg},
		Devices: []config.DeviceConfiguration{
			{
				DeviceID: device1,
			},
		},
	})

	os.RemoveAll(fcfg.Path)

	m := NewModel(cfg, protocol.LocalDeviceID, "device", "syncthing", "dev", ldb)

	m.AddFolder(fcfg)
	m.StartFolderRO("default")

	waitFor := func(status string) error {
		timeout := time.Now().Add(2 * time.Second)
		for {
			if time.Now().After(timeout) {
				return fmt.Errorf("Timed out waiting for status: %s, current status: %s", status, m.cfg.Folders()["default"].Invalid)
			}
			if m.cfg.Folders()["default"].Invalid == status {
				return nil
			}
			time.Sleep(10 * time.Millisecond)
		}
	}

	if err := waitFor("Folder path missing"); err != nil {
		t.Error(err)
		return
	}

	os.Mkdir(fcfg.Path, 0700)

	if err := waitFor("Folder marker missing"); err != nil {
		t.Error(err)
		return
	}

	fd, err := os.Create(filepath.Join(fcfg.Path, ".stfolder"))
	if err != nil {
		t.Error(err)
		return
	}
	fd.Close()

	if err := waitFor(""); err != nil {
		t.Error(err)
		return
	}

	os.Remove(filepath.Join(fcfg.Path, ".stfolder"))

	if err := waitFor("Folder marker missing"); err != nil {
		t.Error(err)
		return
	}

	os.Remove(fcfg.Path)

	if err := waitFor("Folder path missing"); err != nil {
		t.Error(err)
		return
	}
}

func TestRWScanRecovery(t *testing.T) {
	ldb, _ := leveldb.Open(storage.NewMemStorage(), nil)
	set := db.NewFileSet("default", ldb)
	set.Update(protocol.LocalDeviceID, []protocol.FileInfo{
		{Name: "dummyfile"},
	})

	fcfg := config.FolderConfiguration{
		ID:              "default",
		Path:            "testdata/rwtestfolder",
		RescanIntervalS: 1,
	}
	cfg := config.Wrap("/tmp/test", config.Configuration{
		Folders: []config.FolderConfiguration{fcfg},
		Devices: []config.DeviceConfiguration{
			{
				DeviceID: device1,
			},
		},
	})

	os.RemoveAll(fcfg.Path)

	m := NewModel(cfg, protocol.LocalDeviceID, "device", "syncthing", "dev", ldb)

	m.AddFolder(fcfg)
	m.StartFolderRW("default")

	waitFor := func(status string) error {
		timeout := time.Now().Add(2 * time.Second)
		for {
			if time.Now().After(timeout) {
				return fmt.Errorf("Timed out waiting for status: %s, current status: %s", status, m.cfg.Folders()["default"].Invalid)
			}
			if m.cfg.Folders()["default"].Invalid == status {
				return nil
			}
			time.Sleep(10 * time.Millisecond)
		}
	}

	if err := waitFor("Folder path missing"); err != nil {
		t.Error(err)
		return
	}

	os.Mkdir(fcfg.Path, 0700)

	if err := waitFor("Folder marker missing"); err != nil {
		t.Error(err)
		return
	}

	fd, err := os.Create(filepath.Join(fcfg.Path, ".stfolder"))
	if err != nil {
		t.Error(err)
		return
	}
	fd.Close()

	if err := waitFor(""); err != nil {
		t.Error(err)
		return
	}

	os.Remove(filepath.Join(fcfg.Path, ".stfolder"))

	if err := waitFor("Folder marker missing"); err != nil {
		t.Error(err)
		return
	}

	os.Remove(fcfg.Path)

	if err := waitFor("Folder path missing"); err != nil {
		t.Error(err)
		return
	}
}

func TestGlobalDirectoryTree(t *testing.T) {
	db, _ := leveldb.Open(storage.NewMemStorage(), nil)
	m := NewModel(defaultConfig, protocol.LocalDeviceID, "device", "syncthing", "dev", db)
	m.AddFolder(defaultFolderConfig)

	b := func(isfile bool, path ...string) protocol.FileInfo {
		var flags uint32 = protocol.FlagDirectory
		blocks := []protocol.BlockInfo{}
		if isfile {
			flags = 0
			blocks = []protocol.BlockInfo{{Offset: 0x0, Size: 0xa, Hash: []uint8{0x2f, 0x72, 0xcc, 0x11, 0xa6, 0xfc, 0xd0, 0x27, 0x1e, 0xce, 0xf8, 0xc6, 0x10, 0x56, 0xee, 0x1e, 0xb1, 0x24, 0x3b, 0xe3, 0x80, 0x5b, 0xf9, 0xa9, 0xdf, 0x98, 0xf9, 0x2f, 0x76, 0x36, 0xb0, 0x5c}}}
		}
		return protocol.FileInfo{
			Name:     filepath.Join(path...),
			Flags:    flags,
			Modified: 0x666,
			Blocks:   blocks,
		}
	}

	filedata := []int64{0x666, 0xa}

	testdata := []protocol.FileInfo{
		b(false, "another"),
		b(false, "another", "directory"),
		b(true, "another", "directory", "afile"),
		b(false, "another", "directory", "with"),
		b(false, "another", "directory", "with", "a"),
		b(true, "another", "directory", "with", "a", "file"),
		b(true, "another", "directory", "with", "file"),
		b(true, "another", "file"),

		b(false, "other"),
		b(false, "other", "rand"),
		b(false, "other", "random"),
		b(false, "other", "random", "dir"),
		b(false, "other", "random", "dirx"),
		b(false, "other", "randomx"),

		b(false, "some"),
		b(false, "some", "directory"),
		b(false, "some", "directory", "with"),
		b(false, "some", "directory", "with", "a"),
		b(true, "some", "directory", "with", "a", "file"),

		b(true, "rootfile"),
	}
	expectedResult := map[string]interface{}{
		"another": map[string]interface{}{
			"directory": map[string]interface{}{
				"afile": filedata,
				"with": map[string]interface{}{
					"a": map[string]interface{}{
						"file": filedata,
					},
					"file": filedata,
				},
			},
			"file": filedata,
		},
		"other": map[string]interface{}{
			"rand": map[string]interface{}{},
			"random": map[string]interface{}{
				"dir":  map[string]interface{}{},
				"dirx": map[string]interface{}{},
			},
			"randomx": map[string]interface{}{},
		},
		"some": map[string]interface{}{
			"directory": map[string]interface{}{
				"with": map[string]interface{}{
					"a": map[string]interface{}{
						"file": filedata,
					},
				},
			},
		},
		"rootfile": filedata,
	}

	mm := func(data interface{}) string {
		bytes, err := json.Marshal(data)
		if err != nil {
			panic(err)
		}
		return string(bytes)
	}

	m.Index(device1, "default", testdata, 0, nil)

	result := m.GlobalDirectoryTree("default", "", -1, false)

	if !reflect.DeepEqual(result, expectedResult) {
		t.Errorf("Does not match:\n%s\n%s", mm(result), mm(expectedResult))
	}

	result = m.GlobalDirectoryTree("default", "another", -1, false)

	if !reflect.DeepEqual(result, expectedResult["another"]) {
		t.Errorf("Does not match:\n%s\n%s", mm(result), mm(expectedResult["another"]))
	}

	result = m.GlobalDirectoryTree("default", "", 0, false)
	currentResult := map[string]interface{}{
		"another":  map[string]interface{}{},
		"other":    map[string]interface{}{},
		"some":     map[string]interface{}{},
		"rootfile": filedata,
	}

	if !reflect.DeepEqual(result, currentResult) {
		t.Errorf("Does not match:\n%s\n%s", mm(result), mm(currentResult))
	}

	result = m.GlobalDirectoryTree("default", "", 1, false)
	currentResult = map[string]interface{}{
		"another": map[string]interface{}{
			"directory": map[string]interface{}{},
			"file":      filedata,
		},
		"other": map[string]interface{}{
			"rand":    map[string]interface{}{},
			"random":  map[string]interface{}{},
			"randomx": map[string]interface{}{},
		},
		"some": map[string]interface{}{
			"directory": map[string]interface{}{},
		},
		"rootfile": filedata,
	}

	if !reflect.DeepEqual(result, currentResult) {
		t.Errorf("Does not match:\n%s\n%s", mm(result), mm(currentResult))
	}

	result = m.GlobalDirectoryTree("default", "", -1, true)
	currentResult = map[string]interface{}{
		"another": map[string]interface{}{
			"directory": map[string]interface{}{
				"with": map[string]interface{}{
					"a": map[string]interface{}{},
				},
			},
		},
		"other": map[string]interface{}{
			"rand": map[string]interface{}{},
			"random": map[string]interface{}{
				"dir":  map[string]interface{}{},
				"dirx": map[string]interface{}{},
			},
			"randomx": map[string]interface{}{},
		},
		"some": map[string]interface{}{
			"directory": map[string]interface{}{
				"with": map[string]interface{}{
					"a": map[string]interface{}{},
				},
			},
		},
	}

	if !reflect.DeepEqual(result, currentResult) {
		t.Errorf("Does not match:\n%s\n%s", mm(result), mm(currentResult))
	}

	result = m.GlobalDirectoryTree("default", "", 1, true)
	currentResult = map[string]interface{}{
		"another": map[string]interface{}{
			"directory": map[string]interface{}{},
		},
		"other": map[string]interface{}{
			"rand":    map[string]interface{}{},
			"random":  map[string]interface{}{},
			"randomx": map[string]interface{}{},
		},
		"some": map[string]interface{}{
			"directory": map[string]interface{}{},
		},
	}

	if !reflect.DeepEqual(result, currentResult) {
		t.Errorf("Does not match:\n%s\n%s", mm(result), mm(currentResult))
	}

	result = m.GlobalDirectoryTree("default", "another", 0, false)
	currentResult = map[string]interface{}{
		"directory": map[string]interface{}{},
		"file":      filedata,
	}

	if !reflect.DeepEqual(result, currentResult) {
		t.Errorf("Does not match:\n%s\n%s", mm(result), mm(currentResult))
	}

	result = m.GlobalDirectoryTree("default", "some/directory", 0, false)
	currentResult = map[string]interface{}{
		"with": map[string]interface{}{},
	}

	if !reflect.DeepEqual(result, currentResult) {
		t.Errorf("Does not match:\n%s\n%s", mm(result), mm(currentResult))
	}

	result = m.GlobalDirectoryTree("default", "some/directory", 1, false)
	currentResult = map[string]interface{}{
		"with": map[string]interface{}{
			"a": map[string]interface{}{},
		},
	}

	if !reflect.DeepEqual(result, currentResult) {
		t.Errorf("Does not match:\n%s\n%s", mm(result), mm(currentResult))
	}

	result = m.GlobalDirectoryTree("default", "some/directory", 2, false)
	currentResult = map[string]interface{}{
		"with": map[string]interface{}{
			"a": map[string]interface{}{
				"file": filedata,
			},
		},
	}

	if !reflect.DeepEqual(result, currentResult) {
		t.Errorf("Does not match:\n%s\n%s", mm(result), mm(currentResult))
	}

	result = m.GlobalDirectoryTree("default", "another", -1, true)
	currentResult = map[string]interface{}{
		"directory": map[string]interface{}{
			"with": map[string]interface{}{
				"a": map[string]interface{}{},
			},
		},
	}

	if !reflect.DeepEqual(result, currentResult) {
		t.Errorf("Does not match:\n%s\n%s", mm(result), mm(currentResult))
	}

	// No prefix matching!
	result = m.GlobalDirectoryTree("default", "som", -1, false)
	currentResult = map[string]interface{}{}

	if !reflect.DeepEqual(result, currentResult) {
		t.Errorf("Does not match:\n%s\n%s", mm(result), mm(currentResult))
	}
}

func TestGlobalDirectorySelfFixing(t *testing.T) {

	db, _ := leveldb.Open(storage.NewMemStorage(), nil)
	m := NewModel(defaultConfig, protocol.LocalDeviceID, "device", "syncthing", "dev", db)
	m.AddFolder(defaultFolderConfig)

	b := func(isfile bool, path ...string) protocol.FileInfo {
		var flags uint32 = protocol.FlagDirectory
		blocks := []protocol.BlockInfo{}
		if isfile {
			flags = 0
			blocks = []protocol.BlockInfo{{Offset: 0x0, Size: 0xa, Hash: []uint8{0x2f, 0x72, 0xcc, 0x11, 0xa6, 0xfc, 0xd0, 0x27, 0x1e, 0xce, 0xf8, 0xc6, 0x10, 0x56, 0xee, 0x1e, 0xb1, 0x24, 0x3b, 0xe3, 0x80, 0x5b, 0xf9, 0xa9, 0xdf, 0x98, 0xf9, 0x2f, 0x76, 0x36, 0xb0, 0x5c}}}
		}
		return protocol.FileInfo{
			Name:     filepath.Join(path...),
			Flags:    flags,
			Modified: 0x666,
			Blocks:   blocks,
		}
	}

	filedata := []int64{0x666, 0xa}

	testdata := []protocol.FileInfo{
		b(true, "another", "directory", "afile"),
		b(true, "another", "directory", "with", "a", "file"),
		b(true, "another", "directory", "with", "file"),

		b(false, "other", "random", "dirx"),
		b(false, "other", "randomx"),

		b(false, "some", "directory", "with", "x"),
		b(true, "some", "directory", "with", "a", "file"),

		b(false, "this", "is", "a", "deep", "invalid", "directory"),

		b(true, "xthis", "is", "a", "deep", "invalid", "file"),
	}
	expectedResult := map[string]interface{}{
		"another": map[string]interface{}{
			"directory": map[string]interface{}{
				"afile": filedata,
				"with": map[string]interface{}{
					"a": map[string]interface{}{
						"file": filedata,
					},
					"file": filedata,
				},
			},
		},
		"other": map[string]interface{}{
			"random": map[string]interface{}{
				"dirx": map[string]interface{}{},
			},
			"randomx": map[string]interface{}{},
		},
		"some": map[string]interface{}{
			"directory": map[string]interface{}{
				"with": map[string]interface{}{
					"a": map[string]interface{}{
						"file": filedata,
					},
					"x": map[string]interface{}{},
				},
			},
		},
		"this": map[string]interface{}{
			"is": map[string]interface{}{
				"a": map[string]interface{}{
					"deep": map[string]interface{}{
						"invalid": map[string]interface{}{
							"directory": map[string]interface{}{},
						},
					},
				},
			},
		},
		"xthis": map[string]interface{}{
			"is": map[string]interface{}{
				"a": map[string]interface{}{
					"deep": map[string]interface{}{
						"invalid": map[string]interface{}{
							"file": filedata,
						},
					},
				},
			},
		},
	}

	mm := func(data interface{}) string {
		bytes, err := json.Marshal(data)
		if err != nil {
			panic(err)
		}
		return string(bytes)
	}

	m.Index(device1, "default", testdata, 0, nil)

	result := m.GlobalDirectoryTree("default", "", -1, false)

	if !reflect.DeepEqual(result, expectedResult) {
		t.Errorf("Does not match:\n%s\n%s", mm(result), mm(expectedResult))
	}

	result = m.GlobalDirectoryTree("default", "xthis/is/a/deep", -1, false)
	currentResult := map[string]interface{}{
		"invalid": map[string]interface{}{
			"file": filedata,
		},
	}

	if !reflect.DeepEqual(result, currentResult) {
		t.Errorf("Does not match:\n%s\n%s", mm(result), mm(currentResult))
	}

	result = m.GlobalDirectoryTree("default", "xthis/is/a/deep", -1, true)
	currentResult = map[string]interface{}{
		"invalid": map[string]interface{}{},
	}

	if !reflect.DeepEqual(result, currentResult) {
		t.Errorf("Does not match:\n%s\n%s", mm(result), mm(currentResult))
	}

	// !!! This is actually BAD, because we don't have enough level allowance
	// to accept this file, hence the tree is left unbuilt !!!
	result = m.GlobalDirectoryTree("default", "xthis", 1, false)
	currentResult = map[string]interface{}{}

	if !reflect.DeepEqual(result, currentResult) {
		t.Errorf("Does not match:\n%s\n%s", mm(result), mm(currentResult))
	}
}

func genDeepFiles(n, d int) []protocol.FileInfo {
	rand.Seed(int64(n))
	files := make([]protocol.FileInfo, n)
	t := time.Now().Unix()
	for i := 0; i < n; i++ {
		path := ""
		for i := 0; i <= d; i++ {
			path = filepath.Join(path, strconv.Itoa(rand.Int()))
		}

		sofar := ""
		for _, path := range filepath.SplitList(path) {
			sofar = filepath.Join(sofar, path)
			files[i] = protocol.FileInfo{
				Name: sofar,
			}
			i++
		}

		files[i].Modified = t
		files[i].Blocks = []protocol.BlockInfo{{0, 100, []byte("some hash bytes")}}
	}

	return files
}

func BenchmarkTree_10000_50(b *testing.B) {
	db, _ := leveldb.Open(storage.NewMemStorage(), nil)
	m := NewModel(defaultConfig, protocol.LocalDeviceID, "device", "syncthing", "dev", db)
	m.AddFolder(defaultFolderConfig)
	m.ScanFolder("default")
	files := genDeepFiles(10000, 50)

	m.Index(device1, "default", files, 0, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.GlobalDirectoryTree("default", "", -1, false)
	}
}

func BenchmarkTree_10000_10(b *testing.B) {
	db, _ := leveldb.Open(storage.NewMemStorage(), nil)
	m := NewModel(defaultConfig, protocol.LocalDeviceID, "device", "syncthing", "dev", db)
	m.AddFolder(defaultFolderConfig)
	m.ScanFolder("default")
	files := genDeepFiles(10000, 10)

	m.Index(device1, "default", files, 0, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.GlobalDirectoryTree("default", "", -1, false)
	}
}

func BenchmarkTree_00100_50(b *testing.B) {
	db, _ := leveldb.Open(storage.NewMemStorage(), nil)
	m := NewModel(defaultConfig, protocol.LocalDeviceID, "device", "syncthing", "dev", db)
	m.AddFolder(defaultFolderConfig)
	m.ScanFolder("default")
	files := genDeepFiles(100, 50)

	m.Index(device1, "default", files, 0, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.GlobalDirectoryTree("default", "", -1, false)
	}
}

func BenchmarkTree_00100_10(b *testing.B) {
	db, _ := leveldb.Open(storage.NewMemStorage(), nil)
	m := NewModel(defaultConfig, protocol.LocalDeviceID, "device", "syncthing", "dev", db)
	m.AddFolder(defaultFolderConfig)
	m.ScanFolder("default")
	files := genDeepFiles(100, 10)

	m.Index(device1, "default", files, 0, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.GlobalDirectoryTree("default", "", -1, false)
	}
}
