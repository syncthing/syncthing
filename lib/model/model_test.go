// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package model

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/d4l3k/messagediff"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/connections"
	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/protocol"
)

var device1, device2 protocol.DeviceID
var defaultConfig *config.Wrapper
var defaultFolderConfig config.FolderConfiguration

func init() {
	device1, _ = protocol.DeviceIDFromString("AIR6LPZ-7K4PTTV-UXQSMUU-CPQ5YWH-OEDFIIQ-JUG777G-2YQXXR5-YD6AWQR")
	device2, _ = protocol.DeviceIDFromString("GYRZZQB-IRNPV4Z-T7TC52W-EQYJ3TT-FDQW6MW-DFLMU42-SSSU6EM-FBK2VAY")

	defaultFolderConfig = config.NewFolderConfiguration("default", "testdata")
	defaultFolderConfig.Devices = []config.FolderDeviceConfiguration{{DeviceID: device1}}
	_defaultConfig := config.Configuration{
		Folders: []config.FolderConfiguration{defaultFolderConfig},
		Devices: []config.DeviceConfiguration{config.NewDeviceConfiguration(device1, "device1")},
		Options: config.OptionsConfiguration{
			// Don't remove temporaries directly on startup
			KeepTemporariesH: 1,
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
	db := db.OpenMemory()

	m := NewModel(defaultConfig, protocol.LocalDeviceID, "device", "syncthing", "dev", db, nil)

	// device1 shares default, but device2 doesn't
	m.AddFolder(defaultFolderConfig)
	m.StartFolder("default")
	m.ServeBackground()
	m.ScanFolder("default")

	bs := make([]byte, protocol.BlockSize)

	// Existing, shared file
	bs = bs[:6]
	err := m.Request(device1, "default", "foo", 0, nil, 0, nil, bs)
	if err != nil {
		t.Error(err)
	}
	if !bytes.Equal(bs, []byte("foobar")) {
		t.Errorf("Incorrect data from request: %q", string(bs))
	}

	// Existing, nonshared file
	err = m.Request(device2, "default", "foo", 0, nil, 0, nil, bs)
	if err == nil {
		t.Error("Unexpected nil error on insecure file read")
	}

	// Nonexistent file
	err = m.Request(device1, "default", "nonexistent", 0, nil, 0, nil, bs)
	if err == nil {
		t.Error("Unexpected nil error on insecure file read")
	}

	// Shared folder, but disallowed file name
	err = m.Request(device1, "default", "../walk.go", 0, nil, 0, nil, bs)
	if err == nil {
		t.Error("Unexpected nil error on insecure file read")
	}

	// Negative offset
	err = m.Request(device1, "default", "foo", -4, nil, 0, nil, bs[:0])
	if err == nil {
		t.Error("Unexpected nil error on insecure file read")
	}

	// Larger block than available
	bs = bs[:42]
	err = m.Request(device1, "default", "foo", 0, nil, 0, nil, bs)
	if err == nil {
		t.Error("Unexpected nil error on insecure file read")
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

func BenchmarkIndex_10000(b *testing.B) {
	benchmarkIndex(b, 10000)
}

func BenchmarkIndex_100(b *testing.B) {
	benchmarkIndex(b, 100)
}

func benchmarkIndex(b *testing.B, nfiles int) {
	db := db.OpenMemory()
	m := NewModel(defaultConfig, protocol.LocalDeviceID, "device", "syncthing", "dev", db, nil)
	m.AddFolder(defaultFolderConfig)
	m.StartFolder("default")
	m.ServeBackground()

	files := genFiles(nfiles)
	m.Index(device1, "default", files, 0, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Index(device1, "default", files, 0, nil)
	}
	b.ReportAllocs()
}

func BenchmarkIndexUpdate_10000_10000(b *testing.B) {
	benchmarkIndexUpdate(b, 10000, 10000)
}

func BenchmarkIndexUpdate_10000_100(b *testing.B) {
	benchmarkIndexUpdate(b, 10000, 100)
}

func BenchmarkIndexUpdate_10000_1(b *testing.B) {
	benchmarkIndexUpdate(b, 10000, 1)
}

func benchmarkIndexUpdate(b *testing.B, nfiles, nufiles int) {
	db := db.OpenMemory()
	m := NewModel(defaultConfig, protocol.LocalDeviceID, "device", "syncthing", "dev", db, nil)
	m.AddFolder(defaultFolderConfig)
	m.StartFolder("default")
	m.ServeBackground()

	files := genFiles(nfiles)
	ufiles := genFiles(nufiles)

	m.Index(device1, "default", files, 0, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.IndexUpdate(device1, "default", ufiles, 0, nil)
	}
	b.ReportAllocs()
}

type downloadProgressMessage struct {
	folder  string
	updates []protocol.FileDownloadProgressUpdate
	flags   uint32
	options []protocol.Option
}

type FakeConnection struct {
	id                       protocol.DeviceID
	requestData              []byte
	downloadProgressMessages []downloadProgressMessage
}

func (FakeConnection) Close() error {
	return nil
}

func (f FakeConnection) Start() {
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

func (f FakeConnection) Request(folder, name string, offset int64, size int, hash []byte, fromTemporary bool) ([]byte, error) {
	return f.requestData, nil
}

func (FakeConnection) ClusterConfig(protocol.ClusterConfigMessage) {}

func (FakeConnection) Ping() bool {
	return true
}

func (FakeConnection) Closed() bool {
	return false
}

func (FakeConnection) Statistics() protocol.Statistics {
	return protocol.Statistics{}
}

func (f *FakeConnection) DownloadProgress(folder string, updates []protocol.FileDownloadProgressUpdate, flags uint32, options []protocol.Option) {
	f.downloadProgressMessages = append(f.downloadProgressMessages, downloadProgressMessage{
		folder:  folder,
		updates: updates,
		flags:   flags,
		options: options,
	})
}

func BenchmarkRequest(b *testing.B) {
	db := db.OpenMemory()
	m := NewModel(defaultConfig, protocol.LocalDeviceID, "device", "syncthing", "dev", db, nil)
	m.AddFolder(defaultFolderConfig)
	m.ServeBackground()
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

	fc := &FakeConnection{
		id:          device1,
		requestData: []byte("some data to return"),
	}
	m.AddConnection(connections.Connection{
		IntermediateConnection: connections.IntermediateConnection{
			Conn:     tls.Client(&fakeConn{}, nil),
			Type:     "foo",
			Priority: 10,
		},
		Connection: fc,
	}, protocol.HelloMessage{})
	m.Index(device1, "default", files, 0, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		data, err := m.requestGlobal(device1, "default", files[i%n].Name, 0, 32, nil, false)
		if err != nil {
			b.Error(err)
		}
		if data == nil {
			b.Error("nil data")
		}
	}
}

func TestDeviceRename(t *testing.T) {
	hello := protocol.HelloMessage{
		ClientName:    "syncthing",
		ClientVersion: "v0.9.4",
	}
	defer os.Remove("tmpconfig.xml")

	rawCfg := config.New(device1)
	rawCfg.Devices = []config.DeviceConfiguration{
		{
			DeviceID: device1,
		},
	}
	cfg := config.Wrap("tmpconfig.xml", rawCfg)

	db := db.OpenMemory()
	m := NewModel(cfg, protocol.LocalDeviceID, "device", "syncthing", "dev", db, nil)

	if cfg.Devices()[device1].Name != "" {
		t.Errorf("Device already has a name")
	}

	conn := connections.Connection{
		IntermediateConnection: connections.IntermediateConnection{
			Conn:     tls.Client(&fakeConn{}, nil),
			Type:     "foo",
			Priority: 10,
		},
		Connection: &FakeConnection{
			id:          device1,
			requestData: []byte("some data to return"),
		},
	}

	m.AddConnection(conn, hello)

	m.ServeBackground()

	if cfg.Devices()[device1].Name != "" {
		t.Errorf("Device already has a name")
	}

	m.Close(device1, protocol.ErrTimeout)
	hello.DeviceName = "tester"
	m.AddConnection(conn, hello)

	if cfg.Devices()[device1].Name != "tester" {
		t.Errorf("Device did not get a name")
	}

	m.Close(device1, protocol.ErrTimeout)
	hello.DeviceName = "tester2"
	m.AddConnection(conn, hello)

	if cfg.Devices()[device1].Name != "tester" {
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

	m.Close(device1, protocol.ErrTimeout)

	opts := cfg.Options()
	opts.OverwriteRemoteDevNames = true
	cfg.SetOptions(opts)

	hello.DeviceName = "tester2"
	m.AddConnection(conn, hello)

	if cfg.Devices()[device1].Name != "tester2" {
		t.Errorf("Device name not overwritten")
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

	db := db.OpenMemory()

	m := NewModel(config.Wrap("/tmp/test", cfg), protocol.LocalDeviceID, "device", "syncthing", "dev", db, nil)
	m.AddFolder(cfg.Folders[0])
	m.AddFolder(cfg.Folders[1])
	m.ServeBackground()

	cm := m.generateClusterConfig(device2)

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
	if id := r.Devices[0].ID; !bytes.Equal(id, device1[:]) {
		t.Errorf("Incorrect device ID %x != %x", id, device1)
	}
	if r.Devices[0].Flags&protocol.FlagIntroducer == 0 {
		t.Error("Device1 should be flagged as Introducer")
	}
	if id := r.Devices[1].ID; !bytes.Equal(id, device2[:]) {
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
	if id := r.Devices[0].ID; !bytes.Equal(id, device1[:]) {
		t.Errorf("Incorrect device ID %x != %x", id, device1)
	}
	if r.Devices[0].Flags&protocol.FlagIntroducer == 0 {
		t.Error("Device1 should be flagged as Introducer")
	}
	if id := r.Devices[1].ID; !bytes.Equal(id, device2[:]) {
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

	// Assure a clean start state
	ioutil.WriteFile("testdata/.stfolder", nil, 0644)
	ioutil.WriteFile("testdata/.stignore", []byte(".*\nquux\n"), 0644)

	db := db.OpenMemory()
	m := NewModel(defaultConfig, protocol.LocalDeviceID, "device", "syncthing", "dev", db, nil)
	m.AddFolder(defaultFolderConfig)
	m.StartFolder("default")
	m.ServeBackground()

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

	// Invalid path, marker should be missing, hence returns an error.
	m.AddFolder(config.FolderConfiguration{ID: "fresh", RawPath: "XXX"})
	ignores, _, err = m.GetIgnores("fresh")
	if err == nil {
		t.Error("No error")
	}
}

func TestRefuseUnknownBits(t *testing.T) {
	db := db.OpenMemory()
	m := NewModel(defaultConfig, protocol.LocalDeviceID, "device", "syncthing", "dev", db, nil)
	m.AddFolder(defaultFolderConfig)
	m.ServeBackground()

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
	ldb := db.OpenMemory()
	set := db.NewFileSet("default", ldb)
	set.Update(protocol.LocalDeviceID, []protocol.FileInfo{
		{Name: "dummyfile"},
	})

	fcfg := config.FolderConfiguration{
		ID:              "default",
		RawPath:         "testdata/rotestfolder",
		Type:            config.FolderTypeReadOnly,
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

	os.RemoveAll(fcfg.RawPath)

	m := NewModel(cfg, protocol.LocalDeviceID, "device", "syncthing", "dev", ldb, nil)
	m.AddFolder(fcfg)
	m.StartFolder("default")
	m.ServeBackground()

	waitFor := func(status string) error {
		timeout := time.Now().Add(2 * time.Second)
		for {
			if time.Now().After(timeout) {
				return fmt.Errorf("Timed out waiting for status: %s, current status: %s", status, m.cfg.Folders()["default"].Invalid)
			}
			_, _, err := m.State("default")
			if err == nil && status == "" {
				return nil
			}
			if err != nil && err.Error() == status {
				return nil
			}
			time.Sleep(10 * time.Millisecond)
		}
	}

	if err := waitFor("folder path missing"); err != nil {
		t.Error(err)
		return
	}

	os.Mkdir(fcfg.RawPath, 0700)

	if err := waitFor("folder marker missing"); err != nil {
		t.Error(err)
		return
	}

	fd, err := os.Create(filepath.Join(fcfg.RawPath, ".stfolder"))
	if err != nil {
		t.Error(err)
		return
	}
	fd.Close()

	if err := waitFor(""); err != nil {
		t.Error(err)
		return
	}

	os.Remove(filepath.Join(fcfg.RawPath, ".stfolder"))

	if err := waitFor("folder marker missing"); err != nil {
		t.Error(err)
		return
	}

	os.Remove(fcfg.RawPath)

	if err := waitFor("folder path missing"); err != nil {
		t.Error(err)
		return
	}
}

func TestRWScanRecovery(t *testing.T) {
	ldb := db.OpenMemory()
	set := db.NewFileSet("default", ldb)
	set.Update(protocol.LocalDeviceID, []protocol.FileInfo{
		{Name: "dummyfile"},
	})

	fcfg := config.FolderConfiguration{
		ID:              "default",
		RawPath:         "testdata/rwtestfolder",
		Type:            config.FolderTypeReadWrite,
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

	os.RemoveAll(fcfg.RawPath)

	m := NewModel(cfg, protocol.LocalDeviceID, "device", "syncthing", "dev", ldb, nil)
	m.AddFolder(fcfg)
	m.StartFolder("default")
	m.ServeBackground()

	waitFor := func(status string) error {
		timeout := time.Now().Add(2 * time.Second)
		for {
			if time.Now().After(timeout) {
				return fmt.Errorf("Timed out waiting for status: %s, current status: %s", status, m.cfg.Folders()["default"].Invalid)
			}
			_, _, err := m.State("default")
			if err == nil && status == "" {
				return nil
			}
			if err != nil && err.Error() == status {
				return nil
			}
			time.Sleep(10 * time.Millisecond)
		}
	}

	if err := waitFor("folder path missing"); err != nil {
		t.Error(err)
		return
	}

	os.Mkdir(fcfg.RawPath, 0700)

	if err := waitFor("folder marker missing"); err != nil {
		t.Error(err)
		return
	}

	fd, err := os.Create(filepath.Join(fcfg.RawPath, ".stfolder"))
	if err != nil {
		t.Error(err)
		return
	}
	fd.Close()

	if err := waitFor(""); err != nil {
		t.Error(err)
		return
	}

	os.Remove(filepath.Join(fcfg.RawPath, ".stfolder"))

	if err := waitFor("folder marker missing"); err != nil {
		t.Error(err)
		return
	}

	os.Remove(fcfg.RawPath)

	if err := waitFor("folder path missing"); err != nil {
		t.Error(err)
		return
	}
}

func TestGlobalDirectoryTree(t *testing.T) {
	db := db.OpenMemory()
	m := NewModel(defaultConfig, protocol.LocalDeviceID, "device", "syncthing", "dev", db, nil)
	m.AddFolder(defaultFolderConfig)
	m.ServeBackground()

	b := func(isfile bool, path ...string) protocol.FileInfo {
		flags := uint32(protocol.FlagDirectory)
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

	filedata := []interface{}{time.Unix(0x666, 0), 0xa}

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

	if mm(result) != mm(expectedResult) {
		t.Errorf("Does not match:\n%#v\n%#v", result, expectedResult)
	}

	result = m.GlobalDirectoryTree("default", "another", -1, false)

	if mm(result) != mm(expectedResult["another"]) {
		t.Errorf("Does not match:\n%s\n%s", mm(result), mm(expectedResult["another"]))
	}

	result = m.GlobalDirectoryTree("default", "", 0, false)
	currentResult := map[string]interface{}{
		"another":  map[string]interface{}{},
		"other":    map[string]interface{}{},
		"some":     map[string]interface{}{},
		"rootfile": filedata,
	}

	if mm(result) != mm(currentResult) {
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

	if mm(result) != mm(currentResult) {
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

	if mm(result) != mm(currentResult) {
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

	if mm(result) != mm(currentResult) {
		t.Errorf("Does not match:\n%s\n%s", mm(result), mm(currentResult))
	}

	result = m.GlobalDirectoryTree("default", "another", 0, false)
	currentResult = map[string]interface{}{
		"directory": map[string]interface{}{},
		"file":      filedata,
	}

	if mm(result) != mm(currentResult) {
		t.Errorf("Does not match:\n%s\n%s", mm(result), mm(currentResult))
	}

	result = m.GlobalDirectoryTree("default", "some/directory", 0, false)
	currentResult = map[string]interface{}{
		"with": map[string]interface{}{},
	}

	if mm(result) != mm(currentResult) {
		t.Errorf("Does not match:\n%s\n%s", mm(result), mm(currentResult))
	}

	result = m.GlobalDirectoryTree("default", "some/directory", 1, false)
	currentResult = map[string]interface{}{
		"with": map[string]interface{}{
			"a": map[string]interface{}{},
		},
	}

	if mm(result) != mm(currentResult) {
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

	if mm(result) != mm(currentResult) {
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

	if mm(result) != mm(currentResult) {
		t.Errorf("Does not match:\n%s\n%s", mm(result), mm(currentResult))
	}

	// No prefix matching!
	result = m.GlobalDirectoryTree("default", "som", -1, false)
	currentResult = map[string]interface{}{}

	if mm(result) != mm(currentResult) {
		t.Errorf("Does not match:\n%s\n%s", mm(result), mm(currentResult))
	}
}

func TestGlobalDirectorySelfFixing(t *testing.T) {
	db := db.OpenMemory()
	m := NewModel(defaultConfig, protocol.LocalDeviceID, "device", "syncthing", "dev", db, nil)
	m.AddFolder(defaultFolderConfig)
	m.ServeBackground()

	b := func(isfile bool, path ...string) protocol.FileInfo {
		flags := uint32(protocol.FlagDirectory)
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

	filedata := []interface{}{time.Unix(0x666, 0).Format(time.RFC3339), 0xa}

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

	if mm(result) != mm(expectedResult) {
		t.Errorf("Does not match:\n%s\n%s", mm(result), mm(expectedResult))
	}

	result = m.GlobalDirectoryTree("default", "xthis/is/a/deep", -1, false)
	currentResult := map[string]interface{}{
		"invalid": map[string]interface{}{
			"file": filedata,
		},
	}

	if mm(result) != mm(currentResult) {
		t.Errorf("Does not match:\n%s\n%s", mm(result), mm(currentResult))
	}

	result = m.GlobalDirectoryTree("default", "xthis/is/a/deep", -1, true)
	currentResult = map[string]interface{}{
		"invalid": map[string]interface{}{},
	}

	if mm(result) != mm(currentResult) {
		t.Errorf("Does not match:\n%s\n%s", mm(result), mm(currentResult))
	}

	// !!! This is actually BAD, because we don't have enough level allowance
	// to accept this file, hence the tree is left unbuilt !!!
	result = m.GlobalDirectoryTree("default", "xthis", 1, false)
	currentResult = map[string]interface{}{}

	if mm(result) != mm(currentResult) {
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
	benchmarkTree(b, 10000, 50)
}

func BenchmarkTree_100_50(b *testing.B) {
	benchmarkTree(b, 100, 50)
}

func BenchmarkTree_100_10(b *testing.B) {
	benchmarkTree(b, 100, 10)
}

func benchmarkTree(b *testing.B, n1, n2 int) {
	db := db.OpenMemory()
	m := NewModel(defaultConfig, protocol.LocalDeviceID, "device", "syncthing", "dev", db, nil)
	m.AddFolder(defaultFolderConfig)
	m.ServeBackground()

	m.ScanFolder("default")
	files := genDeepFiles(n1, n2)

	m.Index(device1, "default", files, 0, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.GlobalDirectoryTree("default", "", -1, false)
	}
	b.ReportAllocs()
}

func TestIgnoreDelete(t *testing.T) {
	db := db.OpenMemory()
	m := NewModel(defaultConfig, protocol.LocalDeviceID, "device", "syncthing", "dev", db, nil)

	// This folder should ignore external deletes
	cfg := defaultFolderConfig
	cfg.IgnoreDelete = true

	m.AddFolder(cfg)
	m.ServeBackground()
	m.StartFolder("default")
	m.ScanFolder("default")

	// Get a currently existing file
	f, ok := m.CurrentGlobalFile("default", "foo")
	if !ok {
		t.Fatal("foo should exist")
	}

	// Mark it for deletion
	f.Flags = protocol.FlagDeleted
	f.Version = f.Version.Update(142) // arbitrary short remote ID
	f.Blocks = nil

	// Send the index
	m.Index(device1, "default", []protocol.FileInfo{f}, 0, nil)

	// Make sure we ignored it
	f, ok = m.CurrentGlobalFile("default", "foo")
	if !ok {
		t.Fatal("foo should exist")
	}
	if f.IsDeleted() {
		t.Fatal("foo should not be marked for deletion")
	}
}

func TestUnifySubs(t *testing.T) {
	cases := []struct {
		in     []string // input to unifySubs
		exists []string // paths that exist in the database
		out    []string // expected output
	}{
		{
			// 0. trailing slashes are cleaned, known paths are just passed on
			[]string{"foo/", "bar//"},
			[]string{"foo", "bar"},
			[]string{"bar", "foo"}, // the output is sorted
		},
		{
			// 1. "foo/bar" gets trimmed as it's covered by foo
			[]string{"foo", "bar/", "foo/bar/"},
			[]string{"foo", "bar"},
			[]string{"bar", "foo"},
		},
		{
			// 2. "" gets simplified to the empty list; ie scan all
			[]string{"foo", ""},
			[]string{"foo"},
			nil,
		},
		{
			// 3. "foo/bar" is unknown, but it's kept
			// because its parent is known
			[]string{"foo/bar"},
			[]string{"foo"},
			[]string{"foo/bar"},
		},
		{
			// 4. two independent known paths, both are kept
			// "usr/lib" is not a prefix of "usr/libexec"
			[]string{"usr/lib", "usr/libexec"},
			[]string{"usr", "usr/lib", "usr/libexec"},
			[]string{"usr/lib", "usr/libexec"},
		},
		{
			// 5. "usr/lib" is a prefix of "usr/lib/exec"
			[]string{"usr/lib", "usr/lib/exec"},
			[]string{"usr", "usr/lib", "usr/libexec"},
			[]string{"usr/lib"},
		},
		{
			// 6. .stignore and .stfolder are special and are passed on
			// verbatim even though they are unknown
			[]string{".stfolder", ".stignore"},
			[]string{},
			[]string{".stfolder", ".stignore"},
		},
		{
			// 7. but the presence of something else unknown forces an actual
			// scan
			[]string{".stfolder", ".stignore", "foo/bar"},
			[]string{},
			[]string{".stfolder", ".stignore", "foo"},
		},
		{
			// 8. explicit request to scan all
			nil,
			[]string{"foo"},
			nil,
		},
		{
			// 9. empty list of subs
			[]string{},
			[]string{"foo"},
			nil,
		},
	}

	if runtime.GOOS == "windows" {
		// Fixup path separators
		for i := range cases {
			for j, p := range cases[i].in {
				cases[i].in[j] = filepath.FromSlash(p)
			}
			for j, p := range cases[i].exists {
				cases[i].exists[j] = filepath.FromSlash(p)
			}
			for j, p := range cases[i].out {
				cases[i].out[j] = filepath.FromSlash(p)
			}
		}
	}

	for i, tc := range cases {
		exists := func(f string) bool {
			for _, e := range tc.exists {
				if f == e {
					return true
				}
			}
			return false
		}

		out := unifySubs(tc.in, exists)
		if diff, equal := messagediff.PrettyDiff(tc.out, out); !equal {
			t.Errorf("Case %d failed; got %v, expected %v, diff:\n%s", i, out, tc.out, diff)
		}
	}
}

func TestIssue3028(t *testing.T) {
	// Create two files that we'll delete, one with a name that is a prefix of the other.

	if err := ioutil.WriteFile("testdata/testrm", []byte("Hello"), 0644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove("testdata/testrm")
	if err := ioutil.WriteFile("testdata/testrm2", []byte("Hello"), 0644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove("testdata/testrm2")

	// Create a model and default folder

	db := db.OpenMemory()
	m := NewModel(defaultConfig, protocol.LocalDeviceID, "device", "syncthing", "dev", db, nil)
	defCfg := defaultFolderConfig.Copy()
	defCfg.RescanIntervalS = 86400
	m.AddFolder(defCfg)
	m.StartFolder("default")
	m.ServeBackground()

	// Ugly hack for testing: reach into the model for the rwfolder and wait
	// for it to complete the initial scan. The risk is that it otherwise
	// runs during our modifications and screws up the test.
	m.fmut.RLock()
	folder := m.folderRunners["default"].(*rwFolder)
	m.fmut.RUnlock()
	<-folder.initialScanCompleted

	// Get a count of how many files are there now

	locorigfiles, _, _ := m.LocalSize("default")
	globorigfiles, _, _ := m.GlobalSize("default")

	// Delete and rescan specifically these two

	os.Remove("testdata/testrm")
	os.Remove("testdata/testrm2")
	m.ScanFolderSubs("default", []string{"testrm", "testrm2"})

	// Verify that the number of files decreased by two and the number of
	// deleted files increases by two

	locnowfiles, locdelfiles, _ := m.LocalSize("default")
	globnowfiles, globdelfiles, _ := m.GlobalSize("default")
	if locnowfiles != locorigfiles-2 {
		t.Errorf("Incorrect local accounting; got %d current files, expected %d", locnowfiles, locorigfiles-2)
	}
	if globnowfiles != globorigfiles-2 {
		t.Errorf("Incorrect global accounting; got %d current files, expected %d", globnowfiles, globorigfiles-2)
	}
	if locdelfiles != 2 {
		t.Errorf("Incorrect local accounting; got %d deleted files, expected 2", locdelfiles)
	}
	if globdelfiles != 2 {
		t.Errorf("Incorrect global accounting; got %d deleted files, expected 2", globdelfiles)
	}
}

type fakeAddr struct{}

func (fakeAddr) Network() string {
	return "network"
}

func (fakeAddr) String() string {
	return "address"
}

type fakeConn struct{}

func (fakeConn) Close() error {
	return nil
}

func (fakeConn) LocalAddr() net.Addr {
	return &fakeAddr{}
}

func (fakeConn) RemoteAddr() net.Addr {
	return &fakeAddr{}
}

func (fakeConn) Read([]byte) (int, error) {
	return 0, nil
}

func (fakeConn) Write([]byte) (int, error) {
	return 0, nil
}

func (fakeConn) SetDeadline(time.Time) error {
	return nil
}

func (fakeConn) SetReadDeadline(time.Time) error {
	return nil
}

func (fakeConn) SetWriteDeadline(time.Time) error {
	return nil
}
