// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/ignore"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/protocol"
	srand "github.com/syncthing/syncthing/lib/rand"
	"github.com/syncthing/syncthing/lib/scanner"
	"github.com/syncthing/syncthing/lib/versioner"
)

var device1, device2 protocol.DeviceID
var defaultCfgWrapper *config.Wrapper
var defaultFolderConfig config.FolderConfiguration
var defaultFs fs.Filesystem
var defaultCfg config.Configuration
var defaultAutoAcceptCfg config.Configuration
var tmpLocation string

func init() {
	device1, _ = protocol.DeviceIDFromString("AIR6LPZ-7K4PTTV-UXQSMUU-CPQ5YWH-OEDFIIQ-JUG777G-2YQXXR5-YD6AWQR")
	device2, _ = protocol.DeviceIDFromString("GYRZZQB-IRNPV4Z-T7TC52W-EQYJ3TT-FDQW6MW-DFLMU42-SSSU6EM-FBK2VAY")
	defaultFs = fs.NewFilesystem(fs.FilesystemTypeBasic, "testdata")

	defaultFolderConfig = config.NewFolderConfiguration(protocol.LocalDeviceID, "default", "default", fs.FilesystemTypeBasic, "testdata")
	defaultFolderConfig.Devices = []config.FolderDeviceConfiguration{{DeviceID: device1}}
	defaultCfg = config.Configuration{
		Version: config.CurrentVersion,
		Folders: []config.FolderConfiguration{defaultFolderConfig},
		Devices: []config.DeviceConfiguration{config.NewDeviceConfiguration(device1, "device1")},
		Options: config.OptionsConfiguration{
			// Don't remove temporaries directly on startup
			KeepTemporariesH: 1,
		},
	}
	defaultAutoAcceptCfg = config.Configuration{
		Devices: []config.DeviceConfiguration{
			{
				DeviceID: protocol.LocalDeviceID, // self
			},
			{
				DeviceID:          device1,
				AutoAcceptFolders: true,
			},
			{
				DeviceID:          device2,
				AutoAcceptFolders: true,
			},
		},
		Options: config.OptionsConfiguration{
			DefaultFolderPath: "testdata",
		},
	}
}

var testDataExpected = map[string]protocol.FileInfo{
	"foo": {
		Name:      "foo",
		Type:      protocol.FileInfoTypeFile,
		ModifiedS: 0,
		Blocks:    []protocol.BlockInfo{{Offset: 0x0, Size: 0x7, Hash: []uint8{0xae, 0xc0, 0x70, 0x64, 0x5f, 0xe5, 0x3e, 0xe3, 0xb3, 0x76, 0x30, 0x59, 0x37, 0x61, 0x34, 0xf0, 0x58, 0xcc, 0x33, 0x72, 0x47, 0xc9, 0x78, 0xad, 0xd1, 0x78, 0xb6, 0xcc, 0xdf, 0xb0, 0x1, 0x9f}}},
	},
	"empty": {
		Name:      "empty",
		Type:      protocol.FileInfoTypeFile,
		ModifiedS: 0,
		Blocks:    []protocol.BlockInfo{{Offset: 0x0, Size: 0x0, Hash: []uint8{0xe3, 0xb0, 0xc4, 0x42, 0x98, 0xfc, 0x1c, 0x14, 0x9a, 0xfb, 0xf4, 0xc8, 0x99, 0x6f, 0xb9, 0x24, 0x27, 0xae, 0x41, 0xe4, 0x64, 0x9b, 0x93, 0x4c, 0xa4, 0x95, 0x99, 0x1b, 0x78, 0x52, 0xb8, 0x55}}},
	},
	"bar": {
		Name:      "bar",
		Type:      protocol.FileInfoTypeFile,
		ModifiedS: 0,
		Blocks:    []protocol.BlockInfo{{Offset: 0x0, Size: 0xa, Hash: []uint8{0x2f, 0x72, 0xcc, 0x11, 0xa6, 0xfc, 0xd0, 0x27, 0x1e, 0xce, 0xf8, 0xc6, 0x10, 0x56, 0xee, 0x1e, 0xb1, 0x24, 0x3b, 0xe3, 0x80, 0x5b, 0xf9, 0xa9, 0xdf, 0x98, 0xf9, 0x2f, 0x76, 0x36, 0xb0, 0x5c}}},
	},
}

func init() {
	// Fix expected test data to match reality
	for n, f := range testDataExpected {
		fi, _ := os.Stat("testdata/" + n)
		f.Permissions = uint32(fi.Mode())
		f.ModifiedS = fi.ModTime().Unix()
		f.Size = fi.Size()
		testDataExpected[n] = f
	}
}

func TestMain(m *testing.M) {
	tmpLocation = "/tmp"
	if runtime.GOOS == "windows" {
		tmpLocation = filepath.Join("testdata", "tmp")
		if err := os.MkdirAll(tmpLocation, 0777); err != nil {
			panic(err)
		}
	}

	tmpName := fs.TempName("file")
	if err := osutil.Copy(defaultFs, "tmpfile", tmpName); err != nil {
		panic(err)
	}
	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(filepath.Join("testdata", tmpName), future, future); err != nil {
		panic(err)
	}

	defaultCfgWrapper = createTmpWrapper(defaultCfg)

	exitCode := m.Run()

	os.Remove(defaultCfgWrapper.ConfigPath())
	defaultFs.Remove(tmpName)
	defaultFs.RemoveAll(config.DefaultMarkerName)
	defaultFs.RemoveAll(tmpLocation)

	os.Exit(exitCode)
}

func createTmpWrapper(cfg config.Configuration) *config.Wrapper {
	tmpFile, err := ioutil.TempFile(tmpLocation, "syncthing-testConfig-")
	if err != nil {
		panic(err)
	}
	wrapper := config.Wrap(tmpFile.Name(), cfg)
	tmpFile.Close()
	return wrapper
}

func newState(cfg config.Configuration) (*config.Wrapper, *Model) {
	db := db.OpenMemory()

	wcfg := createTmpWrapper(cfg)

	m := NewModel(wcfg, protocol.LocalDeviceID, "syncthing", "dev", db, nil)
	for _, folder := range cfg.Folders {
		if !folder.Paused {
			m.AddFolder(folder)
			m.StartFolder(folder.ID)
		}
	}
	m.ServeBackground()

	for _, dev := range cfg.Devices {
		m.AddConnection(&fakeConnection{id: dev.DeviceID}, protocol.HelloResult{})
	}
	return wcfg, m
}

func TestRequest(t *testing.T) {
	db := db.OpenMemory()

	m := NewModel(defaultCfgWrapper, protocol.LocalDeviceID, "syncthing", "dev", db, nil)

	// device1 shares default, but device2 doesn't
	m.AddFolder(defaultFolderConfig)
	m.StartFolder("default")
	m.ServeBackground()
	defer m.Stop()
	m.ScanFolder("default")

	// Existing, shared file
	res, err := m.Request(device1, "default", "foo", 6, 0, nil, 0, false)
	if err != nil {
		t.Error(err)
	}
	bs := res.Data()
	if !bytes.Equal(bs, []byte("foobar")) {
		t.Errorf("Incorrect data from request: %q", string(bs))
	}

	// Existing, nonshared file
	_, err = m.Request(device2, "default", "foo", 6, 0, nil, 0, false)
	if err == nil {
		t.Error("Unexpected nil error on insecure file read")
	}

	// Nonexistent file
	_, err = m.Request(device1, "default", "nonexistent", 6, 0, nil, 0, false)
	if err == nil {
		t.Error("Unexpected nil error on insecure file read")
	}

	// Shared folder, but disallowed file name
	_, err = m.Request(device1, "default", "../walk.go", 6, 0, nil, 0, false)
	if err == nil {
		t.Error("Unexpected nil error on insecure file read")
	}

	// Negative offset
	_, err = m.Request(device1, "default", "foo", -4, 0, nil, 0, false)
	if err == nil {
		t.Error("Unexpected nil error on insecure file read")
	}

	// Larger block than available
	_, err = m.Request(device1, "default", "foo", 42, 0, nil, 0, false)
	if err == nil {
		t.Error("Unexpected nil error on insecure file read")
	}
}

func genFiles(n int) []protocol.FileInfo {
	files := make([]protocol.FileInfo, n)
	t := time.Now().Unix()
	for i := 0; i < n; i++ {
		files[i] = protocol.FileInfo{
			Name:      fmt.Sprintf("file%d", i),
			ModifiedS: t,
			Sequence:  int64(i + 1),
			Blocks:    []protocol.BlockInfo{{Offset: 0, Size: 100, Hash: []byte("some hash bytes")}},
			Version:   protocol.Vector{Counters: []protocol.Counter{{ID: 42, Value: 1}}},
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
	m := NewModel(defaultCfgWrapper, protocol.LocalDeviceID, "syncthing", "dev", db, nil)
	m.AddFolder(defaultFolderConfig)
	m.StartFolder("default")
	m.ServeBackground()
	defer m.Stop()

	files := genFiles(nfiles)
	m.Index(device1, "default", files)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Index(device1, "default", files)
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
	m := NewModel(defaultCfgWrapper, protocol.LocalDeviceID, "syncthing", "dev", db, nil)
	m.AddFolder(defaultFolderConfig)
	m.StartFolder("default")
	m.ServeBackground()
	defer m.Stop()

	files := genFiles(nfiles)
	ufiles := genFiles(nufiles)

	m.Index(device1, "default", files)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.IndexUpdate(device1, "default", ufiles)
	}
	b.ReportAllocs()
}

type downloadProgressMessage struct {
	folder  string
	updates []protocol.FileDownloadProgressUpdate
}

type fakeConnection struct {
	id                       protocol.DeviceID
	downloadProgressMessages []downloadProgressMessage
	closed                   bool
	files                    []protocol.FileInfo
	fileData                 map[string][]byte
	folder                   string
	model                    *Model
	indexFn                  func(string, []protocol.FileInfo)
	requestFn                func(folder, name string, offset int64, size int, hash []byte, fromTemporary bool) ([]byte, error)
	mut                      sync.Mutex
}

func (f *fakeConnection) Close() error {
	f.mut.Lock()
	defer f.mut.Unlock()
	f.closed = true
	return nil
}

func (f *fakeConnection) Start() {
}

func (f *fakeConnection) ID() protocol.DeviceID {
	return f.id
}

func (f *fakeConnection) Name() string {
	return ""
}

func (f *fakeConnection) String() string {
	return ""
}

func (f *fakeConnection) Option(string) string {
	return ""
}

func (f *fakeConnection) Index(folder string, fs []protocol.FileInfo) error {
	f.mut.Lock()
	defer f.mut.Unlock()
	if f.indexFn != nil {
		f.indexFn(folder, fs)
	}
	return nil
}

func (f *fakeConnection) IndexUpdate(folder string, fs []protocol.FileInfo) error {
	f.mut.Lock()
	defer f.mut.Unlock()
	if f.indexFn != nil {
		f.indexFn(folder, fs)
	}
	return nil
}

func (f *fakeConnection) Request(folder, name string, offset int64, size int, hash []byte, weakHash uint32, fromTemporary bool) ([]byte, error) {
	f.mut.Lock()
	defer f.mut.Unlock()
	if f.requestFn != nil {
		return f.requestFn(folder, name, offset, size, hash, fromTemporary)
	}
	return f.fileData[name], nil
}

func (f *fakeConnection) ClusterConfig(protocol.ClusterConfig) {}

func (f *fakeConnection) Ping() bool {
	f.mut.Lock()
	defer f.mut.Unlock()
	return f.closed
}

func (f *fakeConnection) Closed() bool {
	f.mut.Lock()
	defer f.mut.Unlock()
	return f.closed
}

func (f *fakeConnection) Statistics() protocol.Statistics {
	return protocol.Statistics{}
}

func (f *fakeConnection) RemoteAddr() net.Addr {
	return &fakeAddr{}
}

func (f *fakeConnection) Type() string {
	return "fake"
}
func (f *fakeConnection) Transport() string {
	return "fake"
}
func (f *fakeConnection) Priority() int {
	return 9000
}

func (f *fakeConnection) DownloadProgress(folder string, updates []protocol.FileDownloadProgressUpdate) {
	f.downloadProgressMessages = append(f.downloadProgressMessages, downloadProgressMessage{
		folder:  folder,
		updates: updates,
	})
}

func (f *fakeConnection) addFileLocked(name string, flags uint32, ftype protocol.FileInfoType, data []byte, version protocol.Vector) {
	blockSize := protocol.BlockSize(int64(len(data)))
	blocks, _ := scanner.Blocks(context.TODO(), bytes.NewReader(data), blockSize, int64(len(data)), nil, true)

	if ftype == protocol.FileInfoTypeFile || ftype == protocol.FileInfoTypeDirectory {
		f.files = append(f.files, protocol.FileInfo{
			Name:         name,
			Type:         ftype,
			Size:         int64(len(data)),
			ModifiedS:    time.Now().Unix(),
			Permissions:  flags,
			Version:      version,
			Sequence:     time.Now().UnixNano(),
			RawBlockSize: int32(blockSize),
			Blocks:       blocks,
		})
	} else {
		// Symlink
		f.files = append(f.files, protocol.FileInfo{
			Name:          name,
			Type:          ftype,
			Version:       version,
			Sequence:      time.Now().UnixNano(),
			SymlinkTarget: string(data),
			NoPermissions: true,
		})
	}

	if f.fileData == nil {
		f.fileData = make(map[string][]byte)
	}
	f.fileData[name] = data
}
func (f *fakeConnection) addFile(name string, flags uint32, ftype protocol.FileInfoType, data []byte) {
	f.mut.Lock()
	defer f.mut.Unlock()

	var version protocol.Vector
	version = version.Update(f.id.Short())
	f.addFileLocked(name, flags, ftype, data, version)
}

func (f *fakeConnection) updateFile(name string, flags uint32, ftype protocol.FileInfoType, data []byte) {
	f.mut.Lock()
	defer f.mut.Unlock()

	for i, fi := range f.files {
		if fi.Name == name {
			f.files = append(f.files[:i], f.files[i+1:]...)
			f.addFileLocked(name, flags, ftype, data, fi.Version.Update(f.id.Short()))
			return
		}
	}
}

func (f *fakeConnection) deleteFile(name string) {
	f.mut.Lock()
	defer f.mut.Unlock()

	for i, fi := range f.files {
		if fi.Name == name {
			fi.Deleted = true
			fi.ModifiedS = time.Now().Unix()
			fi.Version = fi.Version.Update(f.id.Short())
			fi.Sequence = time.Now().UnixNano()
			fi.Blocks = nil

			f.files = append(append(f.files[:i], f.files[i+1:]...), fi)
			return
		}
	}
}

func (f *fakeConnection) sendIndexUpdate() {
	f.model.IndexUpdate(f.id, f.folder, f.files)
}

func BenchmarkRequestOut(b *testing.B) {
	db := db.OpenMemory()
	m := NewModel(defaultCfgWrapper, protocol.LocalDeviceID, "syncthing", "dev", db, nil)
	m.AddFolder(defaultFolderConfig)
	m.ServeBackground()
	defer m.Stop()
	m.ScanFolder("default")

	const n = 1000
	files := genFiles(n)

	fc := &fakeConnection{id: device1}
	for _, f := range files {
		fc.addFile(f.Name, 0644, protocol.FileInfoTypeFile, []byte("some data to return"))
	}
	m.AddConnection(fc, protocol.HelloResult{})
	m.Index(device1, "default", files)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		data, err := m.requestGlobal(device1, "default", files[i%n].Name, 0, 32, nil, 0, false)
		if err != nil {
			b.Error(err)
		}
		if data == nil {
			b.Error("nil data")
		}
	}
}

func BenchmarkRequestInSingleFile(b *testing.B) {
	db := db.OpenMemory()
	m := NewModel(defaultCfgWrapper, protocol.LocalDeviceID, "syncthing", "dev", db, nil)
	m.AddFolder(defaultFolderConfig)
	m.ServeBackground()
	defer m.Stop()
	m.ScanFolder("default")

	buf := make([]byte, 128<<10)
	rand.Read(buf)
	os.RemoveAll("testdata/request")
	defer os.RemoveAll("testdata/request")
	os.MkdirAll("testdata/request/for/a/file/in/a/couple/of/dirs", 0755)
	ioutil.WriteFile("testdata/request/for/a/file/in/a/couple/of/dirs/128k", buf, 0644)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := m.Request(device1, "default", "request/for/a/file/in/a/couple/of/dirs/128k", 128<<10, 0, nil, 0, false); err != nil {
			b.Error(err)
		}
	}

	b.SetBytes(128 << 10)
}

func TestDeviceRename(t *testing.T) {
	hello := protocol.HelloResult{
		ClientName:    "syncthing",
		ClientVersion: "v0.9.4",
	}
	defer os.Remove("testdata/tmpconfig.xml")

	rawCfg := config.New(device1)
	rawCfg.Devices = []config.DeviceConfiguration{
		{
			DeviceID: device1,
		},
	}
	cfg := config.Wrap("testdata/tmpconfig.xml", rawCfg)

	db := db.OpenMemory()
	m := NewModel(cfg, protocol.LocalDeviceID, "syncthing", "dev", db, nil)

	if cfg.Devices()[device1].Name != "" {
		t.Errorf("Device already has a name")
	}

	conn := &fakeConnection{id: device1}

	m.AddConnection(conn, hello)

	m.ServeBackground()
	defer m.Stop()

	if cfg.Devices()[device1].Name != "" {
		t.Errorf("Device already has a name")
	}

	m.Closed(conn, protocol.ErrTimeout)
	hello.DeviceName = "tester"
	m.AddConnection(conn, hello)

	if cfg.Devices()[device1].Name != "tester" {
		t.Errorf("Device did not get a name")
	}

	m.Closed(conn, protocol.ErrTimeout)
	hello.DeviceName = "tester2"
	m.AddConnection(conn, hello)

	if cfg.Devices()[device1].Name != "tester" {
		t.Errorf("Device name got overwritten")
	}

	cfgw, err := config.Load("testdata/tmpconfig.xml", protocol.LocalDeviceID)
	if err != nil {
		t.Error(err)
		return
	}
	if cfgw.Devices()[device1].Name != "tester" {
		t.Errorf("Device name not saved in config")
	}

	m.Closed(conn, protocol.ErrTimeout)

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
			ID:   "folder1",
			Path: "testdata1",
			Devices: []config.FolderDeviceConfiguration{
				{DeviceID: device1},
				{DeviceID: device2},
			},
		},
		{
			ID:     "folder2",
			Path:   "testdata2",
			Paused: true, // should still be included
			Devices: []config.FolderDeviceConfiguration{
				{DeviceID: device1},
				{DeviceID: device2},
			},
		},
		{
			ID:   "folder3",
			Path: "testdata3",
			Devices: []config.FolderDeviceConfiguration{
				{DeviceID: device1},
				// should not be included, does not include device2
			},
		},
	}

	db := db.OpenMemory()

	wrapper := createTmpWrapper(cfg)
	defer os.Remove(wrapper.ConfigPath())
	m := NewModel(wrapper, protocol.LocalDeviceID, "syncthing", "dev", db, nil)
	m.AddFolder(cfg.Folders[0])
	m.AddFolder(cfg.Folders[1])
	m.ServeBackground()
	defer m.Stop()

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
	if id := r.Devices[0].ID; id != device1 {
		t.Errorf("Incorrect device ID %s != %s", id, device1)
	}
	if !r.Devices[0].Introducer {
		t.Error("Device1 should be flagged as Introducer")
	}
	if id := r.Devices[1].ID; id != device2 {
		t.Errorf("Incorrect device ID %s != %s", id, device2)
	}
	if r.Devices[1].Introducer {
		t.Error("Device2 should not be flagged as Introducer")
	}

	r = cm.Folders[1]
	if r.ID != "folder2" {
		t.Errorf("Incorrect folder %q != folder2", r.ID)
	}
	if l := len(r.Devices); l != 2 {
		t.Errorf("Incorrect number of devices %d != 2", l)
	}
	if id := r.Devices[0].ID; id != device1 {
		t.Errorf("Incorrect device ID %s != %s", id, device1)
	}
	if !r.Devices[0].Introducer {
		t.Error("Device1 should be flagged as Introducer")
	}
	if id := r.Devices[1].ID; id != device2 {
		t.Errorf("Incorrect device ID %s != %s", id, device2)
	}
	if r.Devices[1].Introducer {
		t.Error("Device2 should not be flagged as Introducer")
	}
}

func TestIntroducer(t *testing.T) {
	var introducedByAnyone protocol.DeviceID

	// LocalDeviceID is a magic value meaning don't check introducer
	contains := func(cfg config.FolderConfiguration, id, introducedBy protocol.DeviceID) bool {
		for _, dev := range cfg.Devices {
			if dev.DeviceID.Equals(id) {
				if introducedBy.Equals(introducedByAnyone) {
					return true
				}
				return dev.IntroducedBy.Equals(introducedBy)
			}
		}
		return false
	}

	wcfg, m := newState(config.Configuration{
		Devices: []config.DeviceConfiguration{
			{
				DeviceID:   device1,
				Introducer: true,
			},
		},
		Folders: []config.FolderConfiguration{
			{
				ID:   "folder1",
				Path: "testdata",
				Devices: []config.FolderDeviceConfiguration{
					{DeviceID: device1},
				},
			},
			{
				ID:   "folder2",
				Path: "testdata",
				Devices: []config.FolderDeviceConfiguration{
					{DeviceID: device1},
				},
			},
		},
	})
	defer os.Remove(wcfg.ConfigPath())
	m.ClusterConfig(device1, protocol.ClusterConfig{
		Folders: []protocol.Folder{
			{
				ID: "folder1",
				Devices: []protocol.Device{
					{
						ID:                       device2,
						Introducer:               true,
						SkipIntroductionRemovals: true,
					},
				},
			},
		},
	})

	if newDev, ok := wcfg.Device(device2); !ok || !newDev.Introducer || !newDev.SkipIntroductionRemovals {
		t.Error("devie 2 missing or wrong flags")
	}

	if !contains(wcfg.Folders()["folder1"], device2, device1) {
		t.Error("expected folder 1 to have device2 introduced by device 1")
	}

	wcfg, m = newState(config.Configuration{
		Devices: []config.DeviceConfiguration{
			{
				DeviceID:   device1,
				Introducer: true,
			},
			{
				DeviceID:     device2,
				IntroducedBy: device1,
			},
		},
		Folders: []config.FolderConfiguration{
			{
				ID:   "folder1",
				Path: "testdata",
				Devices: []config.FolderDeviceConfiguration{
					{DeviceID: device1},
					{DeviceID: device2, IntroducedBy: device1},
				},
			},
			{
				ID:   "folder2",
				Path: "testdata",
				Devices: []config.FolderDeviceConfiguration{
					{DeviceID: device1},
				},
			},
		},
	})
	defer os.Remove(wcfg.ConfigPath())
	m.ClusterConfig(device1, protocol.ClusterConfig{
		Folders: []protocol.Folder{
			{
				ID: "folder2",
				Devices: []protocol.Device{
					{
						ID:                       device2,
						Introducer:               true,
						SkipIntroductionRemovals: true,
					},
				},
			},
		},
	})

	// Should not get introducer, as it's already unset, and it's an existing device.
	if newDev, ok := wcfg.Device(device2); !ok || newDev.Introducer || newDev.SkipIntroductionRemovals {
		t.Error("device 2 missing or changed flags")
	}

	if contains(wcfg.Folders()["folder1"], device2, introducedByAnyone) {
		t.Error("expected device 2 to be removed from folder 1")
	}

	if !contains(wcfg.Folders()["folder2"], device2, device1) {
		t.Error("expected device 2 to be added to folder 2")
	}

	wcfg, m = newState(config.Configuration{
		Devices: []config.DeviceConfiguration{
			{
				DeviceID:   device1,
				Introducer: true,
			},
			{
				DeviceID:     device2,
				IntroducedBy: device1,
			},
		},
		Folders: []config.FolderConfiguration{
			{
				ID:   "folder1",
				Path: "testdata",
				Devices: []config.FolderDeviceConfiguration{
					{DeviceID: device1},
					{DeviceID: device2, IntroducedBy: device1},
				},
			},
			{
				ID:   "folder2",
				Path: "testdata",
				Devices: []config.FolderDeviceConfiguration{
					{DeviceID: device1},
					{DeviceID: device2, IntroducedBy: device1},
				},
			},
		},
	})
	defer os.Remove(wcfg.ConfigPath())
	m.ClusterConfig(device1, protocol.ClusterConfig{})

	if _, ok := wcfg.Device(device2); ok {
		t.Error("device 2 should have been removed")
	}

	if contains(wcfg.Folders()["folder1"], device2, introducedByAnyone) {
		t.Error("expected device 2 to be removed from folder 1")
	}

	if contains(wcfg.Folders()["folder2"], device2, introducedByAnyone) {
		t.Error("expected device 2 to be removed from folder 2")
	}

	// Two cases when removals should not happen
	// 1. Introducer flag no longer set on device

	wcfg, m = newState(config.Configuration{
		Devices: []config.DeviceConfiguration{
			{
				DeviceID:   device1,
				Introducer: false,
			},
			{
				DeviceID:     device2,
				IntroducedBy: device1,
			},
		},
		Folders: []config.FolderConfiguration{
			{
				ID:   "folder1",
				Path: "testdata",
				Devices: []config.FolderDeviceConfiguration{
					{DeviceID: device1},
					{DeviceID: device2, IntroducedBy: device1},
				},
			},
			{
				ID:   "folder2",
				Path: "testdata",
				Devices: []config.FolderDeviceConfiguration{
					{DeviceID: device1},
					{DeviceID: device2, IntroducedBy: device1},
				},
			},
		},
	})
	defer os.Remove(wcfg.ConfigPath())
	m.ClusterConfig(device1, protocol.ClusterConfig{})

	if _, ok := wcfg.Device(device2); !ok {
		t.Error("device 2 should not have been removed")
	}

	if !contains(wcfg.Folders()["folder1"], device2, device1) {
		t.Error("expected device 2 not to be removed from folder 1")
	}

	if !contains(wcfg.Folders()["folder2"], device2, device1) {
		t.Error("expected device 2 not to be removed from folder 2")
	}

	// 2. SkipIntroductionRemovals is set

	wcfg, m = newState(config.Configuration{
		Devices: []config.DeviceConfiguration{
			{
				DeviceID:                 device1,
				Introducer:               true,
				SkipIntroductionRemovals: true,
			},
			{
				DeviceID:     device2,
				IntroducedBy: device1,
			},
		},
		Folders: []config.FolderConfiguration{
			{
				ID:   "folder1",
				Path: "testdata",
				Devices: []config.FolderDeviceConfiguration{
					{DeviceID: device1},
					{DeviceID: device2, IntroducedBy: device1},
				},
			},
			{
				ID:   "folder2",
				Path: "testdata",
				Devices: []config.FolderDeviceConfiguration{
					{DeviceID: device1},
				},
			},
		},
	})
	defer os.Remove(wcfg.ConfigPath())
	m.ClusterConfig(device1, protocol.ClusterConfig{
		Folders: []protocol.Folder{
			{
				ID: "folder2",
				Devices: []protocol.Device{
					{
						ID:                       device2,
						Introducer:               true,
						SkipIntroductionRemovals: true,
					},
				},
			},
		},
	})

	if _, ok := wcfg.Device(device2); !ok {
		t.Error("device 2 should not have been removed")
	}

	if !contains(wcfg.Folders()["folder1"], device2, device1) {
		t.Error("expected device 2 not to be removed from folder 1")
	}

	if !contains(wcfg.Folders()["folder2"], device2, device1) {
		t.Error("expected device 2 not to be added to folder 2")
	}

	// Test device not being removed as it's shared without an introducer.

	wcfg, m = newState(config.Configuration{
		Devices: []config.DeviceConfiguration{
			{
				DeviceID:   device1,
				Introducer: true,
			},
			{
				DeviceID:     device2,
				IntroducedBy: device1,
			},
		},
		Folders: []config.FolderConfiguration{
			{
				ID:   "folder1",
				Path: "testdata",
				Devices: []config.FolderDeviceConfiguration{
					{DeviceID: device1},
					{DeviceID: device2, IntroducedBy: device1},
				},
			},
			{
				ID:   "folder2",
				Path: "testdata",
				Devices: []config.FolderDeviceConfiguration{
					{DeviceID: device1},
					{DeviceID: device2},
				},
			},
		},
	})
	defer os.Remove(wcfg.ConfigPath())
	m.ClusterConfig(device1, protocol.ClusterConfig{})

	if _, ok := wcfg.Device(device2); !ok {
		t.Error("device 2 should not have been removed")
	}

	if contains(wcfg.Folders()["folder1"], device2, introducedByAnyone) {
		t.Error("expected device 2 to be removed from folder 1")
	}

	if !contains(wcfg.Folders()["folder2"], device2, introducedByAnyone) {
		t.Error("expected device 2 not to be removed from folder 2")
	}

	// Test device not being removed as it's shared by a different introducer.

	wcfg, m = newState(config.Configuration{
		Devices: []config.DeviceConfiguration{
			{
				DeviceID:   device1,
				Introducer: true,
			},
			{
				DeviceID:     device2,
				IntroducedBy: device1,
			},
		},
		Folders: []config.FolderConfiguration{
			{
				ID:   "folder1",
				Path: "testdata",
				Devices: []config.FolderDeviceConfiguration{
					{DeviceID: device1},
					{DeviceID: device2, IntroducedBy: device1},
				},
			},
			{
				ID:   "folder2",
				Path: "testdata",
				Devices: []config.FolderDeviceConfiguration{
					{DeviceID: device1},
					{DeviceID: device2, IntroducedBy: protocol.LocalDeviceID},
				},
			},
		},
	})
	defer os.Remove(wcfg.ConfigPath())
	m.ClusterConfig(device1, protocol.ClusterConfig{})

	if _, ok := wcfg.Device(device2); !ok {
		t.Error("device 2 should not have been removed")
	}

	if contains(wcfg.Folders()["folder1"], device2, introducedByAnyone) {
		t.Error("expected device 2 to be removed from folder 1")
	}

	if !contains(wcfg.Folders()["folder2"], device2, introducedByAnyone) {
		t.Error("expected device 2 not to be removed from folder 2")
	}
}

func TestIssue4897(t *testing.T) {
	wcfg, m := newState(config.Configuration{
		Devices: []config.DeviceConfiguration{
			{
				DeviceID:   device1,
				Introducer: true,
			},
		},
		Folders: []config.FolderConfiguration{
			{
				ID:   "folder1",
				Path: "testdata",
				Devices: []config.FolderDeviceConfiguration{
					{DeviceID: device1},
				},
				Paused: true,
			},
		},
	})
	defer os.Remove(wcfg.ConfigPath())

	cm := m.generateClusterConfig(device1)
	if l := len(cm.Folders); l != 1 {
		t.Errorf("Cluster config contains %v folders, expected 1", l)
	}
}

func TestIssue5063(t *testing.T) {
	wcfg, m := newState(defaultAutoAcceptCfg)
	defer os.Remove(wcfg.ConfigPath())

	addAndVerify := func(wg *sync.WaitGroup) {
		id := srand.String(8)
		m.ClusterConfig(device1, protocol.ClusterConfig{
			Folders: []protocol.Folder{
				{
					ID:    id,
					Label: id,
				},
			},
		})
		os.RemoveAll(filepath.Join("testdata", id))
		wg.Done()
		if fcfg, ok := wcfg.Folder(id); !ok || !fcfg.SharedWith(device1) {
			t.Error("expected shared", id)
		}
	}

	wg := &sync.WaitGroup{}
	for i := 0; i <= 10; i++ {
		wg.Add(1)
		go addAndVerify(wg)
	}

	wg.Wait()
}

func TestAutoAcceptRejected(t *testing.T) {
	// Nothing happens if AutoAcceptFolders not set
	tcfg := defaultAutoAcceptCfg.Copy()
	for i := range tcfg.Devices {
		tcfg.Devices[i].AutoAcceptFolders = false
	}
	wcfg, m := newState(tcfg)
	defer os.Remove(wcfg.ConfigPath())
	id := srand.String(8)
	defer os.RemoveAll(filepath.Join("testdata", id))
	m.ClusterConfig(device1, protocol.ClusterConfig{
		Folders: []protocol.Folder{
			{
				ID:    id,
				Label: id,
			},
		},
	})

	if cfg, ok := m.cfg.Folder(id); ok && cfg.SharedWith(device1) {
		t.Error("unexpected shared", id)
	}
}

func TestAutoAcceptNewFolder(t *testing.T) {
	// New folder
	wcfg, m := newState(defaultAutoAcceptCfg)
	defer os.Remove(wcfg.ConfigPath())
	id := srand.String(8)
	defer os.RemoveAll(filepath.Join("testdata", id))
	m.ClusterConfig(device1, protocol.ClusterConfig{
		Folders: []protocol.Folder{
			{
				ID:    id,
				Label: id,
			},
		},
	})
	if fcfg, ok := wcfg.Folder(id); !ok || !fcfg.SharedWith(device1) {
		t.Error("expected shared", id)
	}
}

func TestAutoAcceptNewFolderFromTwoDevices(t *testing.T) {
	wcfg, m := newState(defaultAutoAcceptCfg)
	defer os.Remove(wcfg.ConfigPath())
	id := srand.String(8)
	defer os.RemoveAll(filepath.Join("testdata", id))
	m.ClusterConfig(device1, protocol.ClusterConfig{
		Folders: []protocol.Folder{
			{
				ID:    id,
				Label: id,
			},
		},
	})
	if fcfg, ok := wcfg.Folder(id); !ok || !fcfg.SharedWith(device1) {
		t.Error("expected shared", id)
	}
	if fcfg, ok := wcfg.Folder(id); !ok || fcfg.SharedWith(device2) {
		t.Error("unexpected expected shared", id)
	}
	m.ClusterConfig(device2, protocol.ClusterConfig{
		Folders: []protocol.Folder{
			{
				ID:    id,
				Label: id,
			},
		},
	})
	if fcfg, ok := wcfg.Folder(id); !ok || !fcfg.SharedWith(device2) {
		t.Error("expected shared", id)
	}
	m.Stop()
}

func TestAutoAcceptNewFolderFromOnlyOneDevice(t *testing.T) {
	modifiedCfg := defaultAutoAcceptCfg.Copy()
	modifiedCfg.Devices[2].AutoAcceptFolders = false
	wcfg, m := newState(modifiedCfg)
	defer os.Remove(wcfg.ConfigPath())
	id := srand.String(8)
	defer os.RemoveAll(filepath.Join("testdata", id))
	m.ClusterConfig(device1, protocol.ClusterConfig{
		Folders: []protocol.Folder{
			{
				ID:    id,
				Label: id,
			},
		},
	})
	if fcfg, ok := wcfg.Folder(id); !ok || !fcfg.SharedWith(device1) {
		t.Error("expected shared", id)
	}
	if fcfg, ok := wcfg.Folder(id); !ok || fcfg.SharedWith(device2) {
		t.Error("unexpected expected shared", id)
	}
	m.ClusterConfig(device2, protocol.ClusterConfig{
		Folders: []protocol.Folder{
			{
				ID:    id,
				Label: id,
			},
		},
	})
	if fcfg, ok := wcfg.Folder(id); !ok || fcfg.SharedWith(device2) {
		t.Error("unexpected shared", id)
	}
	m.Stop()
}

func TestAutoAcceptNewFolderPremutationsNoPanic(t *testing.T) {
	if testing.Short() {
		t.Skip("short tests only")
	}
	id := srand.String(8)
	label := srand.String(8)
	premutations := []protocol.Folder{
		{ID: id, Label: id},
		{ID: id, Label: label},
		{ID: label, Label: id},
		{ID: label, Label: label},
	}
	localFolders := append(premutations, protocol.Folder{})
	for _, localFolder := range localFolders {
		for _, localFolderPaused := range []bool{false, true} {
			for _, dev1folder := range premutations {
				for _, dev2folder := range premutations {
					cfg := defaultAutoAcceptCfg.Copy()
					if localFolder.Label != "" {
						fcfg := config.NewFolderConfiguration(protocol.LocalDeviceID, localFolder.ID, localFolder.Label, fs.FilesystemTypeBasic, filepath.Join("testdata", localFolder.ID))
						fcfg.Paused = localFolderPaused
						cfg.Folders = append(cfg.Folders, fcfg)
					}
					wcfg, m := newState(cfg)
					defer os.Remove(wcfg.ConfigPath())
					m.ClusterConfig(device1, protocol.ClusterConfig{
						Folders: []protocol.Folder{dev1folder},
					})
					m.ClusterConfig(device2, protocol.ClusterConfig{
						Folders: []protocol.Folder{dev2folder},
					})
					m.Stop()
					os.RemoveAll(filepath.Join("testdata", id))
					os.RemoveAll(filepath.Join("testdata", label))
				}
			}
		}
	}
}

func TestAutoAcceptMultipleFolders(t *testing.T) {
	// Multiple new folders
	wcfg, m := newState(defaultAutoAcceptCfg)
	defer os.Remove(wcfg.ConfigPath())
	id1 := srand.String(8)
	defer os.RemoveAll(filepath.Join("testdata", id1))
	id2 := srand.String(8)
	defer os.RemoveAll(filepath.Join("testdata", id2))
	m.ClusterConfig(device1, protocol.ClusterConfig{
		Folders: []protocol.Folder{
			{
				ID:    id1,
				Label: id1,
			},
			{
				ID:    id2,
				Label: id2,
			},
		},
	})
	if fcfg, ok := wcfg.Folder(id1); !ok || !fcfg.SharedWith(device1) {
		t.Error("expected shared", id1)
	}
	if fcfg, ok := wcfg.Folder(id2); !ok || !fcfg.SharedWith(device1) {
		t.Error("expected shared", id2)
	}
}

func TestAutoAcceptExistingFolder(t *testing.T) {
	// Existing folder
	id := srand.String(8)
	idOther := srand.String(8) // To check that path does not get changed.
	defer os.RemoveAll(filepath.Join("testdata", id))
	defer os.RemoveAll(filepath.Join("testdata", idOther))

	tcfg := defaultAutoAcceptCfg.Copy()
	tcfg.Folders = []config.FolderConfiguration{
		{
			ID:   id,
			Path: filepath.Join("testdata", idOther), // To check that path does not get changed.
		},
	}
	wcfg, m := newState(tcfg)
	defer os.Remove(wcfg.ConfigPath())
	if fcfg, ok := wcfg.Folder(id); !ok || fcfg.SharedWith(device1) {
		t.Error("missing folder, or shared", id)
	}
	m.ClusterConfig(device1, protocol.ClusterConfig{
		Folders: []protocol.Folder{
			{
				ID:    id,
				Label: id,
			},
		},
	})

	if fcfg, ok := wcfg.Folder(id); !ok || !fcfg.SharedWith(device1) || fcfg.Path != filepath.Join("testdata", idOther) {
		t.Error("missing folder, or unshared, or path changed", id)
	}
}

func TestAutoAcceptNewAndExistingFolder(t *testing.T) {
	// New and existing folder
	id1 := srand.String(8)
	defer os.RemoveAll(filepath.Join("testdata", id1))
	id2 := srand.String(8)
	defer os.RemoveAll(filepath.Join("testdata", id2))

	tcfg := defaultAutoAcceptCfg.Copy()
	tcfg.Folders = []config.FolderConfiguration{
		{
			ID:   id1,
			Path: filepath.Join("testdata", id1), // from previous test case, to verify that path doesn't get changed.
		},
	}
	wcfg, m := newState(tcfg)
	defer os.Remove(wcfg.ConfigPath())
	if fcfg, ok := wcfg.Folder(id1); !ok || fcfg.SharedWith(device1) {
		t.Error("missing folder, or shared", id1)
	}
	m.ClusterConfig(device1, protocol.ClusterConfig{
		Folders: []protocol.Folder{
			{
				ID:    id1,
				Label: id1,
			},
			{
				ID:    id2,
				Label: id2,
			},
		},
	})

	for i, id := range []string{id1, id2} {
		if fcfg, ok := wcfg.Folder(id); !ok || !fcfg.SharedWith(device1) {
			t.Error("missing folder, or unshared", i, id)
		}
	}
}

func TestAutoAcceptAlreadyShared(t *testing.T) {
	// Already shared
	id := srand.String(8)
	defer os.RemoveAll(filepath.Join("testdata", id))
	tcfg := defaultAutoAcceptCfg.Copy()
	tcfg.Folders = []config.FolderConfiguration{
		{
			ID:   id,
			Path: filepath.Join("testdata", id),
			Devices: []config.FolderDeviceConfiguration{
				{
					DeviceID: device1,
				},
			},
		},
	}
	wcfg, m := newState(tcfg)
	defer os.Remove(wcfg.ConfigPath())
	if fcfg, ok := wcfg.Folder(id); !ok || !fcfg.SharedWith(device1) {
		t.Error("missing folder, or not shared", id)
	}
	m.ClusterConfig(device1, protocol.ClusterConfig{
		Folders: []protocol.Folder{
			{
				ID:    id,
				Label: id,
			},
		},
	})

	if fcfg, ok := wcfg.Folder(id); !ok || !fcfg.SharedWith(device1) {
		t.Error("missing folder, or not shared", id)
	}
}

func TestAutoAcceptNameConflict(t *testing.T) {
	id := srand.String(8)
	label := srand.String(8)
	os.MkdirAll(filepath.Join("testdata", id), 0777)
	os.MkdirAll(filepath.Join("testdata", label), 0777)
	defer os.RemoveAll(filepath.Join("testdata", id))
	defer os.RemoveAll(filepath.Join("testdata", label))
	wcfg, m := newState(defaultAutoAcceptCfg)
	defer os.Remove(wcfg.ConfigPath())
	m.ClusterConfig(device1, protocol.ClusterConfig{
		Folders: []protocol.Folder{
			{
				ID:    id,
				Label: label,
			},
		},
	})
	if fcfg, ok := wcfg.Folder(id); ok && fcfg.SharedWith(device1) {
		t.Error("unexpected folder", id)
	}
}

func TestAutoAcceptPrefersLabel(t *testing.T) {
	// Prefers label, falls back to ID.
	wcfg, m := newState(defaultAutoAcceptCfg)
	defer os.Remove(wcfg.ConfigPath())
	id := srand.String(8)
	label := srand.String(8)
	defer os.RemoveAll(filepath.Join("testdata", id))
	defer os.RemoveAll(filepath.Join("testdata", label))
	m.ClusterConfig(device1, protocol.ClusterConfig{
		Folders: []protocol.Folder{
			{
				ID:    id,
				Label: label,
			},
		},
	})
	if fcfg, ok := wcfg.Folder(id); !ok || !fcfg.SharedWith(device1) || !strings.HasSuffix(fcfg.Path, label) {
		t.Error("expected shared, or wrong path", id, label, fcfg.Path)
	}
}

func TestAutoAcceptFallsBackToID(t *testing.T) {
	// Prefers label, falls back to ID.
	wcfg, m := newState(defaultAutoAcceptCfg)
	defer os.Remove(wcfg.ConfigPath())
	id := srand.String(8)
	label := srand.String(8)
	os.MkdirAll(filepath.Join("testdata", label), 0777)
	defer os.RemoveAll(filepath.Join("testdata", label))
	defer os.RemoveAll(filepath.Join("testdata", id))
	m.ClusterConfig(device1, protocol.ClusterConfig{
		Folders: []protocol.Folder{
			{
				ID:    id,
				Label: label,
			},
		},
	})
	if fcfg, ok := wcfg.Folder(id); !ok || !fcfg.SharedWith(device1) || !strings.HasSuffix(fcfg.Path, id) {
		t.Error("expected shared, or wrong path", id, label, fcfg.Path)
	}
}

func TestAutoAcceptPausedWhenFolderConfigChanged(t *testing.T) {
	// Existing folder
	id := srand.String(8)
	idOther := srand.String(8) // To check that path does not get changed.
	defer os.RemoveAll(filepath.Join("testdata", id))
	defer os.RemoveAll(filepath.Join("testdata", idOther))

	tcfg := defaultAutoAcceptCfg.Copy()
	fcfg := config.NewFolderConfiguration(protocol.LocalDeviceID, id, "", fs.FilesystemTypeBasic, filepath.Join("testdata", idOther))
	fcfg.Paused = true
	// The order of devices here is wrong (cfg.clean() sorts them), which will cause the folder to restart.
	// Because of the restart, folder gets removed from m.deviceFolder, which means that generateClusterConfig will not panic.
	// This wasn't an issue before, yet keeping the test case to prove that it still isn't.
	fcfg.Devices = append(fcfg.Devices, config.FolderDeviceConfiguration{
		DeviceID: device1,
	})
	tcfg.Folders = []config.FolderConfiguration{fcfg}
	wcfg, m := newState(tcfg)
	defer os.Remove(wcfg.ConfigPath())
	if fcfg, ok := wcfg.Folder(id); !ok || !fcfg.SharedWith(device1) {
		t.Error("missing folder, or not shared", id)
	}
	if _, ok := m.folderRunners[id]; ok {
		t.Fatal("folder running?")
	}

	m.ClusterConfig(device1, protocol.ClusterConfig{
		Folders: []protocol.Folder{
			{
				ID:    id,
				Label: id,
			},
		},
	})
	m.generateClusterConfig(device1)

	if fcfg, ok := wcfg.Folder(id); !ok {
		t.Error("missing folder")
	} else if fcfg.Path != filepath.Join("testdata", idOther) {
		t.Error("folder path changed")
	} else {
		for _, dev := range fcfg.DeviceIDs() {
			if dev == device1 {
				return
			}
		}
		t.Error("device missing")
	}

	if _, ok := m.folderRunners[id]; ok {
		t.Error("folder started")
	}
}

func TestAutoAcceptPausedWhenFolderConfigNotChanged(t *testing.T) {
	// Existing folder
	id := srand.String(8)
	idOther := srand.String(8) // To check that path does not get changed.
	defer os.RemoveAll(filepath.Join("testdata", id))
	defer os.RemoveAll(filepath.Join("testdata", idOther))

	tcfg := defaultAutoAcceptCfg.Copy()
	fcfg := config.NewFolderConfiguration(protocol.LocalDeviceID, id, "", fs.FilesystemTypeBasic, filepath.Join("testdata", idOther))
	fcfg.Paused = true
	// The new folder is exactly the same as the one constructed by handleAutoAccept, which means
	// the folder will not be restarted (even if it's paused), yet handleAutoAccept used to add the folder
	// to m.deviceFolders which had caused panics when calling generateClusterConfig, as the folder
	// did not have a file set.
	fcfg.Devices = append([]config.FolderDeviceConfiguration{
		{
			DeviceID: device1,
		},
	}, fcfg.Devices...) // Need to ensure this device order to avoid folder restart.
	tcfg.Folders = []config.FolderConfiguration{fcfg}
	wcfg, m := newState(tcfg)
	defer os.Remove(wcfg.ConfigPath())
	if fcfg, ok := wcfg.Folder(id); !ok || !fcfg.SharedWith(device1) {
		t.Error("missing folder, or not shared", id)
	}
	if _, ok := m.folderRunners[id]; ok {
		t.Fatal("folder running?")
	}

	m.ClusterConfig(device1, protocol.ClusterConfig{
		Folders: []protocol.Folder{
			{
				ID:    id,
				Label: id,
			},
		},
	})
	m.generateClusterConfig(device1)

	if fcfg, ok := wcfg.Folder(id); !ok {
		t.Error("missing folder")
	} else if fcfg.Path != filepath.Join("testdata", idOther) {
		t.Error("folder path changed")
	} else {
		for _, dev := range fcfg.DeviceIDs() {
			if dev == device1 {
				return
			}
		}
		t.Error("device missing")
	}

	if _, ok := m.folderRunners[id]; ok {
		t.Error("folder started")
	}
}

func changeIgnores(t *testing.T, m *Model, expected []string) {
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

	if !arrEqual(ignores, ignores2) {
		t.Errorf("Incorrect ignores: %v != %v", ignores2, ignores)
	}

	if runtime.GOOS == "darwin" {
		// see above
		time.Sleep(time.Second)
	} else {
		time.Sleep(time.Millisecond)
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
}

func TestIgnores(t *testing.T) {
	// Assure a clean start state
	os.RemoveAll(filepath.Join("testdata", config.DefaultMarkerName))
	os.MkdirAll(filepath.Join("testdata", config.DefaultMarkerName), 0644)
	ioutil.WriteFile("testdata/.stignore", []byte(".*\nquux\n"), 0644)

	db := db.OpenMemory()
	m := NewModel(defaultCfgWrapper, protocol.LocalDeviceID, "syncthing", "dev", db, nil)
	m.ServeBackground()
	defer m.Stop()

	// m.cfg.SetFolder is not usable as it is non-blocking, and there is no
	// way to know when the folder is actually added.
	m.AddFolder(defaultFolderConfig)
	m.StartFolder("default")

	// Reach in and update the ignore matcher to one that always does
	// reloads when asked to, instead of checking file mtimes. This is
	// because we will be changing the files on disk often enough that the
	// mtimes will be unreliable to determine change status.
	m.fmut.Lock()
	m.folderIgnores["default"] = ignore.New(defaultFs, ignore.WithCache(true), ignore.WithChangeDetector(newAlwaysChanged()))
	m.fmut.Unlock()

	// Make sure the initial scan has finished (ScanFolders is blocking)
	m.ScanFolders()

	expected := []string{
		".*",
		"quux",
	}

	changeIgnores(t, m, expected)

	_, _, err := m.GetIgnores("doesnotexist")
	if err == nil {
		t.Error("No error")
	}

	err = m.SetIgnores("doesnotexist", expected)
	if err == nil {
		t.Error("No error")
	}

	// Invalid path, marker should be missing, hence returns an error.
	m.AddFolder(config.FolderConfiguration{ID: "fresh", Path: "XXX"})
	_, _, err = m.GetIgnores("fresh")
	if err == nil {
		t.Error("No error")
	}

	// Repeat tests with paused folder
	pausedDefaultFolderConfig := defaultFolderConfig
	pausedDefaultFolderConfig.Paused = true

	m.RestartFolder(defaultFolderConfig, pausedDefaultFolderConfig)
	// Here folder initialization is not an issue as a paused folder isn't
	// added to the model and thus there is no initial scan happening.

	changeIgnores(t, m, expected)

	// Make sure no .stignore file is considered valid
	os.Rename("testdata/.stignore", "testdata/.stignore.bak")
	changeIgnores(t, m, []string{})
	os.Rename("testdata/.stignore.bak", "testdata/.stignore")
}

func TestROScanRecovery(t *testing.T) {
	ldb := db.OpenMemory()
	set := db.NewFileSet("default", defaultFs, ldb)
	set.Update(protocol.LocalDeviceID, []protocol.FileInfo{
		{Name: "dummyfile", Version: protocol.Vector{Counters: []protocol.Counter{{ID: 42, Value: 1}}}},
	})

	fcfg := config.FolderConfiguration{
		ID:              "default",
		Path:            "testdata/rotestfolder",
		Type:            config.FolderTypeSendOnly,
		RescanIntervalS: 1,
		MarkerName:      config.DefaultMarkerName,
	}
	cfg := createTmpWrapper(config.Configuration{
		Folders: []config.FolderConfiguration{fcfg},
		Devices: []config.DeviceConfiguration{
			{
				DeviceID: device1,
			},
		},
	})
	defer os.Remove(cfg.ConfigPath())

	os.RemoveAll(fcfg.Path)

	m := NewModel(cfg, protocol.LocalDeviceID, "syncthing", "dev", ldb, nil)
	m.AddFolder(fcfg)
	m.StartFolder("default")
	m.ServeBackground()
	defer m.Stop()

	waitFor := func(status string) error {
		timeout := time.Now().Add(2 * time.Second)
		for {
			_, _, err := m.State("default")
			if err == nil && status == "" {
				return nil
			}
			if err != nil && err.Error() == status {
				return nil
			}

			if time.Now().After(timeout) {
				return fmt.Errorf("Timed out waiting for status: %s, current status: %v", status, err)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}

	if err := waitFor("folder path missing"); err != nil {
		t.Error(err)
		return
	}

	os.Mkdir(fcfg.Path, 0700)

	if err := waitFor("folder marker missing"); err != nil {
		t.Error(err)
		return
	}

	fd, err := os.Create(filepath.Join(fcfg.Path, config.DefaultMarkerName))
	if err != nil {
		t.Error(err)
		return
	}
	fd.Close()

	if err := waitFor(""); err != nil {
		t.Error(err)
		return
	}

	os.Remove(filepath.Join(fcfg.Path, config.DefaultMarkerName))

	if err := waitFor("folder marker missing"); err != nil {
		t.Error(err)
		return
	}

	os.Remove(fcfg.Path)

	if err := waitFor("folder path missing"); err != nil {
		t.Error(err)
		return
	}
}

func TestRWScanRecovery(t *testing.T) {
	ldb := db.OpenMemory()
	set := db.NewFileSet("default", defaultFs, ldb)
	set.Update(protocol.LocalDeviceID, []protocol.FileInfo{
		{Name: "dummyfile", Version: protocol.Vector{Counters: []protocol.Counter{{ID: 42, Value: 1}}}},
	})

	fcfg := config.FolderConfiguration{
		ID:              "default",
		Path:            "testdata/rwtestfolder",
		Type:            config.FolderTypeSendReceive,
		RescanIntervalS: 1,
		MarkerName:      config.DefaultMarkerName,
	}
	cfg := createTmpWrapper(config.Configuration{
		Folders: []config.FolderConfiguration{fcfg},
		Devices: []config.DeviceConfiguration{
			{
				DeviceID: device1,
			},
		},
	})
	defer os.Remove(cfg.ConfigPath())

	os.RemoveAll(fcfg.Path)

	m := NewModel(cfg, protocol.LocalDeviceID, "syncthing", "dev", ldb, nil)
	m.AddFolder(fcfg)
	m.StartFolder("default")
	m.ServeBackground()
	defer m.Stop()

	waitFor := func(status string) error {
		timeout := time.Now().Add(2 * time.Second)
		for {
			_, _, err := m.State("default")
			if err == nil && status == "" {
				return nil
			}
			if err != nil && err.Error() == status {
				return nil
			}

			if time.Now().After(timeout) {
				return fmt.Errorf("Timed out waiting for status: %s, current status: %v", status, err)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}

	if err := waitFor("folder path missing"); err != nil {
		t.Error(err)
		return
	}

	os.Mkdir(fcfg.Path, 0700)

	if err := waitFor("folder marker missing"); err != nil {
		t.Error(err)
		return
	}

	fd, err := os.Create(filepath.Join(fcfg.Path, config.DefaultMarkerName))
	if err != nil {
		t.Error(err)
		return
	}
	fd.Close()

	if err := waitFor(""); err != nil {
		t.Error(err)
		return
	}

	os.Remove(filepath.Join(fcfg.Path, config.DefaultMarkerName))

	if err := waitFor("folder marker missing"); err != nil {
		t.Error(err)
		return
	}

	os.Remove(fcfg.Path)

	if err := waitFor("folder path missing"); err != nil {
		t.Error(err)
		return
	}
}

func TestGlobalDirectoryTree(t *testing.T) {
	db := db.OpenMemory()
	m := NewModel(defaultCfgWrapper, protocol.LocalDeviceID, "syncthing", "dev", db, nil)
	m.AddFolder(defaultFolderConfig)
	m.ServeBackground()
	defer m.Stop()

	b := func(isfile bool, path ...string) protocol.FileInfo {
		typ := protocol.FileInfoTypeDirectory
		blocks := []protocol.BlockInfo{}
		if isfile {
			typ = protocol.FileInfoTypeFile
			blocks = []protocol.BlockInfo{{Offset: 0x0, Size: 0xa, Hash: []uint8{0x2f, 0x72, 0xcc, 0x11, 0xa6, 0xfc, 0xd0, 0x27, 0x1e, 0xce, 0xf8, 0xc6, 0x10, 0x56, 0xee, 0x1e, 0xb1, 0x24, 0x3b, 0xe3, 0x80, 0x5b, 0xf9, 0xa9, 0xdf, 0x98, 0xf9, 0x2f, 0x76, 0x36, 0xb0, 0x5c}}}
		}
		return protocol.FileInfo{
			Name:      filepath.Join(path...),
			Type:      typ,
			ModifiedS: 0x666,
			Blocks:    blocks,
			Size:      0xa,
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

	m.Index(device1, "default", testdata)

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
	m := NewModel(defaultCfgWrapper, protocol.LocalDeviceID, "syncthing", "dev", db, nil)
	m.AddFolder(defaultFolderConfig)
	m.ServeBackground()

	b := func(isfile bool, path ...string) protocol.FileInfo {
		typ := protocol.FileInfoTypeDirectory
		blocks := []protocol.BlockInfo{}
		if isfile {
			typ = protocol.FileInfoTypeFile
			blocks = []protocol.BlockInfo{{Offset: 0x0, Size: 0xa, Hash: []uint8{0x2f, 0x72, 0xcc, 0x11, 0xa6, 0xfc, 0xd0, 0x27, 0x1e, 0xce, 0xf8, 0xc6, 0x10, 0x56, 0xee, 0x1e, 0xb1, 0x24, 0x3b, 0xe3, 0x80, 0x5b, 0xf9, 0xa9, 0xdf, 0x98, 0xf9, 0x2f, 0x76, 0x36, 0xb0, 0x5c}}}
		}
		return protocol.FileInfo{
			Name:      filepath.Join(path...),
			Type:      typ,
			ModifiedS: 0x666,
			Blocks:    blocks,
			Size:      0xa,
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

	m.Index(device1, "default", testdata)

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

		files[i].ModifiedS = t
		files[i].Blocks = []protocol.BlockInfo{{Offset: 0, Size: 100, Hash: []byte("some hash bytes")}}
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
	m := NewModel(defaultCfgWrapper, protocol.LocalDeviceID, "syncthing", "dev", db, nil)
	m.AddFolder(defaultFolderConfig)
	m.ServeBackground()

	m.ScanFolder("default")
	files := genDeepFiles(n1, n2)

	m.Index(device1, "default", files)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.GlobalDirectoryTree("default", "", -1, false)
	}
	b.ReportAllocs()
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
	m := NewModel(defaultCfgWrapper, protocol.LocalDeviceID, "syncthing", "dev", db, nil)
	defCfg := defaultFolderConfig.Copy()
	defCfg.RescanIntervalS = 86400
	m.AddFolder(defCfg)
	m.StartFolder("default")
	m.ServeBackground()

	// Make sure the initial scan has finished (ScanFolders is blocking)
	m.ScanFolders()

	// Get a count of how many files are there now

	locorigfiles := m.LocalSize("default").Files
	globorigfiles := m.GlobalSize("default").Files

	// Delete and rescan specifically these two

	os.Remove("testdata/testrm")
	os.Remove("testdata/testrm2")
	m.ScanFolderSubdirs("default", []string{"testrm", "testrm2"})

	// Verify that the number of files decreased by two and the number of
	// deleted files increases by two

	loc := m.LocalSize("default")
	glob := m.GlobalSize("default")
	if loc.Files != locorigfiles-2 {
		t.Errorf("Incorrect local accounting; got %d current files, expected %d", loc.Files, locorigfiles-2)
	}
	if glob.Files != globorigfiles-2 {
		t.Errorf("Incorrect global accounting; got %d current files, expected %d", glob.Files, globorigfiles-2)
	}
	if loc.Deleted != 2 {
		t.Errorf("Incorrect local accounting; got %d deleted files, expected 2", loc.Deleted)
	}
	if glob.Deleted != 2 {
		t.Errorf("Incorrect global accounting; got %d deleted files, expected 2", glob.Deleted)
	}
}

func TestIssue4357(t *testing.T) {
	db := db.OpenMemory()
	cfg := defaultCfgWrapper.RawCopy()
	// Create a separate wrapper not to pollute other tests.
	wrapper := createTmpWrapper(config.Configuration{})
	defer os.Remove(wrapper.ConfigPath())
	m := NewModel(wrapper, protocol.LocalDeviceID, "syncthing", "dev", db, nil)
	m.ServeBackground()
	defer m.Stop()

	// Force the model to wire itself and add the folders
	p, err := wrapper.Replace(cfg)
	p.Wait()
	if err != nil {
		t.Error(err)
	}

	if _, ok := m.folderCfgs["default"]; !ok {
		t.Error("Folder should be running")
	}

	newCfg := wrapper.RawCopy()
	newCfg.Folders[0].Paused = true

	p, err = wrapper.Replace(newCfg)
	p.Wait()
	if err != nil {
		t.Error(err)
	}

	if _, ok := m.folderCfgs["default"]; ok {
		t.Error("Folder should not be running")
	}

	if _, ok := m.cfg.Folder("default"); !ok {
		t.Error("should still have folder in config")
	}

	p, err = wrapper.Replace(config.Configuration{})
	p.Wait()
	if err != nil {
		t.Error(err)
	}

	if _, ok := m.cfg.Folder("default"); ok {
		t.Error("should not have folder in config")
	}

	// Add the folder back, should be running
	p, err = wrapper.Replace(cfg)
	p.Wait()
	if err != nil {
		t.Error(err)
	}

	if _, ok := m.folderCfgs["default"]; !ok {
		t.Error("Folder should be running")
	}
	if _, ok := m.cfg.Folder("default"); !ok {
		t.Error("should still have folder in config")
	}

	// Should not panic when removing a running folder.
	p, err = wrapper.Replace(config.Configuration{})
	p.Wait()
	if err != nil {
		t.Error(err)
	}

	if _, ok := m.folderCfgs["default"]; ok {
		t.Error("Folder should not be running")
	}
	if _, ok := m.cfg.Folder("default"); ok {
		t.Error("should not have folder in config")
	}
}

func TestIssue2782(t *testing.T) {
	// CheckHealth should accept a symlinked folder, when using tilde-expanded path.

	if runtime.GOOS == "windows" {
		t.Skip("not reliable on Windows")
		return
	}
	home := os.Getenv("HOME")
	if home == "" {
		t.Skip("no home")
	}

	// Create the test env. Needs to be based on $HOME as tilde expansion is
	// part of the issue. Skip the test if any of this fails, as we are a
	// bit outside of our stated domain here...

	testName := ".syncthing-test." + srand.String(16)
	testDir := filepath.Join(home, testName)
	if err := os.RemoveAll(testDir); err != nil {
		t.Skip(err)
	}
	if err := os.MkdirAll(testDir+"/syncdir", 0755); err != nil {
		t.Skip(err)
	}
	if err := ioutil.WriteFile(testDir+"/syncdir/file", []byte("hello, world\n"), 0644); err != nil {
		t.Skip(err)
	}
	if err := os.Symlink("syncdir", testDir+"/synclink"); err != nil {
		t.Skip(err)
	}
	defer os.RemoveAll(testDir)

	db := db.OpenMemory()
	m := NewModel(defaultCfgWrapper, protocol.LocalDeviceID, "syncthing", "dev", db, nil)
	m.AddFolder(config.NewFolderConfiguration(protocol.LocalDeviceID, "default", "default", fs.FilesystemTypeBasic, "~/"+testName+"/synclink/"))
	m.StartFolder("default")
	m.ServeBackground()
	defer m.Stop()

	if err := m.ScanFolder("default"); err != nil {
		t.Error("scan error:", err)
	}

	m.fmut.Lock()
	runner, _ := m.folderRunners["default"]
	m.fmut.Unlock()
	if err := runner.CheckHealth(); err != nil {
		t.Error("health check error:", err)
	}
}

func TestIndexesForUnknownDevicesDropped(t *testing.T) {
	dbi := db.OpenMemory()

	files := db.NewFileSet("default", defaultFs, dbi)
	files.Drop(device1)
	files.Update(device1, genFiles(1))
	files.Drop(device2)
	files.Update(device2, genFiles(1))

	if len(files.ListDevices()) != 2 {
		t.Error("expected two devices")
	}

	m := NewModel(defaultCfgWrapper, protocol.LocalDeviceID, "syncthing", "dev", dbi, nil)
	m.AddFolder(defaultFolderConfig)
	m.StartFolder("default")

	// Remote sequence is cached, hence need to recreated.
	files = db.NewFileSet("default", defaultFs, dbi)

	if len(files.ListDevices()) != 1 {
		t.Error("Expected one device")
	}
}

func TestSharedWithClearedOnDisconnect(t *testing.T) {
	dbi := db.OpenMemory()

	fcfg := config.NewFolderConfiguration(protocol.LocalDeviceID, "default", "default", fs.FilesystemTypeBasic, "testdata")
	fcfg.Devices = []config.FolderDeviceConfiguration{
		{DeviceID: device1},
		{DeviceID: device2},
	}
	cfg := config.Configuration{
		Folders: []config.FolderConfiguration{fcfg},
		Devices: []config.DeviceConfiguration{
			config.NewDeviceConfiguration(device1, "device1"),
			config.NewDeviceConfiguration(device2, "device2"),
		},
		Options: config.OptionsConfiguration{
			// Don't remove temporaries directly on startup
			KeepTemporariesH: 1,
		},
	}

	wcfg := createTmpWrapper(cfg)
	defer os.Remove(wcfg.ConfigPath())

	m := NewModel(wcfg, protocol.LocalDeviceID, "syncthing", "dev", dbi, nil)
	m.AddFolder(fcfg)
	m.StartFolder(fcfg.ID)
	m.ServeBackground()

	conn1 := &fakeConnection{id: device1}
	m.AddConnection(conn1, protocol.HelloResult{})
	conn2 := &fakeConnection{id: device2}
	m.AddConnection(conn2, protocol.HelloResult{})

	m.ClusterConfig(device1, protocol.ClusterConfig{
		Folders: []protocol.Folder{
			{
				ID: "default",
				Devices: []protocol.Device{
					{ID: device1},
					{ID: device2},
				},
			},
		},
	})
	m.ClusterConfig(device2, protocol.ClusterConfig{
		Folders: []protocol.Folder{
			{
				ID: "default",
				Devices: []protocol.Device{
					{ID: device1},
					{ID: device2},
				},
			},
		},
	})

	if fcfg, ok := m.cfg.Folder("default"); !ok || !fcfg.SharedWith(device1) {
		t.Error("not shared with device1")
	}
	if fcfg, ok := m.cfg.Folder("default"); !ok || !fcfg.SharedWith(device2) {
		t.Error("not shared with device2")
	}

	if conn2.Closed() {
		t.Error("conn already closed")
	}

	cfg = cfg.Copy()
	cfg.Devices = cfg.Devices[:1]

	if _, err := wcfg.Replace(cfg); err != nil {
		t.Error(err)
	}

	time.Sleep(100 * time.Millisecond) // Committer notification happens in a separate routine

	fcfg, ok := m.cfg.Folder("default")
	if !ok {
		t.Fatal("default folder missing")
	}
	if !fcfg.SharedWith(device1) {
		t.Error("not shared with device1")
	}
	if fcfg.SharedWith(device2) {
		t.Error("shared with device2")
	}
	for _, dev := range fcfg.Devices {
		if dev.DeviceID == device2 {
			t.Error("still there")
		}
	}

	if !conn2.Closed() {
		t.Error("connection not closed")
	}

	if _, ok := wcfg.Devices()[device2]; ok {
		t.Error("device still in config")
	}

	if _, ok := m.conn[device2]; !ok {
		t.Error("conn missing early")
	}

	if _, ok := m.helloMessages[device2]; !ok {
		t.Error("hello missing early")
	}

	if _, ok := m.deviceDownloads[device2]; !ok {
		t.Error("downloads missing early")
	}

	m.Closed(conn2, fmt.Errorf("foo"))

	if _, ok := m.conn[device2]; ok {
		t.Error("conn not missing")
	}

	if _, ok := m.helloMessages[device2]; ok {
		t.Error("hello not missing")
	}

	if _, ok := m.deviceDownloads[device2]; ok {
		t.Error("downloads not missing")
	}
}

func TestIssue3496(t *testing.T) {
	t.Skip("This test deletes files that the other test depend on. Needs fixing.")

	// It seems like lots of deleted files can cause negative completion
	// percentages. Lets make sure that doesn't happen. Also do some general
	// checks on the completion calculation stuff.

	dbi := db.OpenMemory()
	m := NewModel(defaultCfgWrapper, protocol.LocalDeviceID, "syncthing", "dev", dbi, nil)
	m.AddFolder(defaultFolderConfig)
	m.StartFolder("default")
	m.ServeBackground()
	defer m.Stop()

	m.ScanFolder("default")

	addFakeConn(m, device1)
	addFakeConn(m, device2)

	// Reach into the model and grab the current file list...

	m.fmut.RLock()
	fs := m.folderFiles["default"]
	m.fmut.RUnlock()
	var localFiles []protocol.FileInfo
	fs.WithHave(protocol.LocalDeviceID, func(i db.FileIntf) bool {
		localFiles = append(localFiles, i.(protocol.FileInfo))
		return true
	})

	// Mark all files as deleted and fake it as update from device1

	for i := range localFiles {
		localFiles[i].Deleted = true
		localFiles[i].Version = localFiles[i].Version.Update(device1.Short())
		localFiles[i].Blocks = nil
	}

	// Also add a small file that we're supposed to need, or the global size
	// stuff will bail out early due to the entire folder being zero size.

	localFiles = append(localFiles, protocol.FileInfo{
		Name:    "fake",
		Size:    1234,
		Type:    protocol.FileInfoTypeFile,
		Version: protocol.Vector{Counters: []protocol.Counter{{ID: device1.Short(), Value: 42}}},
	})

	m.IndexUpdate(device1, "default", localFiles)

	// Check that the completion percentage for us makes sense

	comp := m.Completion(protocol.LocalDeviceID, "default")
	if comp.NeedBytes > comp.GlobalBytes {
		t.Errorf("Need more bytes than exist, not possible: %d > %d", comp.NeedBytes, comp.GlobalBytes)
	}
	if comp.CompletionPct < 0 {
		t.Errorf("Less than zero percent complete, not possible: %.02f%%", comp.CompletionPct)
	}
	if comp.NeedBytes == 0 {
		t.Error("Need no bytes even though some files are deleted")
	}
	if comp.CompletionPct == 100 {
		t.Errorf("Fully complete, not possible: %.02f%%", comp.CompletionPct)
	}
	t.Log(comp)

	// Check that NeedSize does the correct thing
	need := m.NeedSize("default")
	if need.Files != 1 || need.Bytes != 1234 {
		// The one we added synthetically above
		t.Errorf("Incorrect need size; %d, %d != 1, 1234", need.Files, need.Bytes)
	}
	if int(need.Deleted) != len(localFiles)-1 {
		// The rest
		t.Errorf("Incorrect need deletes; %d != %d", need.Deleted, len(localFiles)-1)
	}
}

func TestIssue3804(t *testing.T) {
	dbi := db.OpenMemory()
	m := NewModel(defaultCfgWrapper, protocol.LocalDeviceID, "syncthing", "dev", dbi, nil)
	m.AddFolder(defaultFolderConfig)
	m.StartFolder("default")
	m.ServeBackground()
	defer m.Stop()

	// Subdirs ending in slash should be accepted

	if err := m.ScanFolderSubdirs("default", []string{"baz/", "foo"}); err != nil {
		t.Error("Unexpected error:", err)
	}
}

func TestIssue3829(t *testing.T) {
	dbi := db.OpenMemory()
	m := NewModel(defaultCfgWrapper, protocol.LocalDeviceID, "syncthing", "dev", dbi, nil)
	m.AddFolder(defaultFolderConfig)
	m.StartFolder("default")
	m.ServeBackground()
	defer m.Stop()

	// Empty subdirs should be accepted

	if err := m.ScanFolderSubdirs("default", []string{""}); err != nil {
		t.Error("Unexpected error:", err)
	}
}

func TestNoRequestsFromPausedDevices(t *testing.T) {
	t.Skip("broken, fails randomly, #3843")

	dbi := db.OpenMemory()

	fcfg := config.NewFolderConfiguration(protocol.LocalDeviceID, "default", "default", fs.FilesystemTypeBasic, "testdata")
	fcfg.Devices = []config.FolderDeviceConfiguration{
		{DeviceID: device1},
		{DeviceID: device2},
	}
	cfg := config.Configuration{
		Folders: []config.FolderConfiguration{fcfg},
		Devices: []config.DeviceConfiguration{
			config.NewDeviceConfiguration(device1, "device1"),
			config.NewDeviceConfiguration(device2, "device2"),
		},
		Options: config.OptionsConfiguration{
			// Don't remove temporaries directly on startup
			KeepTemporariesH: 1,
		},
	}

	wcfg := createTmpWrapper(cfg)
	defer os.Remove(wcfg.ConfigPath())

	m := NewModel(wcfg, protocol.LocalDeviceID, "syncthing", "dev", dbi, nil)
	m.AddFolder(fcfg)
	m.StartFolder(fcfg.ID)
	m.ServeBackground()

	file := testDataExpected["foo"]
	files := m.folderFiles["default"]
	files.Update(device1, []protocol.FileInfo{file})
	files.Update(device2, []protocol.FileInfo{file})

	avail := m.Availability("default", file, file.Blocks[0])
	if len(avail) != 0 {
		t.Errorf("should not be available, no connections")
	}

	addFakeConn(m, device1)
	addFakeConn(m, device2)

	// !!! This is not what I'd expect to happen, as we don't even know if the peer has the original index !!!

	avail = m.Availability("default", file, file.Blocks[0])
	if len(avail) != 2 {
		t.Errorf("should have two available")
	}

	cc := protocol.ClusterConfig{
		Folders: []protocol.Folder{
			{
				ID: "default",
				Devices: []protocol.Device{
					{ID: device1},
					{ID: device2},
				},
			},
		},
	}

	m.ClusterConfig(device1, cc)
	m.ClusterConfig(device2, cc)

	avail = m.Availability("default", file, file.Blocks[0])
	if len(avail) != 2 {
		t.Errorf("should have two available")
	}

	m.Closed(&fakeConnection{id: device1}, errDeviceUnknown)
	m.Closed(&fakeConnection{id: device2}, errDeviceUnknown)

	avail = m.Availability("default", file, file.Blocks[0])
	if len(avail) != 0 {
		t.Errorf("should have no available")
	}

	// Test that remote paused folders are not used.

	addFakeConn(m, device1)
	addFakeConn(m, device2)

	m.ClusterConfig(device1, cc)
	ccp := cc
	ccp.Folders[0].Paused = true
	m.ClusterConfig(device1, ccp)

	avail = m.Availability("default", file, file.Blocks[0])
	if len(avail) != 1 {
		t.Errorf("should have one available")
	}
}

// TestIssue2571 tests replacing a directory with content with a symlink
func TestIssue2571(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Scanning symlinks isn't supported on windows")
	}

	err := defaultFs.MkdirAll("replaceDir", 0755)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		defaultFs.RemoveAll("replaceDir")
	}()

	testFs := fs.NewFilesystem(fs.FilesystemTypeBasic, filepath.Join(defaultFs.URI(), "replaceDir"))

	for _, dir := range []string{"toLink", "linkTarget"} {
		err := testFs.MkdirAll(dir, 0775)
		if err != nil {
			t.Fatal(err)
		}
		fd, err := testFs.Create(filepath.Join(dir, "a"))
		if err != nil {
			t.Fatal(err)
		}
		fd.Close()
	}

	dbi := db.OpenMemory()
	m := NewModel(defaultCfgWrapper, protocol.LocalDeviceID, "syncthing", "dev", dbi, nil)
	m.AddFolder(defaultFolderConfig)
	m.StartFolder("default")
	m.ServeBackground()
	defer m.Stop()
	m.ScanFolder("default")

	if err = testFs.RemoveAll("toLink"); err != nil {
		t.Fatal(err)
	}

	if err := osutil.DebugSymlinkForTestsOnly(filepath.Join(testFs.URI(), "linkTarget"), filepath.Join(testFs.URI(), "toLink")); err != nil {
		t.Fatal(err)
	}

	m.ScanFolder("default")

	if dir, ok := m.CurrentFolderFile("default", filepath.Join("replaceDir", "toLink")); !ok {
		t.Fatalf("Dir missing in db")
	} else if !dir.IsSymlink() {
		t.Errorf("Dir wasn't changed to symlink")
	}
	if file, ok := m.CurrentFolderFile("default", filepath.Join("replaceDir", "toLink", "a")); !ok {
		t.Fatalf("File missing in db")
	} else if !file.Deleted {
		t.Errorf("File below symlink has not been marked as deleted")
	}
}

// TestIssue4573 tests that contents of an unavailable dir aren't marked deleted
func TestIssue4573(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Can't make the dir inaccessible on windows")
	}

	err := defaultFs.MkdirAll("inaccessible", 0755)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		defaultFs.Chmod("inaccessible", 0777)
		defaultFs.RemoveAll("inaccessible")
	}()

	file := filepath.Join("inaccessible", "a")
	fd, err := defaultFs.Create(file)
	if err != nil {
		t.Fatal(err)
	}
	fd.Close()

	dbi := db.OpenMemory()
	m := NewModel(defaultCfgWrapper, protocol.LocalDeviceID, "syncthing", "dev", dbi, nil)
	m.AddFolder(defaultFolderConfig)
	m.StartFolder("default")
	m.ServeBackground()
	defer m.Stop()
	m.ScanFolder("default")

	err = defaultFs.Chmod("inaccessible", 0000)
	if err != nil {
		t.Fatal(err)
	}

	m.ScanFolder("default")

	if file, ok := m.CurrentFolderFile("default", file); !ok {
		t.Fatalf("File missing in db")
	} else if file.Deleted {
		t.Errorf("Inaccessible file has been marked as deleted.")
	}
}

// TestInternalScan checks whether various fs operations are correctly represented
// in the db after scanning.
func TestInternalScan(t *testing.T) {
	err := defaultFs.MkdirAll("internalScan", 0755)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		defaultFs.RemoveAll("internalScan")
	}()

	testFs := fs.NewFilesystem(fs.FilesystemTypeBasic, filepath.Join(defaultFs.URI(), "internalScan"))

	testCases := map[string]func(protocol.FileInfo) bool{
		"removeDir": func(f protocol.FileInfo) bool {
			return !f.Deleted
		},
		"dirToFile": func(f protocol.FileInfo) bool {
			return f.Deleted || f.IsDirectory()
		},
	}

	baseDirs := []string{"dirToFile", "removeDir"}
	for _, dir := range baseDirs {
		sub := filepath.Join(dir, "subDir")
		for _, dir := range []string{dir, sub} {
			err := testFs.MkdirAll(dir, 0775)
			if err != nil {
				t.Fatalf("%v: %v", dir, err)
			}
		}
		testCases[sub] = func(f protocol.FileInfo) bool {
			return !f.Deleted
		}
		for _, dir := range []string{dir, sub} {
			file := filepath.Join(dir, "a")
			fd, err := testFs.Create(file)
			if err != nil {
				t.Fatal(err)
			}
			fd.Close()
			testCases[file] = func(f protocol.FileInfo) bool {
				return !f.Deleted
			}
		}
	}

	dbi := db.OpenMemory()
	m := NewModel(defaultCfgWrapper, protocol.LocalDeviceID, "syncthing", "dev", dbi, nil)
	m.AddFolder(defaultFolderConfig)
	m.StartFolder("default")
	m.ServeBackground()
	defer m.Stop()
	m.ScanFolder("default")

	for _, dir := range baseDirs {
		if err = testFs.RemoveAll(dir); err != nil {
			t.Fatal(err)
		}
	}

	fd, err := testFs.Create("dirToFile")
	if err != nil {
		t.Fatal(err)
	}
	fd.Close()

	m.ScanFolder("default")

	for path, cond := range testCases {
		if f, ok := m.CurrentFolderFile("default", filepath.Join("internalScan", path)); !ok {
			t.Fatalf("%v missing in db", path)
		} else if cond(f) {
			t.Errorf("Incorrect db entry for %v", path)
		}
	}
}

func TestCustomMarkerName(t *testing.T) {
	ldb := db.OpenMemory()
	set := db.NewFileSet("default", defaultFs, ldb)
	set.Update(protocol.LocalDeviceID, []protocol.FileInfo{
		{Name: "dummyfile"},
	})

	fcfg := config.FolderConfiguration{
		ID:              "default",
		Path:            "testdata/rwtestfolder",
		Type:            config.FolderTypeSendReceive,
		RescanIntervalS: 1,
		MarkerName:      "myfile",
	}
	cfg := createTmpWrapper(config.Configuration{
		Folders: []config.FolderConfiguration{fcfg},
		Devices: []config.DeviceConfiguration{
			{
				DeviceID: device1,
			},
		},
	})
	defer os.Remove(cfg.ConfigPath())

	os.RemoveAll(fcfg.Path)
	defer os.RemoveAll(fcfg.Path)

	m := NewModel(cfg, protocol.LocalDeviceID, "syncthing", "dev", ldb, nil)
	m.AddFolder(fcfg)
	m.StartFolder("default")
	m.ServeBackground()
	defer m.Stop()

	waitFor := func(status string) error {
		timeout := time.Now().Add(2 * time.Second)
		for {
			_, _, err := m.State("default")
			if err == nil && status == "" {
				return nil
			}
			if err != nil && err.Error() == status {
				return nil
			}

			if time.Now().After(timeout) {
				return fmt.Errorf("Timed out waiting for status: %s, current status: %v", status, err)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}

	if err := waitFor("folder path missing"); err != nil {
		t.Error(err)
		return
	}

	os.Mkdir(fcfg.Path, 0700)
	fd, err := os.Create(filepath.Join(fcfg.Path, "myfile"))
	if err != nil {
		t.Error(err)
		return
	}
	fd.Close()

	if err := waitFor(""); err != nil {
		t.Error(err)
		return
	}
}

func TestRemoveDirWithContent(t *testing.T) {
	defer func() {
		defaultFs.RemoveAll("dirwith")
	}()

	defaultFs.MkdirAll("dirwith", 0755)
	content := filepath.Join("dirwith", "content")
	fd, err := defaultFs.Create(content)
	if err != nil {
		t.Fatal(err)
		return
	}
	fd.Close()

	dbi := db.OpenMemory()
	m := NewModel(defaultCfgWrapper, protocol.LocalDeviceID, "syncthing", "dev", dbi, nil)
	m.AddFolder(defaultFolderConfig)
	m.StartFolder("default")
	m.ServeBackground()
	defer m.Stop()
	m.ScanFolder("default")

	dir, ok := m.CurrentFolderFile("default", "dirwith")
	if !ok {
		t.Fatalf("Can't get dir \"dirwith\" after initial scan")
	}
	dir.Deleted = true
	dir.Version = dir.Version.Update(device1.Short()).Update(device1.Short())

	file, ok := m.CurrentFolderFile("default", content)
	if !ok {
		t.Fatalf("Can't get file \"%v\" after initial scan", content)
	}
	file.Deleted = true
	file.Version = file.Version.Update(device1.Short()).Update(device1.Short())

	m.IndexUpdate(device1, "default", []protocol.FileInfo{dir, file})

	// Is there something we could trigger on instead of just waiting?
	timeout := time.NewTimer(5 * time.Second)
	for {
		dir, ok := m.CurrentFolderFile("default", "dirwith")
		if !ok {
			t.Fatalf("Can't get dir \"dirwith\" after index update")
		}
		file, ok := m.CurrentFolderFile("default", content)
		if !ok {
			t.Fatalf("Can't get file \"%v\" after index update", content)
		}
		if dir.Deleted && file.Deleted {
			return
		}

		select {
		case <-timeout.C:
			if !dir.Deleted && !file.Deleted {
				t.Errorf("Neither the dir nor its content was deleted before timing out.")
			} else if !dir.Deleted {
				t.Errorf("The dir was not deleted before timing out.")
			} else {
				t.Errorf("The content of the dir was not deleted before timing out.")
			}
			return
		default:
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func TestIssue4475(t *testing.T) {
	defer func() {
		defaultFs.RemoveAll("delDir")
	}()

	err := defaultFs.MkdirAll("delDir", 0755)
	if err != nil {
		t.Fatal(err)
	}

	dbi := db.OpenMemory()
	m := NewModel(defaultCfgWrapper, protocol.LocalDeviceID, "syncthing", "dev", dbi, nil)
	m.AddFolder(defaultFolderConfig)
	m.StartFolder("default")
	m.ServeBackground()
	defer m.Stop()
	m.ScanFolder("default")

	// Scenario: Dir is deleted locally and before syncing/index exchange
	// happens, a file is create in that dir on the remote.
	// This should result in the directory being recreated and added to the
	// db locally.

	if err = defaultFs.RemoveAll("delDir"); err != nil {
		t.Fatal(err)
	}

	m.ScanFolder("default")

	conn := addFakeConn(m, device1)
	conn.folder = "default"

	if fcfg, ok := m.cfg.Folder("default"); !ok || !fcfg.SharedWith(device1) {
		t.Fatal("not shared with device1")
	}

	fileName := filepath.Join("delDir", "file")
	conn.addFile(fileName, 0644, protocol.FileInfoTypeFile, nil)
	conn.sendIndexUpdate()

	// Is there something we could trigger on instead of just waiting?
	timeout := time.NewTimer(5 * time.Second)
	created := false
	for {
		if !created {
			if _, ok := m.CurrentFolderFile("default", fileName); ok {
				created = true
			}
		} else {
			dir, ok := m.CurrentFolderFile("default", "delDir")
			if !ok {
				t.Fatalf("can't get dir from db")
			}
			if !dir.Deleted {
				return
			}
		}

		select {
		case <-timeout.C:
			if created {
				t.Errorf("Timed out before file from remote was created")
			} else {
				t.Errorf("Timed out before directory was resurrected in db")
			}
			return
		default:
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func TestVersionRestore(t *testing.T) {
	// We create a bunch of files which we restore
	// In each file, we write the filename as the content
	// We verify that the content matches at the expected filenames
	// after the restore operation.
	dir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	dbi := db.OpenMemory()

	fcfg := config.NewFolderConfiguration(protocol.LocalDeviceID, "default", "default", fs.FilesystemTypeBasic, dir)
	fcfg.Versioning.Type = "simple"
	fcfg.FSWatcherEnabled = false
	filesystem := fcfg.Filesystem()

	rawConfig := config.Configuration{
		Folders: []config.FolderConfiguration{fcfg},
	}
	cfg := createTmpWrapper(rawConfig)
	defer os.Remove(cfg.ConfigPath())

	m := NewModel(cfg, protocol.LocalDeviceID, "syncthing", "dev", dbi, nil)
	m.AddFolder(fcfg)
	m.StartFolder("default")
	m.ServeBackground()
	defer m.Stop()
	m.ScanFolder("default")

	sentinel, err := time.ParseInLocation(versioner.TimeFormat, "20200101-010101", locationLocal)
	if err != nil {
		t.Fatal(err)
	}
	sentinelTag := sentinel.Format(versioner.TimeFormat)

	for _, file := range []string{
		// Versions directory
		".stversions/file~20171210-040404.txt",  // will be restored
		".stversions/existing~20171210-040404",  // exists, should expect to be archived.
		".stversions/something~20171210-040404", // will become directory, hence error
		".stversions/dir/file~20171210-040404.txt",
		".stversions/dir/file~20171210-040405.txt",
		".stversions/dir/file~20171210-040406.txt",
		".stversions/very/very/deep/one~20171210-040406.txt", // lives deep down, no directory exists.
		".stversions/dir/existing~20171210-040406.txt",       // exists, should expect to be archived.
		".stversions/dir/file.txt~20171210-040405",           // incorrect tag format, ignored.
		".stversions/dir/cat",                                // incorrect tag format, ignored.

		// "file.txt" will be restored
		"existing",
		"something/file", // Becomes directory
		"dir/file.txt",
		"dir/existing.txt",
	} {
		if runtime.GOOS == "windows" {
			file = filepath.FromSlash(file)
		}
		dir := filepath.Dir(file)
		if err := filesystem.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if fd, err := filesystem.Create(file); err != nil {
			t.Fatal(err)
		} else if _, err := fd.Write([]byte(file)); err != nil {
			t.Fatal(err)
		} else if err := fd.Close(); err != nil {
			t.Fatal(err)
		} else if err := filesystem.Chtimes(file, sentinel, sentinel); err != nil {
			t.Fatal(err)
		}
	}

	versions, err := m.GetFolderVersions("default")
	if err != nil {
		t.Fatal(err)
	}
	expectedVersions := map[string]int{
		"file.txt":               1,
		"existing":               1,
		"something":              1,
		"dir/file.txt":           3,
		"dir/existing.txt":       1,
		"very/very/deep/one.txt": 1,
	}

	for name, vers := range versions {
		cnt, ok := expectedVersions[name]
		if !ok {
			t.Errorf("unexpected %s", name)
		}
		if len(vers) != cnt {
			t.Errorf("%s: %d != %d", name, cnt, len(vers))
		}
		// Delete, so we can check if we didn't hit something we expect afterwards.
		delete(expectedVersions, name)
	}

	for name := range expectedVersions {
		t.Errorf("not found expected %s", name)
	}

	// Restoring non existing folder fails.
	_, err = m.RestoreFolderVersions("does not exist", nil)
	if err == nil {
		t.Errorf("expected an error")
	}

	makeTime := func(s string) time.Time {
		tm, err := time.ParseInLocation(versioner.TimeFormat, s, locationLocal)
		if err != nil {
			t.Error(err)
		}
		return tm.Truncate(time.Second)
	}

	restore := map[string]time.Time{
		"file.txt":               makeTime("20171210-040404"),
		"existing":               makeTime("20171210-040404"),
		"something":              makeTime("20171210-040404"),
		"dir/file.txt":           makeTime("20171210-040406"),
		"dir/existing.txt":       makeTime("20171210-040406"),
		"very/very/deep/one.txt": makeTime("20171210-040406"),
	}

	ferr, err := m.RestoreFolderVersions("default", restore)
	if err != nil {
		t.Fatal(err)
	}

	if err, ok := ferr["something"]; len(ferr) > 1 || !ok || err != "cannot replace a non-file" {
		t.Fatalf("incorrect error or count: %d %s", len(ferr), ferr)
	}

	// Failed items are not expected to be restored.
	// Remove them from expectations
	for name := range ferr {
		delete(restore, name)
	}

	// Check that content of files matches to the version they've been restored.
	for file, version := range restore {
		if runtime.GOOS == "windows" {
			file = filepath.FromSlash(file)
		}
		tag := version.In(locationLocal).Truncate(time.Second).Format(versioner.TimeFormat)
		taggedName := filepath.Join(".stversions", versioner.TagFilename(file, tag))
		fd, err := filesystem.Open(file)
		if err != nil {
			t.Error(err)
		}
		defer fd.Close()

		content, err := ioutil.ReadAll(fd)
		if err != nil {
			t.Error(err)
		}
		if !bytes.Equal(content, []byte(taggedName)) {
			t.Errorf("%s: %s != %s", file, string(content), taggedName)
		}
	}

	// Simple versioner uses modtime for timestamp generation, so we can check
	// if existing stuff was correctly archived as we restored.
	expectArchived := map[string]struct{}{
		"existing":         {},
		"dir/file.txt":     {},
		"dir/existing.txt": {},
	}

	// Even if they are at the archived path, content should have the non
	// archived name.
	for file := range expectArchived {
		if runtime.GOOS == "windows" {
			file = filepath.FromSlash(file)
		}
		taggedName := versioner.TagFilename(file, sentinelTag)
		taggedArchivedName := filepath.Join(".stversions", taggedName)

		fd, err := filesystem.Open(taggedArchivedName)
		if err != nil {
			t.Fatal(err)
		}
		defer fd.Close()

		content, err := ioutil.ReadAll(fd)
		if err != nil {
			t.Error(err)
		}
		if !bytes.Equal(content, []byte(file)) {
			t.Errorf("%s: %s != %s", file, string(content), file)
		}
	}

	// Check for other unexpected things that are tagged.
	filesystem.Walk(".", func(path string, f fs.FileInfo, err error) error {
		if !f.IsRegular() {
			return nil
		}
		if strings.Contains(path, sentinelTag) {
			path = osutil.NormalizedFilename(path)
			name, _ := versioner.UntagFilename(path)
			name = strings.TrimPrefix(name, ".stversions/")
			if _, ok := expectArchived[name]; !ok {
				t.Errorf("unexpected file with sentinel tag: %s", name)
			}
		}
		return nil
	})
}

func TestPausedFolders(t *testing.T) {
	// Create a separate wrapper not to pollute other tests.
	cfg := defaultCfgWrapper.RawCopy()
	wrapper := createTmpWrapper(cfg)
	defer os.Remove(wrapper.ConfigPath())

	db := db.OpenMemory()
	m := NewModel(wrapper, protocol.LocalDeviceID, "syncthing", "dev", db, nil)
	m.AddFolder(defaultFolderConfig)
	m.StartFolder("default")
	m.ServeBackground()
	defer m.Stop()

	if err := m.ScanFolder("default"); err != nil {
		t.Error(err)
	}

	pausedConfig := wrapper.RawCopy()
	pausedConfig.Folders[0].Paused = true
	w, err := m.cfg.Replace(pausedConfig)
	if err != nil {
		t.Fatal(err)
	}
	w.Wait()

	if err := m.ScanFolder("default"); err != ErrFolderPaused {
		t.Errorf("Expected folder paused error, received: %v", err)
	}

	if err := m.ScanFolder("nonexistent"); err != errFolderMissing {
		t.Errorf("Expected missing folder error, received: %v", err)
	}
}

func TestIssue4094(t *testing.T) {
	db := db.OpenMemory()
	// Create a separate wrapper not to pollute other tests.
	wrapper := createTmpWrapper(config.Configuration{})
	defer os.Remove(wrapper.ConfigPath())
	m := NewModel(wrapper, protocol.LocalDeviceID, "syncthing", "dev", db, nil)
	m.ServeBackground()
	defer m.Stop()

	// Force the model to wire itself and add the folders
	folderPath := "testdata/nonexistent"
	defer os.RemoveAll(folderPath)
	cfg := defaultCfgWrapper.RawCopy()
	fcfg := config.FolderConfiguration{
		ID:     "folder1",
		Path:   folderPath,
		Paused: true,
		Devices: []config.FolderDeviceConfiguration{
			{DeviceID: device1},
		},
	}
	cfg.Folders = []config.FolderConfiguration{fcfg}
	p, err := wrapper.Replace(cfg)
	if err != nil {
		t.Fatal(err)
	}
	p.Wait()

	if err := m.SetIgnores(fcfg.ID, []string{"foo"}); err != nil {
		t.Fatalf("failed setting ignores: %v", err)
	}

	if _, err := fcfg.Filesystem().Lstat(".stignore"); err != nil {
		t.Fatalf("failed stating .stignore: %v", err)
	}
}

func TestIssue4903(t *testing.T) {
	db := db.OpenMemory()
	// Create a separate wrapper not to pollute other tests.
	wrapper := createTmpWrapper(config.Configuration{})
	defer os.Remove(wrapper.ConfigPath())
	m := NewModel(wrapper, protocol.LocalDeviceID, "syncthing", "dev", db, nil)
	m.ServeBackground()
	defer m.Stop()

	// Force the model to wire itself and add the folders
	folderPath := "testdata/nonexistent"
	defer os.RemoveAll(folderPath)
	cfg := defaultCfgWrapper.RawCopy()
	fcfg := config.FolderConfiguration{
		ID:     "folder1",
		Path:   folderPath,
		Paused: true,
		Devices: []config.FolderDeviceConfiguration{
			{DeviceID: device1},
		},
	}
	cfg.Folders = []config.FolderConfiguration{fcfg}
	p, err := wrapper.Replace(cfg)
	if err != nil {
		t.Fatal(err)
	}
	p.Wait()

	if err := fcfg.CheckPath(); err != config.ErrPathMissing {
		t.Fatalf("expected path missing error, got: %v", err)
	}

	if _, err := fcfg.Filesystem().Lstat("."); !fs.IsNotExist(err) {
		t.Fatalf("Expected missing path error, got: %v", err)
	}
}

func TestIssue5002(t *testing.T) {
	// recheckFile should not panic when given an index equal to the number of blocks

	db := db.OpenMemory()
	m := NewModel(defaultCfgWrapper, protocol.LocalDeviceID, "syncthing", "dev", db, nil)
	m.AddFolder(defaultFolderConfig)
	m.StartFolder("default")

	m.ServeBackground()
	defer m.Stop()

	if err := m.ScanFolder("default"); err != nil {
		t.Error(err)
	}

	file, ok := m.CurrentFolderFile("default", "foo")
	if !ok {
		t.Fatal("test file should exist")
	}
	nBlocks := len(file.Blocks)

	m.recheckFile(protocol.LocalDeviceID, defaultFolderConfig.Filesystem(), "default", "foo", nBlocks-1, []byte{1, 2, 3, 4})
	m.recheckFile(protocol.LocalDeviceID, defaultFolderConfig.Filesystem(), "default", "foo", nBlocks, []byte{1, 2, 3, 4}) // panic
	m.recheckFile(protocol.LocalDeviceID, defaultFolderConfig.Filesystem(), "default", "foo", nBlocks+1, []byte{1, 2, 3, 4})
}

func TestParentOfUnignored(t *testing.T) {
	wcfg, m := newState(defaultCfg)
	defer func() {
		m.Stop()
		defaultFolderConfig.Filesystem().Remove(".stignore")
		os.Remove(wcfg.ConfigPath())
	}()

	m.SetIgnores("default", []string{"!quux", "*"})

	if parent, ok := m.CurrentFolderFile("default", "baz"); !ok {
		t.Errorf(`Directory "baz" missing in db`)
	} else if parent.IsIgnored() {
		t.Errorf(`Directory "baz" is ignored`)
	}
}

func addFakeConn(m *Model, dev protocol.DeviceID) *fakeConnection {
	fc := &fakeConnection{id: dev, model: m}
	m.AddConnection(fc, protocol.HelloResult{})

	m.ClusterConfig(dev, protocol.ClusterConfig{
		Folders: []protocol.Folder{
			{
				ID: "default",
				Devices: []protocol.Device{
					{ID: device1},
					{ID: device2},
				},
			},
		},
	})

	return fc
}

func TestFolderRestartZombies(t *testing.T) {
	// This is for issue 5233, where multiple concurrent folder restarts
	// would leave more than one folder runner alive.

	wrapper := createTmpWrapper(defaultCfg.Copy())
	defer os.Remove(wrapper.ConfigPath())
	folderCfg, _ := wrapper.Folder("default")
	folderCfg.FilesystemType = fs.FilesystemTypeFake
	wrapper.SetFolder(folderCfg)

	db := db.OpenMemory()
	m := NewModel(wrapper, protocol.LocalDeviceID, "syncthing", "dev", db, nil)
	m.AddFolder(folderCfg)
	m.StartFolder("default")

	m.ServeBackground()
	defer m.Stop()

	// Make sure the folder is up and running, because we want to count it.
	m.ScanFolder("default")

	// Check how many running folders we have running before the test.
	if r := atomic.LoadInt32(&m.foldersRunning); r != 1 {
		t.Error("Expected one running folder, not", r)
	}

	// Run a few parallel configuration changers for one second. Each waits
	// for the commit to complete, but there are many of them.
	var wg sync.WaitGroup
	for i := 0; i < 25; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			t0 := time.Now()
			for time.Since(t0) < time.Second {
				cfg := folderCfg.Copy()
				cfg.MaxConflicts = rand.Int() // safe change that should cause a folder restart
				w, err := wrapper.SetFolder(cfg)
				if err != nil {
					panic(err)
				}
				w.Wait()
			}
		}()
	}

	// Wait for the above to complete and check how many folders we have
	// running now. It should not have increased.
	wg.Wait()
	// Make sure the folder is up and running, because we want to count it.
	m.ScanFolder("default")
	if r := atomic.LoadInt32(&m.foldersRunning); r != 1 {
		t.Error("Expected one running folder, not", r)
	}
}

type fakeAddr struct{}

func (fakeAddr) Network() string {
	return "network"
}

func (fakeAddr) String() string {
	return "address"
}

type alwaysChangedKey struct {
	fs   fs.Filesystem
	name string
}

// alwaysChanges is an ignore.ChangeDetector that always returns true on Changed()
type alwaysChanged struct {
	seen map[alwaysChangedKey]struct{}
}

func newAlwaysChanged() *alwaysChanged {
	return &alwaysChanged{
		seen: make(map[alwaysChangedKey]struct{}),
	}
}

func (c *alwaysChanged) Remember(fs fs.Filesystem, name string, _ time.Time) {
	c.seen[alwaysChangedKey{fs, name}] = struct{}{}
}

func (c *alwaysChanged) Reset() {
	c.seen = make(map[alwaysChangedKey]struct{})
}

func (c *alwaysChanged) Seen(fs fs.Filesystem, name string) bool {
	_, ok := c.seen[alwaysChangedKey{fs, name}]
	return ok
}

func (c *alwaysChanged) Changed() bool {
	return true
}

func TestRequestLimit(t *testing.T) {
	cfg := defaultCfg.Copy()
	cfg.Devices = append(cfg.Devices, config.NewDeviceConfiguration(device2, "device2"))
	cfg.Devices[1].MaxRequestKiB = 1
	cfg.Folders[0].Devices = []config.FolderDeviceConfiguration{
		{DeviceID: device1},
		{DeviceID: device2},
	}
	m, _, wrapper := setupModelWithConnectionManual(cfg)
	defer m.Stop()
	defer os.Remove(wrapper.ConfigPath())

	file := "tmpfile"
	befReq := time.Now()
	first, err := m.Request(device2, "default", file, 2000, 0, nil, 0, false)
	if err != nil {
		t.Fatalf("First request failed: %v", err)
	}
	reqDur := time.Since(befReq)
	returned := make(chan struct{})
	go func() {
		second, err := m.Request(device2, "default", file, 2000, 0, nil, 0, false)
		if err != nil {
			t.Fatalf("Second request failed: %v", err)
		}
		close(returned)
		second.Close()
	}()
	time.Sleep(10 * reqDur)
	select {
	case <-returned:
		t.Fatalf("Second request returned before first was done")
	default:
	}
	first.Close()
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatalf("Second request did not return after first was done")
	}
}

func TestSanitizePath(t *testing.T) {
	cases := [][2]string{
		{"", ""},
		{"foo", "foo"},
		{`\*/foo\?/bar[{!@$%^&*#()}]`, "foo bar ()"},
		{"Räksmörgås", "Räksmörgås"},
		{`Räk \/ smörgås`, "Räk smörgås"},
		{"هذا هو *\x07?اسم الملف", "هذا هو اسم الملف"},
		{`../foo.txt`, `.. foo.txt`},
	}

	for _, tc := range cases {
		res := sanitizePath(tc[0])
		if res != tc[1] {
			t.Errorf("sanitizePath(%q) => %q, expected %q", tc[0], res, tc[1])
		}
	}
}
