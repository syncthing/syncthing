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
	"errors"
	"fmt"
	"io"
	mrand "math/rand"
	"os"
	"path/filepath"
	"runtime/pprof"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/syncthing/syncthing/lib/build"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/db/backend"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/ignore"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/protocol"
	protocolmocks "github.com/syncthing/syncthing/lib/protocol/mocks"
	srand "github.com/syncthing/syncthing/lib/rand"
	"github.com/syncthing/syncthing/lib/semaphore"
	"github.com/syncthing/syncthing/lib/testutil"
	"github.com/syncthing/syncthing/lib/versioner"
)

func newState(t testing.TB, cfg config.Configuration) (*testModel, context.CancelFunc) {
	wcfg, cancel := newConfigWrapper(cfg)

	m := setupModel(t, wcfg)

	for _, dev := range cfg.Devices {
		m.AddConnection(newFakeConnection(dev.DeviceID, m), protocol.Hello{})
	}

	return m, cancel
}

func createClusterConfig(remote protocol.DeviceID, ids ...string) protocol.ClusterConfig {
	cc := protocol.ClusterConfig{
		Folders: make([]protocol.Folder, len(ids)),
	}
	for i, id := range ids {
		cc.Folders[i] = protocol.Folder{
			ID:    id,
			Label: id,
		}
	}
	return addFolderDevicesToClusterConfig(cc, remote)
}

func addFolderDevicesToClusterConfig(cc protocol.ClusterConfig, remote protocol.DeviceID) protocol.ClusterConfig {
	for i := range cc.Folders {
		cc.Folders[i].Devices = []protocol.Device{
			{ID: myID},
			{ID: remote},
		}
	}
	return cc
}

func TestRequest(t *testing.T) {
	wrapper, fcfg, cancel := newDefaultCfgWrapper()
	ffs := fcfg.Filesystem(nil)
	defer cancel()
	m := setupModel(t, wrapper)
	defer cleanupModel(m)

	fd, err := ffs.Create("foo")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fd.Write([]byte("foobar")); err != nil {
		t.Fatal(err)
	}
	fd.Close()

	m.ScanFolder("default")

	// Existing, shared file
	res, err := m.Request(device1Conn, "default", "foo", 0, 6, 0, nil, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	bs := res.Data()
	if !bytes.Equal(bs, []byte("foobar")) {
		t.Errorf("Incorrect data from request: %q", string(bs))
	}

	// Existing, nonshared file
	_, err = m.Request(device2Conn, "default", "foo", 0, 6, 0, nil, 0, false)
	if err == nil {
		t.Error("Unexpected nil error on insecure file read")
	}

	// Nonexistent file
	_, err = m.Request(device1Conn, "default", "nonexistent", 0, 6, 0, nil, 0, false)
	if err == nil {
		t.Error("Unexpected nil error on insecure file read")
	}

	// Shared folder, but disallowed file name
	_, err = m.Request(device1Conn, "default", "../walk.go", 0, 6, 0, nil, 0, false)
	if err == nil {
		t.Error("Unexpected nil error on insecure file read")
	}

	// Negative offset
	_, err = m.Request(device1Conn, "default", "foo", 0, -4, 0, nil, 0, false)
	if err == nil {
		t.Error("Unexpected nil error on insecure file read")
	}

	// Larger block than available
	_, err = m.Request(device1Conn, "default", "foo", 0, 42, 0, []byte("hash necessary but not checked"), 0, false)
	if err == nil {
		t.Error("Unexpected nil error on read past end of file")
	}
	_, err = m.Request(device1Conn, "default", "foo", 0, 42, 0, nil, 0, false)
	if err != nil {
		t.Error("Unexpected error when large read should be permitted")
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
	m, _, fcfg, wcfgCancel := setupModelWithConnection(b)
	defer wcfgCancel()
	defer cleanupModelAndRemoveDir(m, fcfg.Filesystem(nil).URI())

	files := genFiles(nfiles)
	must(b, m.Index(device1Conn, fcfg.ID, files))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		must(b, m.Index(device1Conn, fcfg.ID, files))
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
	m, _, fcfg, wcfgCancel := setupModelWithConnection(b)
	defer wcfgCancel()
	defer cleanupModelAndRemoveDir(m, fcfg.Filesystem(nil).URI())

	files := genFiles(nfiles)
	ufiles := genFiles(nufiles)

	must(b, m.Index(device1Conn, fcfg.ID, files))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		must(b, m.IndexUpdate(device1Conn, fcfg.ID, ufiles))
	}
	b.ReportAllocs()
}

func BenchmarkRequestOut(b *testing.B) {
	m := setupModel(b, defaultCfgWrapper)
	defer cleanupModel(m)

	const n = 1000
	files := genFiles(n)

	fc := newFakeConnection(device1, m)
	for _, f := range files {
		fc.addFile(f.Name, 0o644, protocol.FileInfoTypeFile, []byte("some data to return"))
	}
	m.AddConnection(fc, protocol.Hello{})
	must(b, m.Index(device1Conn, "default", files))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		data, err := m.requestGlobal(context.Background(), device1, "default", files[i%n].Name, 0, 0, 32, nil, 0, false)
		if err != nil {
			b.Error(err)
		}
		if data == nil {
			b.Error("nil data")
		}
	}
}

func BenchmarkRequestInSingleFile(b *testing.B) {
	w, cancel := newConfigWrapper(defaultCfg)
	defer cancel()
	ffs := w.FolderList()[0].Filesystem(nil)
	m := setupModel(b, w)
	defer cleanupModel(m)

	buf := make([]byte, 128<<10)
	srand.Read(buf)
	must(b, ffs.MkdirAll("request/for/a/file/in/a/couple/of/dirs", 0o755))
	writeFile(b, ffs, "request/for/a/file/in/a/couple/of/dirs/128k", buf)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := m.Request(device1Conn, "default", "request/for/a/file/in/a/couple/of/dirs/128k", 0, 128<<10, 0, nil, 0, false); err != nil {
			b.Error(err)
		}
	}

	b.SetBytes(128 << 10)
}

func TestDeviceRename(t *testing.T) {
	hello := protocol.Hello{
		ClientName:    "syncthing",
		ClientVersion: "v0.9.4",
	}

	rawCfg := config.New(device1)
	rawCfg.Devices = []config.DeviceConfiguration{
		{
			DeviceID: device1,
		},
	}
	cfg, cfgCancel := newConfigWrapper(rawCfg)
	defer cfgCancel()

	m := newModel(t, cfg, myID, "syncthing", "dev", nil)

	if cfg.Devices()[device1].Name != "" {
		t.Errorf("Device already has a name")
	}

	conn := newFakeConnection(device1, m)

	m.AddConnection(conn, hello)

	m.ServeBackground()
	defer cleanupModel(m)

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

	ffs := fs.NewFilesystem(fs.FilesystemTypeFake, srand.String(32)+"?content=true")
	path := "someConfigfile"

	must(t, saveConfig(ffs, path, cfg.RawCopy()))

	cfgw, _, err := loadConfig(ffs, path, myID, events.NoopLogger)
	if err != nil {
		t.Error(err)
		return
	}
	if cfgw.Devices()[device1].Name != "tester" {
		t.Errorf("Device name not saved in config")
	}

	m.Closed(conn, protocol.ErrTimeout)

	waiter, err := cfg.Modify(func(cfg *config.Configuration) {
		cfg.Options.OverwriteRemoteDevNames = true
	})
	must(t, err)
	waiter.Wait()

	hello.DeviceName = "tester2"
	m.AddConnection(conn, hello)

	if cfg.Devices()[device1].Name != "tester2" {
		t.Errorf("Device name not overwritten")
	}
}

// Adjusted copy of the original function for testing purposes
func saveConfig(ffs fs.Filesystem, path string, cfg config.Configuration) error {
	fd, err := ffs.Create(path)
	if err != nil {
		l.Debugln("Create:", err)
		return err
	}

	if err := cfg.WriteXML(osutil.LineEndingsWriter(fd)); err != nil {
		l.Debugln("WriteXML:", err)
		fd.Close()
		return err
	}

	if err := fd.Close(); err != nil {
		l.Debugln("Close:", err)
		return err
	}
	if _, err := ffs.Lstat(path); err != nil {
		return err
	}

	return nil
}

// Adjusted copy of the original function for testing purposes
func loadConfig(ffs fs.Filesystem, path string, myID protocol.DeviceID, evLogger events.Logger) (config.Wrapper, int, error) {
	if _, err := ffs.Lstat(path); err != nil {
		return nil, 0, err
	}
	fd, err := ffs.OpenFile(path, fs.OptReadWrite, 0o666)
	if err != nil {
		return nil, 0, err
	}
	defer fd.Close()

	cfg, originalVersion, err := config.ReadXML(fd, myID)
	if err != nil {
		return nil, 0, err
	}

	return config.Wrap(path, cfg, myID, evLogger), originalVersion, nil
}

func TestClusterConfig(t *testing.T) {
	cfg := config.New(device1)
	cfg.Options.MinHomeDiskFree.Value = 0 // avoids unnecessary free space checks
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
			FilesystemType: fs.FilesystemTypeFake,
			ID:             "folder1",
			Path:           "testdata1",
			Devices: []config.FolderDeviceConfiguration{
				{DeviceID: device1},
				{DeviceID: device2},
			},
		},
		{
			FilesystemType: fs.FilesystemTypeFake,
			ID:             "folder2",
			Path:           "testdata2",
			Paused:         true, // should still be included
			Devices: []config.FolderDeviceConfiguration{
				{DeviceID: device1},
				{DeviceID: device2},
			},
		},
		{
			FilesystemType: fs.FilesystemTypeFake,
			ID:             "folder3",
			Path:           "testdata3",
			Devices: []config.FolderDeviceConfiguration{
				{DeviceID: device1},
				// should not be included, does not include device2
			},
		},
	}

	wrapper, cancel := newConfigWrapper(cfg)
	defer cancel()
	m := newModel(t, wrapper, myID, "syncthing", "dev", nil)
	m.ServeBackground()
	defer cleanupModel(m)

	cm, _ := m.generateClusterConfig(device2)

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

	m, cancel := newState(t, config.Configuration{
		Version: config.CurrentVersion,
		Devices: []config.DeviceConfiguration{
			{
				DeviceID:   device1,
				Introducer: true,
			},
		},
		Folders: []config.FolderConfiguration{
			{
				FilesystemType: fs.FilesystemTypeFake,
				ID:             "folder1",
				Path:           "testdata",
				Devices: []config.FolderDeviceConfiguration{
					{DeviceID: device1},
				},
			},
			{
				FilesystemType: fs.FilesystemTypeFake,
				ID:             "folder2",
				Path:           "testdata",
				Devices: []config.FolderDeviceConfiguration{
					{DeviceID: device1},
				},
			},
		},
	})
	cc := basicClusterConfig(myID, device1, "folder1", "folder2")
	cc.Folders[0].Devices = append(cc.Folders[0].Devices, protocol.Device{
		ID:                       device2,
		Introducer:               true,
		SkipIntroductionRemovals: true,
	})
	cc.Folders[1].Devices = append(cc.Folders[1].Devices, protocol.Device{
		ID:                       device2,
		Introducer:               true,
		SkipIntroductionRemovals: true,
		EncryptionPasswordToken:  []byte("faketoken"),
	})
	m.ClusterConfig(device1Conn, cc)

	if newDev, ok := m.cfg.Device(device2); !ok || !newDev.Introducer || !newDev.SkipIntroductionRemovals {
		t.Error("device 2 missing or wrong flags")
	}

	if !contains(m.cfg.Folders()["folder1"], device2, device1) {
		t.Error("expected folder 1 to have device2 introduced by device 1")
	}

	for _, devCfg := range m.cfg.Folders()["folder2"].Devices {
		if devCfg.DeviceID == device2 {
			t.Error("Device was added even though it's untrusted")
		}
	}

	cleanupModel(m)
	cancel()
	m, cancel = newState(t, config.Configuration{
		Version: config.CurrentVersion,
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
				FilesystemType: fs.FilesystemTypeFake,
				ID:             "folder1",
				Path:           "testdata",
				Devices: []config.FolderDeviceConfiguration{
					{DeviceID: device1},
					{DeviceID: device2, IntroducedBy: device1},
				},
			},
			{
				FilesystemType: fs.FilesystemTypeFake,
				ID:             "folder2",
				Path:           "testdata",
				Devices: []config.FolderDeviceConfiguration{
					{DeviceID: device1},
				},
			},
		},
	})
	cc = basicClusterConfig(myID, device1, "folder2")
	cc.Folders[0].Devices = append(cc.Folders[0].Devices, protocol.Device{
		ID:                       device2,
		Introducer:               true,
		SkipIntroductionRemovals: true,
	})
	m.ClusterConfig(device1Conn, cc)

	// Should not get introducer, as it's already unset, and it's an existing device.
	if newDev, ok := m.cfg.Device(device2); !ok || newDev.Introducer || newDev.SkipIntroductionRemovals {
		t.Error("device 2 missing or changed flags")
	}

	if contains(m.cfg.Folders()["folder1"], device2, introducedByAnyone) {
		t.Error("expected device 2 to be removed from folder 1")
	}

	if !contains(m.cfg.Folders()["folder2"], device2, device1) {
		t.Error("expected device 2 to be added to folder 2")
	}

	cleanupModel(m)
	cancel()
	m, cancel = newState(t, config.Configuration{
		Version: config.CurrentVersion,
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
				FilesystemType: fs.FilesystemTypeFake,
				ID:             "folder1",
				Path:           "testdata",
				Devices: []config.FolderDeviceConfiguration{
					{DeviceID: device1},
					{DeviceID: device2, IntroducedBy: device1},
				},
			},
			{
				FilesystemType: fs.FilesystemTypeFake,
				ID:             "folder2",
				Path:           "testdata",
				Devices: []config.FolderDeviceConfiguration{
					{DeviceID: device1},
					{DeviceID: device2, IntroducedBy: device1},
				},
			},
		},
	})
	m.ClusterConfig(device1Conn, protocol.ClusterConfig{})

	if _, ok := m.cfg.Device(device2); ok {
		t.Error("device 2 should have been removed")
	}

	if contains(m.cfg.Folders()["folder1"], device2, introducedByAnyone) {
		t.Error("expected device 2 to be removed from folder 1")
	}

	if contains(m.cfg.Folders()["folder2"], device2, introducedByAnyone) {
		t.Error("expected device 2 to be removed from folder 2")
	}

	// Two cases when removals should not happen
	// 1. Introducer flag no longer set on device

	cleanupModel(m)
	cancel()
	m, cancel = newState(t, config.Configuration{
		Version: config.CurrentVersion,
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
				FilesystemType: fs.FilesystemTypeFake,
				ID:             "folder1",
				Path:           "testdata",
				Devices: []config.FolderDeviceConfiguration{
					{DeviceID: device1},
					{DeviceID: device2, IntroducedBy: device1},
				},
			},
			{
				FilesystemType: fs.FilesystemTypeFake,
				ID:             "folder2",
				Path:           "testdata",
				Devices: []config.FolderDeviceConfiguration{
					{DeviceID: device1},
					{DeviceID: device2, IntroducedBy: device1},
				},
			},
		},
	})
	m.ClusterConfig(device1Conn, protocol.ClusterConfig{})

	if _, ok := m.cfg.Device(device2); !ok {
		t.Error("device 2 should not have been removed")
	}

	if !contains(m.cfg.Folders()["folder1"], device2, device1) {
		t.Error("expected device 2 not to be removed from folder 1")
	}

	if !contains(m.cfg.Folders()["folder2"], device2, device1) {
		t.Error("expected device 2 not to be removed from folder 2")
	}

	// 2. SkipIntroductionRemovals is set

	cleanupModel(m)
	cancel()
	m, cancel = newState(t, config.Configuration{
		Version: config.CurrentVersion,
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
				FilesystemType: fs.FilesystemTypeFake,
				ID:             "folder1",
				Path:           "testdata",
				Devices: []config.FolderDeviceConfiguration{
					{DeviceID: device1},
					{DeviceID: device2, IntroducedBy: device1},
				},
			},
			{
				FilesystemType: fs.FilesystemTypeFake,
				ID:             "folder2",
				Path:           "testdata",
				Devices: []config.FolderDeviceConfiguration{
					{DeviceID: device1},
				},
			},
		},
	})
	cc = basicClusterConfig(myID, device1, "folder2")
	cc.Folders[0].Devices = append(cc.Folders[0].Devices, protocol.Device{
		ID:                       device2,
		Introducer:               true,
		SkipIntroductionRemovals: true,
	})
	m.ClusterConfig(device1Conn, cc)

	if _, ok := m.cfg.Device(device2); !ok {
		t.Error("device 2 should not have been removed")
	}

	if !contains(m.cfg.Folders()["folder1"], device2, device1) {
		t.Error("expected device 2 not to be removed from folder 1")
	}

	if !contains(m.cfg.Folders()["folder2"], device2, device1) {
		t.Error("expected device 2 not to be added to folder 2")
	}

	// Test device not being removed as it's shared without an introducer.

	cleanupModel(m)
	cancel()
	m, cancel = newState(t, config.Configuration{
		Version: config.CurrentVersion,
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
				FilesystemType: fs.FilesystemTypeFake,
				ID:             "folder1",
				Path:           "testdata",
				Devices: []config.FolderDeviceConfiguration{
					{DeviceID: device1},
					{DeviceID: device2, IntroducedBy: device1},
				},
			},
			{
				FilesystemType: fs.FilesystemTypeFake,
				ID:             "folder2",
				Path:           "testdata",
				Devices: []config.FolderDeviceConfiguration{
					{DeviceID: device1},
					{DeviceID: device2},
				},
			},
		},
	})
	m.ClusterConfig(device1Conn, protocol.ClusterConfig{})

	if _, ok := m.cfg.Device(device2); !ok {
		t.Error("device 2 should not have been removed")
	}

	if contains(m.cfg.Folders()["folder1"], device2, introducedByAnyone) {
		t.Error("expected device 2 to be removed from folder 1")
	}

	if !contains(m.cfg.Folders()["folder2"], device2, introducedByAnyone) {
		t.Error("expected device 2 not to be removed from folder 2")
	}

	// Test device not being removed as it's shared by a different introducer.

	cleanupModel(m)
	cancel()
	m, cancel = newState(t, config.Configuration{
		Version: config.CurrentVersion,
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
				FilesystemType: fs.FilesystemTypeFake,
				ID:             "folder1",
				Path:           "testdata",
				Devices: []config.FolderDeviceConfiguration{
					{DeviceID: device1},
					{DeviceID: device2, IntroducedBy: device1},
				},
			},
			{
				FilesystemType: fs.FilesystemTypeFake,
				ID:             "folder2",
				Path:           "testdata",
				Devices: []config.FolderDeviceConfiguration{
					{DeviceID: device1},
					{DeviceID: device2, IntroducedBy: myID},
				},
			},
		},
	})
	defer cleanupModel(m)
	defer cancel()
	m.ClusterConfig(device1Conn, protocol.ClusterConfig{})

	if _, ok := m.cfg.Device(device2); !ok {
		t.Error("device 2 should not have been removed")
	}

	if contains(m.cfg.Folders()["folder1"], device2, introducedByAnyone) {
		t.Error("expected device 2 to be removed from folder 1")
	}

	if !contains(m.cfg.Folders()["folder2"], device2, introducedByAnyone) {
		t.Error("expected device 2 not to be removed from folder 2")
	}
}

func TestIssue4897(t *testing.T) {
	m, cancel := newState(t, config.Configuration{
		Version: config.CurrentVersion,
		Devices: []config.DeviceConfiguration{
			{
				DeviceID:   device1,
				Introducer: true,
			},
		},
		Folders: []config.FolderConfiguration{
			{
				FilesystemType: fs.FilesystemTypeFake,
				ID:             "folder1",
				Path:           "testdata",
				Devices: []config.FolderDeviceConfiguration{
					{DeviceID: device1},
				},
				Paused: true,
			},
		},
	})
	defer cleanupModel(m)
	cancel()

	cm, _ := m.generateClusterConfig(device1)
	if l := len(cm.Folders); l != 1 {
		t.Errorf("Cluster config contains %v folders, expected 1", l)
	}
}

// TestIssue5063 is about a panic in connection with modifying config in quick
// succession, related with auto accepted folders. It's unclear what exactly, a
// relevant bit seems to be here:
// PR-comments: https://github.com/syncthing/syncthing/pull/5069/files#r203146546
// Issue: https://github.com/syncthing/syncthing/pull/5509
func TestIssue5063(t *testing.T) {
	m, cancel := newState(t, defaultAutoAcceptCfg)
	defer cleanupModel(m)
	defer cancel()

	m.pmut.Lock()
	for _, c := range m.conn {
		conn := c.(*fakeConnection)
		conn.CloseCalls(func(_ error) {})
		defer m.Closed(c, errStopped) // to unblock deferred m.Stop()
	}
	m.pmut.Unlock()

	wg := sync.WaitGroup{}

	addAndVerify := func(id string) {
		m.ClusterConfig(device1Conn, createClusterConfig(device1, id))
		if fcfg, ok := m.cfg.Folder(id); !ok || !fcfg.SharedWith(device1) {
			t.Error("expected shared", id)
		}
		wg.Done()
	}

	reps := 10
	ids := make([]string, reps)
	for i := 0; i < reps; i++ {
		wg.Add(1)
		ids[i] = srand.String(8)
		go addAndVerify(ids[i])
	}

	finished := make(chan struct{})
	go func() {
		wg.Wait()
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(10 * time.Second):
		pprof.Lookup("goroutine").WriteTo(os.Stdout, 1)
		t.Fatal("Timed out before all devices were added")
	}
}

func TestAutoAcceptRejected(t *testing.T) {
	// Nothing happens if AutoAcceptFolders not set
	tcfg := defaultAutoAcceptCfg.Copy()
	for i := range tcfg.Devices {
		tcfg.Devices[i].AutoAcceptFolders = false
	}
	m, cancel := newState(t, tcfg)
	// defer cleanupModel(m)
	defer cancel()
	id := srand.String(8)
	m.ClusterConfig(device1Conn, createClusterConfig(device1, id))

	if cfg, ok := m.cfg.Folder(id); ok && cfg.SharedWith(device1) {
		t.Error("unexpected shared", id)
	}
}

func TestAutoAcceptNewFolder(t *testing.T) {
	// New folder
	m, cancel := newState(t, defaultAutoAcceptCfg)
	defer cleanupModel(m)
	defer cancel()
	id := srand.String(8)
	m.ClusterConfig(device1Conn, createClusterConfig(device1, id))
	if fcfg, ok := m.cfg.Folder(id); !ok || !fcfg.SharedWith(device1) {
		t.Error("expected shared", id)
	}
}

func TestAutoAcceptNewFolderFromTwoDevices(t *testing.T) {
	m, cancel := newState(t, defaultAutoAcceptCfg)
	defer cleanupModel(m)
	defer cancel()
	id := srand.String(8)
	defer os.RemoveAll(id)
	m.ClusterConfig(device1Conn, createClusterConfig(device1, id))
	if fcfg, ok := m.cfg.Folder(id); !ok || !fcfg.SharedWith(device1) {
		t.Error("expected shared", id)
	}
	if fcfg, ok := m.cfg.Folder(id); !ok || fcfg.SharedWith(device2) {
		t.Error("unexpected expected shared", id)
	}
	m.ClusterConfig(device2Conn, createClusterConfig(device2, id))
	if fcfg, ok := m.cfg.Folder(id); !ok || !fcfg.SharedWith(device2) {
		t.Error("expected shared", id)
	}
}

func TestAutoAcceptNewFolderFromOnlyOneDevice(t *testing.T) {
	modifiedCfg := defaultAutoAcceptCfg.Copy()
	modifiedCfg.Devices[2].AutoAcceptFolders = false
	m, cancel := newState(t, modifiedCfg)
	id := srand.String(8)
	defer os.RemoveAll(id)
	defer cleanupModel(m)
	defer cancel()
	m.ClusterConfig(device1Conn, createClusterConfig(device1, id))
	if fcfg, ok := m.cfg.Folder(id); !ok || !fcfg.SharedWith(device1) {
		t.Error("expected shared", id)
	}
	if fcfg, ok := m.cfg.Folder(id); !ok || fcfg.SharedWith(device2) {
		t.Error("unexpected expected shared", id)
	}
	m.ClusterConfig(device2Conn, createClusterConfig(device2, id))
	if fcfg, ok := m.cfg.Folder(id); !ok || fcfg.SharedWith(device2) {
		t.Error("unexpected shared", id)
	}
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
						fcfg := newFolderConfiguration(defaultCfgWrapper, localFolder.ID, localFolder.Label, fs.FilesystemTypeFake, localFolder.ID)
						fcfg.Paused = localFolderPaused
						cfg.Folders = append(cfg.Folders, fcfg)
					}
					m, cancel := newState(t, cfg)
					m.ClusterConfig(device1Conn, protocol.ClusterConfig{
						Folders: []protocol.Folder{dev1folder},
					})
					m.ClusterConfig(device2Conn, protocol.ClusterConfig{
						Folders: []protocol.Folder{dev2folder},
					})
					cleanupModel(m)
					cancel()
				}
			}
		}
	}
}

func TestAutoAcceptMultipleFolders(t *testing.T) {
	// Multiple new folders
	id1 := srand.String(8)
	defer os.RemoveAll(id1)
	id2 := srand.String(8)
	defer os.RemoveAll(id2)
	m, cancel := newState(t, defaultAutoAcceptCfg)
	defer cleanupModel(m)
	defer cancel()
	m.ClusterConfig(device1Conn, createClusterConfig(device1, id1, id2))
	if fcfg, ok := m.cfg.Folder(id1); !ok || !fcfg.SharedWith(device1) {
		t.Error("expected shared", id1)
	}
	if fcfg, ok := m.cfg.Folder(id2); !ok || !fcfg.SharedWith(device1) {
		t.Error("expected shared", id2)
	}
}

func TestAutoAcceptExistingFolder(t *testing.T) {
	// Existing folder
	id := srand.String(8)
	idOther := srand.String(8) // To check that path does not get changed.

	tcfg := defaultAutoAcceptCfg.Copy()
	tcfg.Folders = []config.FolderConfiguration{
		{
			FilesystemType: fs.FilesystemTypeFake,
			ID:             id,
			Path:           idOther, // To check that path does not get changed.
		},
	}
	m, cancel := newState(t, tcfg)
	defer cleanupModel(m)
	defer cancel()
	if fcfg, ok := m.cfg.Folder(id); !ok || fcfg.SharedWith(device1) {
		t.Error("missing folder, or shared", id)
	}
	m.ClusterConfig(device1Conn, createClusterConfig(device1, id))

	if fcfg, ok := m.cfg.Folder(id); !ok || !fcfg.SharedWith(device1) || fcfg.Path != idOther {
		t.Error("missing folder, or unshared, or path changed", id)
	}
}

func TestAutoAcceptNewAndExistingFolder(t *testing.T) {
	// New and existing folder
	id1 := srand.String(8)
	id2 := srand.String(8)

	tcfg := defaultAutoAcceptCfg.Copy()
	tcfg.Folders = []config.FolderConfiguration{
		{
			FilesystemType: fs.FilesystemTypeFake,
			ID:             id1,
			Path:           id1, // from previous test case, to verify that path doesn't get changed.
		},
	}
	m, cancel := newState(t, tcfg)
	defer cleanupModel(m)
	defer cancel()
	if fcfg, ok := m.cfg.Folder(id1); !ok || fcfg.SharedWith(device1) {
		t.Error("missing folder, or shared", id1)
	}
	m.ClusterConfig(device1Conn, createClusterConfig(device1, id1, id2))

	for i, id := range []string{id1, id2} {
		if fcfg, ok := m.cfg.Folder(id); !ok || !fcfg.SharedWith(device1) {
			t.Error("missing folder, or unshared", i, id)
		}
	}
}

func TestAutoAcceptAlreadyShared(t *testing.T) {
	// Already shared
	id := srand.String(8)
	tcfg := defaultAutoAcceptCfg.Copy()
	tcfg.Folders = []config.FolderConfiguration{
		{
			FilesystemType: fs.FilesystemTypeFake,
			ID:             id,
			Path:           id,
			Devices: []config.FolderDeviceConfiguration{
				{
					DeviceID: device1,
				},
			},
		},
	}
	m, cancel := newState(t, tcfg)
	defer cleanupModel(m)
	defer cancel()
	if fcfg, ok := m.cfg.Folder(id); !ok || !fcfg.SharedWith(device1) {
		t.Error("missing folder, or not shared", id)
	}
	m.ClusterConfig(device1Conn, createClusterConfig(device1, id))

	if fcfg, ok := m.cfg.Folder(id); !ok || !fcfg.SharedWith(device1) {
		t.Error("missing folder, or not shared", id)
	}
}

func TestAutoAcceptNameConflict(t *testing.T) {
	ffs := fs.NewFilesystem(fs.FilesystemTypeFake, srand.String(32))
	id := srand.String(8)
	label := srand.String(8)
	ffs.MkdirAll(id, 0o777)
	ffs.MkdirAll(label, 0o777)
	m, cancel := newState(t, defaultAutoAcceptCfg)
	defer cleanupModel(m)
	defer cancel()
	m.ClusterConfig(device1Conn, protocol.ClusterConfig{
		Folders: []protocol.Folder{
			{
				ID:    id,
				Label: label,
			},
		},
	})
	if fcfg, ok := m.cfg.Folder(id); ok && fcfg.SharedWith(device1) {
		t.Error("unexpected folder", id)
	}
}

func TestAutoAcceptPrefersLabel(t *testing.T) {
	// Prefers label, falls back to ID.
	m, cancel := newState(t, defaultAutoAcceptCfg)
	id := srand.String(8)
	label := srand.String(8)
	defer cleanupModel(m)
	defer cancel()
	m.ClusterConfig(device1Conn, addFolderDevicesToClusterConfig(protocol.ClusterConfig{
		Folders: []protocol.Folder{
			{
				ID:    id,
				Label: label,
			},
		},
	}, device1))
	if fcfg, ok := m.cfg.Folder(id); !ok || !fcfg.SharedWith(device1) || !strings.HasSuffix(fcfg.Path, label) {
		t.Error("expected shared, or wrong path", id, label, fcfg.Path)
	}
}

func TestAutoAcceptFallsBackToID(t *testing.T) {
	// Prefers label, falls back to ID.
	m, cancel := newState(t, defaultAutoAcceptCfg)
	ffs := defaultFolderConfig.Filesystem(nil)
	id := srand.String(8)
	label := srand.String(8)
	if err := ffs.MkdirAll(label, 0o777); err != nil {
		t.Error(err)
	}
	defer cleanupModel(m)
	defer cancel()
	m.ClusterConfig(device1Conn, addFolderDevicesToClusterConfig(protocol.ClusterConfig{
		Folders: []protocol.Folder{
			{
				ID:    id,
				Label: label,
			},
		},
	}, device1))
	fcfg, ok := m.cfg.Folder(id)
	if !ok {
		t.Error("folder configuration missing")
	}
	if !fcfg.SharedWith(device1) {
		t.Error("folder is not shared with device1")
	}
}

func TestAutoAcceptPausedWhenFolderConfigChanged(t *testing.T) {
	// Existing folder
	id := srand.String(8)
	idOther := srand.String(8) // To check that path does not get changed.

	tcfg := defaultAutoAcceptCfg.Copy()
	fcfg := newFolderConfiguration(defaultCfgWrapper, id, "", fs.FilesystemTypeFake, idOther)
	fcfg.Paused = true
	// The order of devices here is wrong (cfg.clean() sorts them), which will cause the folder to restart.
	// Because of the restart, folder gets removed from m.deviceFolder, which means that generateClusterConfig will not panic.
	// This wasn't an issue before, yet keeping the test case to prove that it still isn't.
	fcfg.Devices = append(fcfg.Devices, config.FolderDeviceConfiguration{
		DeviceID: device1,
	})
	tcfg.Folders = []config.FolderConfiguration{fcfg}
	m, cancel := newState(t, tcfg)
	defer cleanupModel(m)
	defer cancel()
	if fcfg, ok := m.cfg.Folder(id); !ok || !fcfg.SharedWith(device1) {
		t.Error("missing folder, or not shared", id)
	}
	if _, ok := m.folderRunners[id]; ok {
		t.Fatal("folder running?")
	}

	m.ClusterConfig(device1Conn, createClusterConfig(device1, id))
	m.generateClusterConfig(device1)

	if fcfg, ok := m.cfg.Folder(id); !ok {
		t.Error("missing folder")
	} else if fcfg.Path != idOther {
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

	tcfg := defaultAutoAcceptCfg.Copy()
	fcfg := newFolderConfiguration(defaultCfgWrapper, id, "", fs.FilesystemTypeFake, idOther)
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
	m, cancel := newState(t, tcfg)
	defer cleanupModel(m)
	defer cancel()
	if fcfg, ok := m.cfg.Folder(id); !ok || !fcfg.SharedWith(device1) {
		t.Error("missing folder, or not shared", id)
	}
	if _, ok := m.folderRunners[id]; ok {
		t.Fatal("folder running?")
	}

	m.ClusterConfig(device1Conn, createClusterConfig(device1, id))
	m.generateClusterConfig(device1)

	if fcfg, ok := m.cfg.Folder(id); !ok {
		t.Error("missing folder")
	} else if fcfg.Path != idOther {
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

func TestAutoAcceptEnc(t *testing.T) {
	tcfg := defaultAutoAcceptCfg.Copy()
	m, cancel := newState(t, tcfg)
	defer cleanupModel(m)
	defer cancel()

	id := srand.String(8)
	defer os.RemoveAll(id)

	token := []byte("token")
	basicCC := func() protocol.ClusterConfig {
		return protocol.ClusterConfig{
			Folders: []protocol.Folder{{
				ID:    id,
				Label: id,
			}},
		}
	}

	// Earlier tests might cause the connection to get closed, thus ClusterConfig
	// would panic.
	clusterConfig := func(deviceID protocol.DeviceID, cm protocol.ClusterConfig) {
		m.AddConnection(newFakeConnection(deviceID, m), protocol.Hello{})
		m.ClusterConfig(&protocolmocks.Connection{DeviceIDStub: func() protocol.DeviceID { return deviceID }}, cm)
	}

	clusterConfig(device1, basicCC())
	if _, ok := m.cfg.Folder(id); ok {
		t.Fatal("unexpected added")
	}
	cc := basicCC()
	cc.Folders[0].Devices = []protocol.Device{{ID: device1}}
	clusterConfig(device1, cc)
	if _, ok := m.cfg.Folder(id); ok {
		t.Fatal("unexpected added")
	}
	cc = basicCC()
	cc.Folders[0].Devices = []protocol.Device{{ID: myID}}
	clusterConfig(device1, cc)
	if _, ok := m.cfg.Folder(id); ok {
		t.Fatal("unexpected added")
	}

	// New folder, encrypted -> add as enc

	cc = createClusterConfig(device1, id)
	cc.Folders[0].Devices[1].EncryptionPasswordToken = token
	clusterConfig(device1, cc)
	if cfg, ok := m.cfg.Folder(id); !ok {
		t.Fatal("unexpected unadded")
	} else {
		if !cfg.SharedWith(device1) {
			t.Fatal("unexpected unshared")
		}
		if cfg.Type != config.FolderTypeReceiveEncrypted {
			t.Fatal("Folder not added as receiveEncrypted")
		}
	}

	// New device, unencrypted on encrypted folder -> reject

	clusterConfig(device2, createClusterConfig(device2, id))
	if cfg, _ := m.cfg.Folder(id); cfg.SharedWith(device2) {
		t.Fatal("unexpected shared")
	}

	// New device, encrypted on encrypted folder -> share

	cc = createClusterConfig(device2, id)
	cc.Folders[0].Devices[1].EncryptionPasswordToken = token
	clusterConfig(device2, cc)
	if cfg, _ := m.cfg.Folder(id); !cfg.SharedWith(device2) {
		t.Fatal("unexpected unshared")
	}

	// New folder, no encrypted -> add "normal"

	id = srand.String(8)
	defer os.RemoveAll(id)

	clusterConfig(device1, createClusterConfig(device1, id))
	if cfg, ok := m.cfg.Folder(id); !ok {
		t.Fatal("unexpected unadded")
	} else {
		if !cfg.SharedWith(device1) {
			t.Fatal("unexpected unshared")
		}
		if cfg.Type != config.FolderTypeSendReceive {
			t.Fatal("Folder not added as send-receive")
		}
	}

	// New device, encrypted on unencrypted folder -> reject

	cc = createClusterConfig(device2, id)
	cc.Folders[0].Devices[1].EncryptionPasswordToken = token
	clusterConfig(device2, cc)
	if cfg, _ := m.cfg.Folder(id); cfg.SharedWith(device2) {
		t.Fatal("unexpected shared")
	}

	// New device, unencrypted on unencrypted folder -> share

	clusterConfig(device2, createClusterConfig(device2, id))
	if cfg, _ := m.cfg.Folder(id); !cfg.SharedWith(device2) {
		t.Fatal("unexpected unshared")
	}
}

func changeIgnores(t *testing.T, m *testModel, expected []string) {
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

	ignores, _, err := m.LoadIgnores("default")
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

	ignores2, _, err := m.LoadIgnores("default")
	if err != nil {
		t.Error(err)
	}

	if !arrEqual(ignores, ignores2) {
		t.Errorf("Incorrect ignores: %v != %v", ignores2, ignores)
	}

	if build.IsDarwin {
		// see above
		time.Sleep(time.Second)
	} else {
		time.Sleep(time.Millisecond)
	}
	err = m.SetIgnores("default", expected)
	if err != nil {
		t.Error(err)
	}

	ignores, _, err = m.LoadIgnores("default")
	if err != nil {
		t.Error(err)
	}

	if !arrEqual(ignores, expected) {
		t.Errorf("Incorrect ignores: %v != %v", ignores, expected)
	}
}

func TestIgnores(t *testing.T) {
	w, cancel := newConfigWrapper(defaultCfg)
	defer cancel()
	ffs := w.FolderList()[0].Filesystem(nil)
	m := setupModel(t, w)
	defer cleanupModel(m)

	// Assure a clean start state
	must(t, ffs.MkdirAll(config.DefaultMarkerName, 0o644))
	writeFile(t, ffs, ".stignore", []byte(".*\nquux\n"))

	folderIgnoresAlwaysReload(t, m, defaultFolderConfig)

	// Make sure the initial scan has finished (ScanFolders is blocking)
	m.ScanFolders()

	expected := []string{
		".*",
		"quux",
	}

	changeIgnores(t, m, expected)

	_, _, err := m.LoadIgnores("doesnotexist")
	if err == nil {
		t.Error("No error")
	}

	err = m.SetIgnores("doesnotexist", expected)
	if err == nil {
		t.Error("No error")
	}

	// Invalid path, treated like no patterns at all.
	fcfg := config.FolderConfiguration{
		ID: "fresh", Path: "XXX",
		FilesystemType: fs.FilesystemTypeFake,
	}
	ignores := ignore.New(fcfg.Filesystem(nil), ignore.WithCache(m.cfg.Options().CacheIgnoredFiles))
	m.fmut.Lock()
	m.folderCfgs[fcfg.ID] = fcfg
	m.folderIgnores[fcfg.ID] = ignores
	m.fmut.Unlock()

	_, _, err = m.LoadIgnores("fresh")
	if err != nil {
		t.Error("Got error for inexistent folder path")
	}

	// Repeat tests with paused folder
	pausedDefaultFolderConfig := defaultFolderConfig
	pausedDefaultFolderConfig.Paused = true

	m.restartFolder(defaultFolderConfig, pausedDefaultFolderConfig, false)
	// Here folder initialization is not an issue as a paused folder isn't
	// added to the model and thus there is no initial scan happening.

	changeIgnores(t, m, expected)

	// Make sure no .stignore file is considered valid
	defer func() {
		must(t, ffs.Rename(".stignore.bak", ".stignore"))
	}()
	must(t, ffs.Rename(".stignore", ".stignore.bak"))
	changeIgnores(t, m, []string{})
}

func TestEmptyIgnores(t *testing.T) {
	w, cancel := newConfigWrapper(defaultCfg)
	defer cancel()
	ffs := w.FolderList()[0].Filesystem(nil)
	m := setupModel(t, w)
	defer cleanupModel(m)

	if err := m.SetIgnores("default", []string{}); err != nil {
		t.Error(err)
	}
	if _, err := ffs.Stat(".stignore"); err == nil {
		t.Error(".stignore was created despite being empty")
	}

	if err := m.SetIgnores("default", []string{".*", "quux"}); err != nil {
		t.Error(err)
	}
	if _, err := ffs.Stat(".stignore"); os.IsNotExist(err) {
		t.Error(".stignore does not exist")
	}

	if err := m.SetIgnores("default", []string{}); err != nil {
		t.Error(err)
	}
	if _, err := ffs.Stat(".stignore"); err == nil {
		t.Error(".stignore should have been deleted because it is empty")
	}
}

func waitForState(t *testing.T, sub events.Subscription, folder, expected string) {
	t.Helper()
	timeout := time.After(5 * time.Second)
	var err string
	for {
		select {
		case ev := <-sub.C():
			data := ev.Data.(map[string]interface{})
			if data["folder"].(string) == folder {
				if data["error"] == nil {
					err = ""
				} else {
					err = data["error"].(string)
				}
				if err == expected {
					return
				} else {
					t.Error(ev)
				}
			}
		case <-timeout:
			t.Fatalf("Timed out waiting for status: %s, current status: %v", expected, err)
		}
	}
}

func TestROScanRecovery(t *testing.T) {
	fcfg := config.FolderConfiguration{
		FilesystemType:  fs.FilesystemTypeFake,
		ID:              "default",
		Path:            srand.String(32),
		Type:            config.FolderTypeSendOnly,
		RescanIntervalS: 1,
		MarkerName:      config.DefaultMarkerName,
	}
	cfg, cancel := newConfigWrapper(config.Configuration{
		Version: config.CurrentVersion,
		Folders: []config.FolderConfiguration{fcfg},
		Devices: []config.DeviceConfiguration{
			{
				DeviceID: device1,
			},
		},
	})
	defer cancel()
	m := newModel(t, cfg, myID, "syncthing", "dev", nil)

	set := newFileSet(t, "default", m.db)
	set.Update(protocol.LocalDeviceID, []protocol.FileInfo{
		{Name: "dummyfile", Version: protocol.Vector{Counters: []protocol.Counter{{ID: 42, Value: 1}}}},
	})

	ffs := fcfg.Filesystem(nil)

	// Remove marker to generate an error
	ffs.Remove(fcfg.MarkerName)

	sub := m.evLogger.Subscribe(events.StateChanged)
	defer sub.Unsubscribe()
	m.ServeBackground()
	defer cleanupModel(m)

	waitForState(t, sub, "default", config.ErrMarkerMissing.Error())

	fd, err := ffs.Create(config.DefaultMarkerName)
	if err != nil {
		t.Fatal(err)
	}
	fd.Close()

	waitForState(t, sub, "default", "")
}

func TestRWScanRecovery(t *testing.T) {
	fcfg := config.FolderConfiguration{
		FilesystemType:  fs.FilesystemTypeFake,
		ID:              "default",
		Path:            srand.String(32),
		Type:            config.FolderTypeSendReceive,
		RescanIntervalS: 1,
		MarkerName:      config.DefaultMarkerName,
	}
	cfg, cancel := newConfigWrapper(config.Configuration{
		Version: config.CurrentVersion,
		Folders: []config.FolderConfiguration{fcfg},
		Devices: []config.DeviceConfiguration{
			{
				DeviceID: device1,
			},
		},
	})
	defer cancel()
	m := newModel(t, cfg, myID, "syncthing", "dev", nil)

	set := newFileSet(t, "default", m.db)
	set.Update(protocol.LocalDeviceID, []protocol.FileInfo{
		{Name: "dummyfile", Version: protocol.Vector{Counters: []protocol.Counter{{ID: 42, Value: 1}}}},
	})

	ffs := fcfg.Filesystem(nil)

	// Generate error
	if err := ffs.Remove(config.DefaultMarkerName); err != nil {
		t.Fatal(err)
	}

	sub := m.evLogger.Subscribe(events.StateChanged)
	defer sub.Unsubscribe()
	m.ServeBackground()
	defer cleanupModel(m)

	waitForState(t, sub, "default", config.ErrMarkerMissing.Error())

	fd, err := ffs.Create(config.DefaultMarkerName)
	if err != nil {
		t.Error(err)
	}
	fd.Close()

	waitForState(t, sub, "default", "")
}

func TestGlobalDirectoryTree(t *testing.T) {
	m, conn, fcfg, wCancel := setupModelWithConnection(t)
	defer wCancel()
	defer cleanupModelAndRemoveDir(m, fcfg.Filesystem(nil).URI())

	b := func(isfile bool, path ...string) protocol.FileInfo {
		typ := protocol.FileInfoTypeDirectory
		var blocks []protocol.BlockInfo

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
	f := func(name string) *TreeEntry {
		return &TreeEntry{
			Name:    name,
			ModTime: time.Unix(0x666, 0),
			Size:    0xa,
			Type:    protocol.FileInfoTypeFile,
		}
	}
	d := func(name string, entries ...*TreeEntry) *TreeEntry {
		return &TreeEntry{
			Name:     name,
			ModTime:  time.Unix(0x666, 0),
			Size:     128,
			Type:     protocol.FileInfoTypeDirectory,
			Children: entries,
		}
	}

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

		b(true, "zzrootfile"),
	}
	expectedResult := []*TreeEntry{
		d("another",
			d("directory",
				f("afile"),
				d("with",
					d("a",
						f("file"),
					),
					f("file"),
				),
			),
			f("file"),
		),
		d("other",
			d("rand"),
			d("random",
				d("dir"),
				d("dirx"),
			),
			d("randomx"),
		),
		d("some",
			d("directory",
				d("with",
					d("a",
						f("file"),
					),
				),
			),
		),
		f("zzrootfile"),
	}

	mm := func(data interface{}) string {
		bytes, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			panic(err)
		}
		return string(bytes)
	}

	must(t, m.Index(conn, "default", testdata))

	result, _ := m.GlobalDirectoryTree("default", "", -1, false)

	if mm(result) != mm(expectedResult) {
		t.Errorf("Does not match:\n%s\n============\n%s", mm(result), mm(expectedResult))
	}

	result, _ = m.GlobalDirectoryTree("default", "another", -1, false)

	if mm(result) != mm(findByName(expectedResult, "another").Children) {
		t.Errorf("Does not match:\n%s\n============\n%s", mm(result), mm(findByName(expectedResult, "another").Children))
	}

	result, _ = m.GlobalDirectoryTree("default", "", 0, false)
	currentResult := []*TreeEntry{
		d("another"),
		d("other"),
		d("some"),
		f("zzrootfile"),
	}

	if mm(result) != mm(currentResult) {
		t.Errorf("Does not match:\n%s\n============\n%s", mm(result), mm(currentResult))
	}

	result, _ = m.GlobalDirectoryTree("default", "", 1, false)
	currentResult = []*TreeEntry{
		d("another",
			d("directory"),
			f("file"),
		),
		d("other",
			d("rand"),
			d("random"),
			d("randomx"),
		),
		d("some",
			d("directory"),
		),
		f("zzrootfile"),
	}

	if mm(result) != mm(currentResult) {
		t.Errorf("Does not match:\n%s\n%s", mm(result), mm(currentResult))
	}

	result, _ = m.GlobalDirectoryTree("default", "", -1, true)
	currentResult = []*TreeEntry{
		d("another",
			d("directory",
				d("with",
					d("a"),
				),
			),
		),
		d("other",
			d("rand"),
			d("random",
				d("dir"),
				d("dirx"),
			),
			d("randomx"),
		),
		d("some",
			d("directory",
				d("with",
					d("a"),
				),
			),
		),
	}

	if mm(result) != mm(currentResult) {
		t.Errorf("Does not match:\n%s\n%s", mm(result), mm(currentResult))
	}

	result, _ = m.GlobalDirectoryTree("default", "", 1, true)
	currentResult = []*TreeEntry{
		d("another",
			d("directory"),
		),
		d("other",
			d("rand"),
			d("random"),
			d("randomx"),
		),
		d("some",
			d("directory"),
		),
	}

	if mm(result) != mm(currentResult) {
		t.Errorf("Does not match:\n%s\n%s", mm(result), mm(currentResult))
	}

	result, _ = m.GlobalDirectoryTree("default", "another", 0, false)
	currentResult = []*TreeEntry{
		d("directory"),
		f("file"),
	}

	if mm(result) != mm(currentResult) {
		t.Errorf("Does not match:\n%s\n%s", mm(result), mm(currentResult))
	}

	result, _ = m.GlobalDirectoryTree("default", "some/directory", 0, false)
	currentResult = []*TreeEntry{
		d("with"),
	}

	if mm(result) != mm(currentResult) {
		t.Errorf("Does not match:\n%s\n%s", mm(result), mm(currentResult))
	}

	result, _ = m.GlobalDirectoryTree("default", "some/directory", 1, false)
	currentResult = []*TreeEntry{
		d("with",
			d("a"),
		),
	}

	if mm(result) != mm(currentResult) {
		t.Errorf("Does not match:\n%s\n%s", mm(result), mm(currentResult))
	}

	result, _ = m.GlobalDirectoryTree("default", "some/directory", 2, false)
	currentResult = []*TreeEntry{
		d("with",
			d("a",
				f("file"),
			),
		),
	}

	if mm(result) != mm(currentResult) {
		t.Errorf("Does not match:\n%s\n%s", mm(result), mm(currentResult))
	}

	result, _ = m.GlobalDirectoryTree("default", "another", -1, true)
	currentResult = []*TreeEntry{
		d("directory",
			d("with",
				d("a"),
			),
		),
	}

	if mm(result) != mm(currentResult) {
		t.Errorf("Does not match:\n%s\n%s", mm(result), mm(currentResult))
	}

	// No prefix matching!
	result, _ = m.GlobalDirectoryTree("default", "som", -1, false)
	currentResult = []*TreeEntry{}

	if mm(result) != mm(currentResult) {
		t.Errorf("Does not match:\n%s\n%s", mm(result), mm(currentResult))
	}
}

func genDeepFiles(n, d int) []protocol.FileInfo {
	mrand.Seed(int64(n))
	files := make([]protocol.FileInfo, n)
	t := time.Now().Unix()
	for i := 0; i < n; i++ {
		path := ""
		for i := 0; i <= d; i++ {
			path = filepath.Join(path, strconv.Itoa(mrand.Int()))
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
	m, _, fcfg, wcfgCancel := setupModelWithConnection(b)
	defer wcfgCancel()
	defer cleanupModelAndRemoveDir(m, fcfg.Filesystem(nil).URI())

	m.ScanFolder(fcfg.ID)
	files := genDeepFiles(n1, n2)

	must(b, m.Index(device1Conn, fcfg.ID, files))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.GlobalDirectoryTree(fcfg.ID, "", -1, false)
	}
	b.ReportAllocs()
}

func TestIssue3028(t *testing.T) {
	w, cancel := newConfigWrapper(defaultCfg)
	defer cancel()
	ffs := w.FolderList()[0].Filesystem(nil)
	m := setupModel(t, w)
	defer cleanupModel(m)

	// Create two files that we'll delete, one with a name that is a prefix of the other.

	writeFile(t, ffs, "testrm", []byte("Hello"))
	writeFile(t, ffs, "testrm2", []byte("Hello"))

	// Scan, and get a count of how many files are there now

	m.ScanFolderSubdirs("default", []string{"testrm", "testrm2"})
	locorigfiles := localSize(t, m, "default").Files
	globorigfiles := globalSize(t, m, "default").Files

	// Delete

	must(t, ffs.Remove("testrm"))
	must(t, ffs.Remove("testrm2"))

	// Verify that the number of files decreased by two and the number of
	// deleted files increases by two

	m.ScanFolderSubdirs("default", []string{"testrm", "testrm2"})
	loc := localSize(t, m, "default")
	glob := globalSize(t, m, "default")

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
	cfg := defaultCfgWrapper.RawCopy()
	// Create a separate wrapper not to pollute other tests.
	wrapper, cancel := newConfigWrapper(config.Configuration{Version: config.CurrentVersion})
	defer cancel()
	m := newModel(t, wrapper, myID, "syncthing", "dev", nil)
	m.ServeBackground()
	defer cleanupModel(m)

	// Force the model to wire itself and add the folders
	replace(t, wrapper, cfg)

	if _, ok := m.folderCfgs["default"]; !ok {
		t.Error("Folder should be running")
	}

	newCfg := wrapper.RawCopy()
	newCfg.Folders[0].Paused = true

	replace(t, wrapper, newCfg)

	if _, ok := m.folderCfgs["default"]; ok {
		t.Error("Folder should not be running")
	}

	if _, ok := m.cfg.Folder("default"); !ok {
		t.Error("should still have folder in config")
	}

	replace(t, wrapper, config.Configuration{Version: config.CurrentVersion})

	if _, ok := m.cfg.Folder("default"); ok {
		t.Error("should not have folder in config")
	}

	// Add the folder back, should be running
	replace(t, wrapper, cfg)

	if _, ok := m.folderCfgs["default"]; !ok {
		t.Error("Folder should be running")
	}
	if _, ok := m.cfg.Folder("default"); !ok {
		t.Error("should still have folder in config")
	}

	// Should not panic when removing a running folder.
	replace(t, wrapper, config.Configuration{Version: config.CurrentVersion})

	if _, ok := m.folderCfgs["default"]; ok {
		t.Error("Folder should not be running")
	}
	if _, ok := m.cfg.Folder("default"); ok {
		t.Error("should not have folder in config")
	}
}

func TestIndexesForUnknownDevicesDropped(t *testing.T) {
	m := newModel(t, defaultCfgWrapper, myID, "syncthing", "dev", nil)

	files := newFileSet(t, "default", m.db)
	files.Drop(device1)
	files.Update(device1, genFiles(1))
	files.Drop(device2)
	files.Update(device2, genFiles(1))

	if len(files.ListDevices()) != 2 {
		t.Error("expected two devices")
	}

	m.newFolder(defaultFolderConfig, false)
	defer cleanupModel(m)

	// Remote sequence is cached, hence need to recreated.
	files = newFileSet(t, "default", m.db)

	if l := len(files.ListDevices()); l != 1 {
		t.Errorf("Expected one device got %v", l)
	}
}

func TestSharedWithClearedOnDisconnect(t *testing.T) {
	wcfg, cancel := newConfigWrapper(defaultCfg)
	defer cancel()
	addDevice2(t, wcfg, wcfg.FolderList()[0])

	m := setupModel(t, wcfg)
	defer cleanupModel(m)

	conn1 := newFakeConnection(device1, m)
	m.AddConnection(conn1, protocol.Hello{})
	conn2 := newFakeConnection(device2, m)
	m.AddConnection(conn2, protocol.Hello{})

	m.ClusterConfig(conn1, protocol.ClusterConfig{
		Folders: []protocol.Folder{
			{
				ID: "default",
				Devices: []protocol.Device{
					{ID: myID},
					{ID: device1},
					{ID: device2},
				},
			},
		},
	})
	m.ClusterConfig(conn2, protocol.ClusterConfig{
		Folders: []protocol.Folder{
			{
				ID: "default",
				Devices: []protocol.Device{
					{ID: myID},
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

	select {
	case <-conn2.Closed():
		t.Error("conn already closed")
	default:
	}

	if _, err := wcfg.RemoveDevice(device2); err != nil {
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

	select {
	case <-conn2.Closed():
	default:
		t.Error("connection not closed")
	}

	if _, ok := wcfg.Devices()[device2]; ok {
		t.Error("device still in config")
	}

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

func TestIssue3804(t *testing.T) {
	m := setupModel(t, defaultCfgWrapper)
	defer cleanupModel(m)

	// Subdirs ending in slash should be accepted

	if err := m.ScanFolderSubdirs("default", []string{"baz/", "foo"}); err != nil {
		t.Error("Unexpected error:", err)
	}
}

func TestIssue3829(t *testing.T) {
	m := setupModel(t, defaultCfgWrapper)
	defer cleanupModel(m)

	// Empty subdirs should be accepted

	if err := m.ScanFolderSubdirs("default", []string{""}); err != nil {
		t.Error("Unexpected error:", err)
	}
}

// TestIssue4573 tests that contents of an unavailable dir aren't marked deleted
func TestIssue4573(t *testing.T) {
	w, fcfg, wCancel := newDefaultCfgWrapper()
	defer wCancel()
	testFs := fcfg.Filesystem(nil)
	defer os.RemoveAll(testFs.URI())

	must(t, testFs.MkdirAll("inaccessible", 0o755))
	defer testFs.Chmod("inaccessible", 0o777)

	file := filepath.Join("inaccessible", "a")
	fd, err := testFs.Create(file)
	must(t, err)
	fd.Close()

	m := setupModel(t, w)
	defer cleanupModel(m)

	must(t, testFs.Chmod("inaccessible", 0o000))

	m.ScanFolder("default")

	if file, ok := m.testCurrentFolderFile("default", file); !ok {
		t.Fatalf("File missing in db")
	} else if file.Deleted {
		t.Errorf("Inaccessible file has been marked as deleted.")
	}
}

// TestInternalScan checks whether various fs operations are correctly represented
// in the db after scanning.
func TestInternalScan(t *testing.T) {
	w, fcfg, wCancel := newDefaultCfgWrapper()
	defer wCancel()
	testFs := fcfg.Filesystem(nil)
	defer os.RemoveAll(testFs.URI())

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
			if err := testFs.MkdirAll(dir, 0o775); err != nil {
				t.Fatalf("%v: %v", dir, err)
			}
		}
		testCases[sub] = func(f protocol.FileInfo) bool {
			return !f.Deleted
		}
		for _, dir := range []string{dir, sub} {
			file := filepath.Join(dir, "a")
			fd, err := testFs.Create(file)
			must(t, err)
			fd.Close()
			testCases[file] = func(f protocol.FileInfo) bool {
				return !f.Deleted
			}
		}
	}

	m := setupModel(t, w)
	defer cleanupModel(m)

	for _, dir := range baseDirs {
		must(t, testFs.RemoveAll(dir))
	}

	fd, err := testFs.Create("dirToFile")
	must(t, err)
	fd.Close()

	m.ScanFolder("default")

	for path, cond := range testCases {
		if f, ok := m.testCurrentFolderFile("default", path); !ok {
			t.Fatalf("%v missing in db", path)
		} else if cond(f) {
			t.Errorf("Incorrect db entry for %v", path)
		}
	}
}

func TestCustomMarkerName(t *testing.T) {
	fcfg := newFolderConfig()
	fcfg.ID = "default"
	fcfg.RescanIntervalS = 1
	fcfg.MarkerName = "myfile"
	cfg, cancel := newConfigWrapper(config.Configuration{
		Version: config.CurrentVersion,
		Folders: []config.FolderConfiguration{fcfg},
		Devices: []config.DeviceConfiguration{
			{
				DeviceID: device1,
			},
		},
	})
	defer cancel()

	ffs := fcfg.Filesystem(nil)

	m := newModel(t, cfg, myID, "syncthing", "dev", nil)

	set := newFileSet(t, "default", m.db)
	set.Update(protocol.LocalDeviceID, []protocol.FileInfo{
		{Name: "dummyfile"},
	})

	if err := ffs.Remove(config.DefaultMarkerName); err != nil {
		t.Fatal(err)
	}

	sub := m.evLogger.Subscribe(events.StateChanged)
	defer sub.Unsubscribe()
	m.ServeBackground()
	defer cleanupModel(m)

	waitForState(t, sub, "default", config.ErrMarkerMissing.Error())

	fd, _ := ffs.Create("myfile")
	fd.Close()

	waitForState(t, sub, "default", "")
}

func TestRemoveDirWithContent(t *testing.T) {
	m, conn, fcfg, wcfgCancel := setupModelWithConnection(t)
	defer wcfgCancel()
	tfs := fcfg.Filesystem(nil)
	defer cleanupModelAndRemoveDir(m, tfs.URI())

	tfs.MkdirAll("dirwith", 0o755)
	content := filepath.Join("dirwith", "content")
	fd, err := tfs.Create(content)
	must(t, err)
	fd.Close()

	must(t, m.ScanFolder(fcfg.ID))

	dir, ok := m.testCurrentFolderFile(fcfg.ID, "dirwith")
	if !ok {
		t.Fatalf("Can't get dir \"dirwith\" after initial scan")
	}
	dir.Deleted = true
	dir.Version = dir.Version.Update(device1.Short()).Update(device1.Short())

	file, ok := m.testCurrentFolderFile(fcfg.ID, content)
	if !ok {
		t.Fatalf("Can't get file \"%v\" after initial scan", content)
	}
	file.Deleted = true
	file.Version = file.Version.Update(device1.Short()).Update(device1.Short())

	must(t, m.IndexUpdate(conn, fcfg.ID, []protocol.FileInfo{dir, file}))

	// Is there something we could trigger on instead of just waiting?
	timeout := time.NewTimer(5 * time.Second)
	for {
		dir, ok := m.testCurrentFolderFile(fcfg.ID, "dirwith")
		if !ok {
			t.Fatalf("Can't get dir \"dirwith\" after index update")
		}
		file, ok := m.testCurrentFolderFile(fcfg.ID, content)
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
	m, conn, fcfg, wcfgCancel := setupModelWithConnection(t)
	defer wcfgCancel()
	defer cleanupModel(m)
	testFs := fcfg.Filesystem(nil)

	// Scenario: Dir is deleted locally and before syncing/index exchange
	// happens, a file is create in that dir on the remote.
	// This should result in the directory being recreated and added to the
	// db locally.

	must(t, testFs.MkdirAll("delDir", 0o755))

	m.ScanFolder("default")

	if fcfg, ok := m.cfg.Folder("default"); !ok || !fcfg.SharedWith(device1) {
		t.Fatal("not shared with device1")
	}

	fileName := filepath.Join("delDir", "file")
	conn.addFile(fileName, 0o644, protocol.FileInfoTypeFile, nil)
	conn.sendIndexUpdate()

	// Is there something we could trigger on instead of just waiting?
	timeout := time.NewTimer(5 * time.Second)
	created := false
	for {
		if !created {
			if _, ok := m.testCurrentFolderFile("default", fileName); ok {
				created = true
			}
		} else {
			dir, ok := m.testCurrentFolderFile("default", "delDir")
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
	t.Skip("incompatible with fakefs")

	// We create a bunch of files which we restore
	// In each file, we write the filename as the content
	// We verify that the content matches at the expected filenames
	// after the restore operation.

	fcfg := newFolderConfiguration(defaultCfgWrapper, "default", "default", fs.FilesystemTypeFake, srand.String(32))
	fcfg.Versioning.Type = "simple"
	fcfg.FSWatcherEnabled = false
	filesystem := fcfg.Filesystem(nil)

	rawConfig := config.Configuration{
		Version: config.CurrentVersion,
		Folders: []config.FolderConfiguration{fcfg},
	}
	cfg, cancel := newConfigWrapper(rawConfig)
	defer cancel()

	m := setupModel(t, cfg)
	defer cleanupModel(m)
	m.ScanFolder("default")

	sentinel, err := time.ParseInLocation(versioner.TimeFormat, "20180101-010101", time.Local)
	if err != nil {
		t.Fatal(err)
	}

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
		".stversions/dir/cat",                                // untagged which was used by trashcan, supported

		// "file.txt" will be restored
		"existing",
		"something/file", // Becomes directory
		"dir/file.txt",
		"dir/existing.txt",
	} {
		if build.IsWindows {
			file = filepath.FromSlash(file)
		}
		dir := filepath.Dir(file)
		must(t, filesystem.MkdirAll(dir, 0o755))
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
	must(t, err)
	expectedVersions := map[string]int{
		"file.txt":               1,
		"existing":               1,
		"something":              1,
		"dir/file.txt":           3,
		"dir/existing.txt":       1,
		"very/very/deep/one.txt": 1,
		"dir/cat":                1,
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
		tm, err := time.ParseInLocation(versioner.TimeFormat, s, time.Local)
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

	beforeRestore := time.Now().Truncate(time.Second)

	ferr, err := m.RestoreFolderVersions("default", restore)
	must(t, err)

	if err, ok := ferr["something"]; len(ferr) > 1 || !ok || !errors.Is(err, versioner.ErrDirectory) {
		t.Fatalf("incorrect error or count: %d %s", len(ferr), ferr)
	}

	// Failed items are not expected to be restored.
	// Remove them from expectations
	for name := range ferr {
		delete(restore, name)
	}

	// Check that content of files matches to the version they've been restored.
	for file, version := range restore {
		if build.IsWindows {
			file = filepath.FromSlash(file)
		}
		tag := version.In(time.Local).Truncate(time.Second).Format(versioner.TimeFormat)
		taggedName := filepath.Join(versioner.DefaultPath, versioner.TagFilename(file, tag))
		fd, err := filesystem.Open(file)
		if err != nil {
			t.Error(err)
		}
		defer fd.Close()

		content, err := io.ReadAll(fd)
		if err != nil {
			t.Error(err)
		}
		if !bytes.Equal(content, []byte(taggedName)) {
			t.Errorf("%s: %s != %s", file, string(content), taggedName)
		}
	}

	// Simple versioner uses now for timestamp generation, so we can check
	// if existing stuff was correctly archived as we restored (oppose to deleteD), and version time as after beforeRestore
	expectArchived := map[string]struct{}{
		"existing":         {},
		"dir/file.txt":     {},
		"dir/existing.txt": {},
	}

	allFileVersions, err := m.GetFolderVersions("default")
	must(t, err)
	for file, versions := range allFileVersions {
		key := file
		if build.IsWindows {
			file = filepath.FromSlash(file)
		}
		for _, version := range versions {
			if version.VersionTime.Equal(beforeRestore) || version.VersionTime.After(beforeRestore) {
				fd, err := filesystem.Open(versioner.DefaultPath + "/" + versioner.TagFilename(file, version.VersionTime.Format(versioner.TimeFormat)))
				must(t, err)
				defer fd.Close()

				content, err := io.ReadAll(fd)
				if err != nil {
					t.Error(err)
				}
				// Even if they are at the archived path, content should have the non
				// archived name.
				if !bytes.Equal(content, []byte(file)) {
					t.Errorf("%s (%s): %s != %s", file, fd.Name(), string(content), file)
				}
				_, ok := expectArchived[key]
				if !ok {
					t.Error("unexpected archived file with future timestamp", file, version.VersionTime)
				}
				delete(expectArchived, key)
			}
		}
	}

	if len(expectArchived) != 0 {
		t.Fatal("missed some archived files", expectArchived)
	}
}

func TestPausedFolders(t *testing.T) {
	// Create a separate wrapper not to pollute other tests.
	wrapper, cancel := newConfigWrapper(defaultCfgWrapper.RawCopy())
	defer cancel()
	m := setupModel(t, wrapper)
	defer cleanupModel(m)

	if err := m.ScanFolder("default"); err != nil {
		t.Error(err)
	}

	pausedConfig := wrapper.RawCopy()
	pausedConfig.Folders[0].Paused = true
	replace(t, wrapper, pausedConfig)

	if err := m.ScanFolder("default"); err != ErrFolderPaused {
		t.Errorf("Expected folder paused error, received: %v", err)
	}

	if err := m.ScanFolder("nonexistent"); err != ErrFolderMissing {
		t.Errorf("Expected missing folder error, received: %v", err)
	}
}

func TestIssue4094(t *testing.T) {
	// Create a separate wrapper not to pollute other tests.
	wrapper, cancel := newConfigWrapper(config.Configuration{Version: config.CurrentVersion})
	defer cancel()
	m := newModel(t, wrapper, myID, "syncthing", "dev", nil)
	m.ServeBackground()
	defer cleanupModel(m)

	// Force the model to wire itself and add the folders
	folderPath := "nonexistent"
	cfg := defaultCfgWrapper.RawCopy()
	fcfg := config.FolderConfiguration{
		FilesystemType: fs.FilesystemTypeFake,
		ID:             "folder1",
		Path:           folderPath,
		Paused:         true,
		Devices: []config.FolderDeviceConfiguration{
			{DeviceID: device1},
		},
	}
	cfg.Folders = []config.FolderConfiguration{fcfg}
	replace(t, wrapper, cfg)

	if err := m.SetIgnores(fcfg.ID, []string{"foo"}); err != nil {
		t.Fatalf("failed setting ignores: %v", err)
	}

	if _, err := fcfg.Filesystem(nil).Lstat(".stignore"); err != nil {
		t.Fatalf("failed stating .stignore: %v", err)
	}
}

func TestIssue4903(t *testing.T) {
	wrapper, cancel := newConfigWrapper(config.Configuration{Version: config.CurrentVersion})
	defer cancel()
	m := setupModel(t, wrapper)
	defer cleanupModel(m)

	// Force the model to wire itself and add the folders
	folderPath := "nonexistent"
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
	replace(t, wrapper, cfg)

	if err := fcfg.CheckPath(); err != config.ErrPathMissing {
		t.Fatalf("expected path missing error, got: %v, debug: %s", err, fcfg.CheckPath())
	}

	if _, err := fcfg.Filesystem(nil).Lstat("."); !fs.IsNotExist(err) {
		t.Fatalf("Expected missing path error, got: %v", err)
	}
}

func TestIssue5002(t *testing.T) {
	// recheckFile should not panic when given an index equal to the number of blocks

	w, fcfg, wCancel := newDefaultCfgWrapper()
	defer wCancel()
	ffs := fcfg.Filesystem(nil)

	fd, err := ffs.Create("foo")
	must(t, err)
	_, err = fd.Write([]byte("foobar"))
	must(t, err)
	fd.Close()

	m := setupModel(t, w)
	defer cleanupModel(m)

	if err := m.ScanFolder("default"); err != nil {
		t.Error(err)
	}

	file, ok := m.testCurrentFolderFile("default", "foo")
	if !ok {
		t.Fatal("test file should exist")
	}
	blockSize := int32(file.BlockSize())

	m.recheckFile(protocol.LocalDeviceID, "default", "foo", file.Size-int64(blockSize), []byte{1, 2, 3, 4}, 0)
	m.recheckFile(protocol.LocalDeviceID, "default", "foo", file.Size, []byte{1, 2, 3, 4}, 0) // panic
	m.recheckFile(protocol.LocalDeviceID, "default", "foo", file.Size+int64(blockSize), []byte{1, 2, 3, 4}, 0)
}

func TestParentOfUnignored(t *testing.T) {
	w, fcfg, wCancel := newDefaultCfgWrapper()
	defer wCancel()
	ffs := fcfg.Filesystem(nil)

	must(t, ffs.Mkdir("bar", 0o755))
	must(t, ffs.Mkdir("baz", 0o755))
	must(t, ffs.Mkdir("baz/quux", 0o755))

	m := setupModel(t, w)
	defer cleanupModel(m)

	m.SetIgnores("default", []string{"!quux", "*"})
	m.ScanFolder("default")

	if bar, ok := m.testCurrentFolderFile("default", "bar"); !ok {
		t.Error(`Directory "bar" missing in db`)
	} else if !bar.IsIgnored() {
		t.Error(`Directory "bar" is not ignored`)
	}

	if baz, ok := m.testCurrentFolderFile("default", "baz"); !ok {
		t.Error(`Directory "baz" missing in db`)
	} else if baz.IsIgnored() {
		t.Error(`Directory "baz" is ignored`)
	}
}

// TestFolderRestartZombies reproduces issue 5233, where multiple concurrent folder
// restarts would leave more than one folder runner alive.
func TestFolderRestartZombies(t *testing.T) {
	wrapper, cancel := newConfigWrapper(defaultCfg.Copy())
	defer cancel()

	waiter, err := wrapper.Modify(func(cfg *config.Configuration) {
		cfg.Options.RawMaxFolderConcurrency = -1
		_, i, _ := cfg.Folder("default")
		cfg.Folders[i].FilesystemType = fs.FilesystemTypeFake
	})
	must(t, err)
	waiter.Wait()
	folderCfg, _ := wrapper.Folder("default")

	m := setupModel(t, wrapper)
	defer cleanupModel(m)

	// Make sure the folder is up and running, because we want to count it.
	m.ScanFolder("default")

	// Check how many running folders we have running before the test.
	if r := m.foldersRunning.Load(); r != 1 {
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
				fcfg := folderCfg.Copy()
				fcfg.MaxConflicts = mrand.Int() // safe change that should cause a folder restart
				setFolder(t, wrapper, fcfg)
			}
		}()
	}

	// Wait for the above to complete and check how many folders we have
	// running now. It should not have increased.
	wg.Wait()
	// Make sure the folder is up and running, because we want to count it.
	m.ScanFolder("default")
	if r := m.foldersRunning.Load(); r != 1 {
		t.Error("Expected one running folder, not", r)
	}
}

func TestRequestLimit(t *testing.T) {
	wrapper, fcfg, cancel := newDefaultCfgWrapper()
	ffs := fcfg.Filesystem(nil)

	file := "tmpfile"
	fd, err := ffs.Create(file)
	must(t, err)
	fd.Close()

	defer cancel()
	waiter, err := wrapper.Modify(func(cfg *config.Configuration) {
		_, i, _ := cfg.Device(device1)
		cfg.Devices[i].MaxRequestKiB = 1
	})
	must(t, err)
	waiter.Wait()
	m, conn := setupModelWithConnectionFromWrapper(t, wrapper)
	defer cleanupModel(m)
	m.ScanFolder("default")

	befReq := time.Now()
	first, err := m.Request(conn, "default", file, 0, 2000, 0, nil, 0, false)
	if err != nil {
		t.Fatalf("First request failed: %v", err)
	}
	reqDur := time.Since(befReq)
	returned := make(chan struct{})
	go func() {
		second, err := m.Request(conn, "default", file, 0, 2000, 0, nil, 0, false)
		if err != nil {
			t.Errorf("Second request failed: %v", err)
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

// TestConnCloseOnRestart checks that there is no deadlock when calling Close
// on a protocol connection that has a blocking reader (blocking writer can't
// be done as the test requires clusterconfigs to go through).
func TestConnCloseOnRestart(t *testing.T) {
	oldCloseTimeout := protocol.CloseTimeout
	protocol.CloseTimeout = 100 * time.Millisecond
	defer func() {
		protocol.CloseTimeout = oldCloseTimeout
	}()

	w, fcfg, wCancel := newDefaultCfgWrapper()
	defer wCancel()
	m := setupModel(t, w)
	defer cleanupModelAndRemoveDir(m, fcfg.Filesystem(nil).URI())

	br := &testutil.BlockingRW{}
	nw := &testutil.NoopRW{}
	ci := &protocolmocks.ConnectionInfo{}
	m.AddConnection(protocol.NewConnection(device1, br, nw, testutil.NoopCloser{}, m, ci, protocol.CompressionNever, nil, m.keyGen), protocol.Hello{})
	m.pmut.RLock()
	if len(m.closed) != 1 {
		t.Fatalf("Expected just one conn (len(m.closed) == %v)", len(m.closed))
	}
	var closed chan struct{}
	for _, c := range m.closed {
		closed = c
	}
	m.pmut.RUnlock()

	waiter, err := w.RemoveDevice(device1)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		waiter.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Timed out before config took effect")
	}
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("Timed out before connection was closed")
	}
}

func TestModTimeWindow(t *testing.T) {
	w, fcfg, wCancel := newDefaultCfgWrapper()
	defer wCancel()
	tfs := modtimeTruncatingFS{
		trunc:      0,
		Filesystem: fcfg.Filesystem(nil),
	}
	// fcfg.RawModTimeWindowS = 2
	setFolder(t, w, fcfg)
	m := setupModel(t, w)
	defer cleanupModelAndRemoveDir(m, tfs.URI())

	name := "foo"

	fd, err := tfs.Create(name)
	must(t, err)
	stat, err := fd.Stat()
	must(t, err)
	modTime := stat.ModTime()
	fd.Close()

	m.ScanFolders()

	// Get current version

	fi, ok := m.testCurrentFolderFile("default", name)
	if !ok {
		t.Fatal("File missing")
	}
	v := fi.Version

	// Change the filesystem to only return modtimes to the closest two
	// seconds, like FAT.

	tfs.trunc = 2 * time.Second

	// Scan again

	m.ScanFolders()

	// No change due to within window

	fi, _ = m.testCurrentFolderFile("default", name)
	if !fi.Version.Equal(v) {
		t.Fatalf("Got version %v, expected %v", fi.Version, v)
	}

	// Update to be outside window

	err = tfs.Chtimes(name, time.Now(), modTime.Add(2*time.Second))
	must(t, err)

	m.ScanFolders()

	// Version should have updated

	fi, _ = m.testCurrentFolderFile("default", name)
	if fi.Version.Compare(v) != protocol.Greater {
		t.Fatalf("Got result %v, expected %v", fi.Version.Compare(v), protocol.Greater)
	}
}

func TestDevicePause(t *testing.T) {
	m, _, fcfg, wcfgCancel := setupModelWithConnection(t)
	defer wcfgCancel()
	defer cleanupModelAndRemoveDir(m, fcfg.Filesystem(nil).URI())

	sub := m.evLogger.Subscribe(events.DevicePaused)
	defer sub.Unsubscribe()

	m.pmut.RLock()
	var closed chan struct{}
	for _, c := range m.closed {
		closed = c
	}
	m.pmut.RUnlock()

	pauseDevice(t, m.cfg, device1, true)

	timeout := time.NewTimer(5 * time.Second)
	select {
	case <-sub.C():
		select {
		case <-closed:
		case <-timeout.C:
			t.Fatal("Timed out before connection was closed")
		}
	case <-timeout.C:
		t.Fatal("Timed out before device was paused")
	}
}

func TestDeviceWasSeen(t *testing.T) {
	m, _, fcfg, wcfgCancel := setupModelWithConnection(t)
	defer wcfgCancel()
	defer cleanupModelAndRemoveDir(m, fcfg.Filesystem(nil).URI())

	m.deviceWasSeen(device1)

	stats, err := m.DeviceStatistics()
	if err != nil {
		t.Error("Unexpected error:", err)
	}
	entry := stats[device1]
	if time.Since(entry.LastSeen) > time.Second {
		t.Error("device should have been seen now")
	}
}

func TestNewLimitedRequestResponse(t *testing.T) {
	l0 := semaphore.New(0)
	l1 := semaphore.New(1024)
	l2 := (*semaphore.Semaphore)(nil)

	// Should take 500 bytes from any non-unlimited non-nil limiters.
	res := newLimitedRequestResponse(500, l0, l1, l2)

	if l1.Available() != 1024-500 {
		t.Error("should have taken bytes from limited limiter")
	}

	// Closing the result should return the bytes.
	res.Close()

	// Try to take 1024 bytes to make sure the bytes were returned.
	done := make(chan struct{})
	go func() {
		l1.Take(1024)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Error("Bytes weren't returned in a timely fashion")
	}
}

func TestSummaryPausedNoError(t *testing.T) {
	wcfg, fcfg, wcfgCancel := newDefaultCfgWrapper()
	defer wcfgCancel()
	pauseFolder(t, wcfg, fcfg.ID, true)
	m := setupModel(t, wcfg)
	defer cleanupModel(m)

	fss := NewFolderSummaryService(wcfg, m, myID, events.NoopLogger)
	if _, err := fss.Summary(fcfg.ID); err != nil {
		t.Error("Expected no error getting a summary for a paused folder:", err)
	}
}

func TestFolderAPIErrors(t *testing.T) {
	wcfg, fcfg, wcfgCancel := newDefaultCfgWrapper()
	defer wcfgCancel()
	pauseFolder(t, wcfg, fcfg.ID, true)
	m := setupModel(t, wcfg)
	defer cleanupModel(m)

	methods := []func(folder string) error{
		m.ScanFolder,
		func(folder string) error {
			return m.ScanFolderSubdirs(folder, nil)
		},
		func(folder string) error {
			_, err := m.GetFolderVersions(folder)
			return err
		},
		func(folder string) error {
			_, err := m.RestoreFolderVersions(folder, nil)
			return err
		},
	}

	for i, method := range methods {
		if err := method(fcfg.ID); err != ErrFolderPaused {
			t.Errorf(`Expected "%v", got "%v" (method no %v)`, ErrFolderPaused, err, i)
		}
		if err := method("notexisting"); err != ErrFolderMissing {
			t.Errorf(`Expected "%v", got "%v" (method no %v)`, ErrFolderMissing, err, i)
		}
	}
}

func TestRenameSequenceOrder(t *testing.T) {
	wcfg, fcfg, wcfgCancel := newDefaultCfgWrapper()
	defer wcfgCancel()
	m := setupModel(t, wcfg)
	defer cleanupModel(m)

	numFiles := 20

	ffs := fcfg.Filesystem(nil)
	for i := 0; i < numFiles; i++ {
		v := fmt.Sprintf("%d", i)
		writeFile(t, ffs, v, []byte(v))
	}

	m.ScanFolders()

	count := 0
	snap := dbSnapshot(t, m, "default")
	snap.WithHave(protocol.LocalDeviceID, func(i protocol.FileIntf) bool {
		count++
		return true
	})
	snap.Release()

	if count != numFiles {
		t.Errorf("Unexpected count: %d != %d", count, numFiles)
	}

	// Modify all the files, other than the ones we expect to rename
	for i := 0; i < numFiles; i++ {
		if i == 3 || i == 17 || i == 16 || i == 4 {
			continue
		}
		v := fmt.Sprintf("%d", i)
		writeFile(t, ffs, v, []byte(v+"-new"))
	}
	// Rename
	must(t, ffs.Rename("3", "17"))
	must(t, ffs.Rename("16", "4"))

	// Scan
	m.ScanFolders()

	// Verify sequence of a appearing is followed by c disappearing.
	snap = dbSnapshot(t, m, "default")
	defer snap.Release()

	var firstExpectedSequence int64
	var secondExpectedSequence int64
	failed := false
	snap.WithHaveSequence(0, func(i protocol.FileIntf) bool {
		t.Log(i)
		if i.FileName() == "17" {
			firstExpectedSequence = i.SequenceNo() + 1
		}
		if i.FileName() == "4" {
			secondExpectedSequence = i.SequenceNo() + 1
		}
		if i.FileName() == "3" {
			failed = i.SequenceNo() != firstExpectedSequence || failed
		}
		if i.FileName() == "16" {
			failed = i.SequenceNo() != secondExpectedSequence || failed
		}
		return true
	})
	if failed {
		t.Fail()
	}
}

func TestRenameSameFile(t *testing.T) {
	wcfg, fcfg, wcfgCancel := newDefaultCfgWrapper()
	defer wcfgCancel()
	m := setupModel(t, wcfg)
	defer cleanupModel(m)

	ffs := fcfg.Filesystem(nil)
	writeFile(t, ffs, "file", []byte("file"))

	m.ScanFolders()

	count := 0
	snap := dbSnapshot(t, m, "default")
	snap.WithHave(protocol.LocalDeviceID, func(i protocol.FileIntf) bool {
		count++
		return true
	})
	snap.Release()

	if count != 1 {
		t.Errorf("Unexpected count: %d != %d", count, 1)
	}

	must(t, ffs.Rename("file", "file1"))
	must(t, osutil.Copy(fs.CopyRangeMethodStandard, ffs, ffs, "file1", "file0"))
	must(t, osutil.Copy(fs.CopyRangeMethodStandard, ffs, ffs, "file1", "file2"))
	must(t, osutil.Copy(fs.CopyRangeMethodStandard, ffs, ffs, "file1", "file3"))
	must(t, osutil.Copy(fs.CopyRangeMethodStandard, ffs, ffs, "file1", "file4"))

	m.ScanFolders()

	snap = dbSnapshot(t, m, "default")
	defer snap.Release()

	prevSeq := int64(0)
	seen := false
	snap.WithHaveSequence(0, func(i protocol.FileIntf) bool {
		if i.SequenceNo() <= prevSeq {
			t.Fatalf("non-increasing sequences: %d <= %d", i.SequenceNo(), prevSeq)
		}
		if i.FileName() == "file" {
			if seen {
				t.Fatal("already seen file")
			}
			seen = true
		}
		prevSeq = i.SequenceNo()
		return true
	})
}

func TestRenameEmptyFile(t *testing.T) {
	wcfg, fcfg, wcfgCancel := newDefaultCfgWrapper()
	defer wcfgCancel()
	m := setupModel(t, wcfg)
	defer cleanupModel(m)

	ffs := fcfg.Filesystem(nil)

	writeFile(t, ffs, "file", []byte("data"))
	writeFile(t, ffs, "empty", nil)

	m.ScanFolders()

	snap := dbSnapshot(t, m, "default")
	defer snap.Release()
	empty, eok := snap.Get(protocol.LocalDeviceID, "empty")
	if !eok {
		t.Fatal("failed to find empty file")
	}
	file, fok := snap.Get(protocol.LocalDeviceID, "file")
	if !fok {
		t.Fatal("failed to find non-empty file")
	}

	count := 0
	snap.WithBlocksHash(empty.BlocksHash, func(_ protocol.FileIntf) bool {
		count++
		return true
	})

	if count != 0 {
		t.Fatalf("Found %d entries for empty file, expected 0", count)
	}

	count = 0
	snap.WithBlocksHash(file.BlocksHash, func(_ protocol.FileIntf) bool {
		count++
		return true
	})

	if count != 1 {
		t.Fatalf("Found %d entries for non-empty file, expected 1", count)
	}

	must(t, ffs.Rename("file", "new-file"))
	must(t, ffs.Rename("empty", "new-empty"))

	// Scan
	m.ScanFolders()

	snap = dbSnapshot(t, m, "default")
	defer snap.Release()

	count = 0
	snap.WithBlocksHash(empty.BlocksHash, func(_ protocol.FileIntf) bool {
		count++
		return true
	})

	if count != 0 {
		t.Fatalf("Found %d entries for empty file, expected 0", count)
	}

	count = 0
	snap.WithBlocksHash(file.BlocksHash, func(i protocol.FileIntf) bool {
		count++
		if i.FileName() != "new-file" {
			t.Fatalf("unexpected file name %s, expected new-file", i.FileName())
		}
		return true
	})

	if count != 1 {
		t.Fatalf("Found %d entries for non-empty file, expected 1", count)
	}
}

func TestBlockListMap(t *testing.T) {
	wcfg, fcfg, wcfgCancel := newDefaultCfgWrapper()
	defer wcfgCancel()
	m := setupModel(t, wcfg)
	defer cleanupModel(m)

	ffs := fcfg.Filesystem(nil)
	writeFile(t, ffs, "one", []byte("content"))
	writeFile(t, ffs, "two", []byte("content"))
	writeFile(t, ffs, "three", []byte("content"))
	writeFile(t, ffs, "four", []byte("content"))
	writeFile(t, ffs, "five", []byte("content"))

	m.ScanFolders()

	snap := dbSnapshot(t, m, "default")
	defer snap.Release()
	fi, ok := snap.Get(protocol.LocalDeviceID, "one")
	if !ok {
		t.Error("failed to find existing file")
	}
	var paths []string

	snap.WithBlocksHash(fi.BlocksHash, func(fi protocol.FileIntf) bool {
		paths = append(paths, fi.FileName())
		return true
	})
	snap.Release()

	expected := []string{"one", "two", "three", "four", "five"}
	if !equalStringsInAnyOrder(paths, expected) {
		t.Errorf("expected %q got %q", expected, paths)
	}

	// Fudge the files around
	// Remove
	must(t, ffs.Remove("one"))

	// Modify
	must(t, ffs.Remove("two"))
	writeFile(t, ffs, "two", []byte("mew-content"))

	// Rename
	must(t, ffs.Rename("three", "new-three"))

	// Change type
	must(t, ffs.Remove("four"))
	must(t, ffs.Mkdir("four", 0o644))

	m.ScanFolders()

	// Check we're left with 2 of the 5
	snap = dbSnapshot(t, m, "default")
	defer snap.Release()

	paths = paths[:0]
	snap.WithBlocksHash(fi.BlocksHash, func(fi protocol.FileIntf) bool {
		paths = append(paths, fi.FileName())
		return true
	})
	snap.Release()

	expected = []string{"new-three", "five"}
	if !equalStringsInAnyOrder(paths, expected) {
		t.Errorf("expected %q got %q", expected, paths)
	}
}

func TestScanRenameCaseOnly(t *testing.T) {
	wcfg, fcfg, wcfgCancel := newDefaultCfgWrapper()
	defer wcfgCancel()
	m := setupModel(t, wcfg)
	defer cleanupModel(m)

	ffs := fcfg.Filesystem(nil)
	name := "foo"
	writeFile(t, ffs, name, []byte("contents"))

	m.ScanFolders()

	snap := dbSnapshot(t, m, fcfg.ID)
	defer snap.Release()
	found := false
	snap.WithHave(protocol.LocalDeviceID, func(i protocol.FileIntf) bool {
		if found {
			t.Fatal("got more than one file")
		}
		if i.FileName() != name {
			t.Fatalf("got file %v, expected %v", i.FileName(), name)
		}
		found = true
		return true
	})
	snap.Release()

	upper := strings.ToUpper(name)
	must(t, ffs.Rename(name, upper))
	m.ScanFolders()

	snap = dbSnapshot(t, m, fcfg.ID)
	defer snap.Release()
	found = false
	snap.WithHave(protocol.LocalDeviceID, func(i protocol.FileIntf) bool {
		if i.FileName() == name {
			if i.IsDeleted() {
				return true
			}
			t.Fatal("renamed file not deleted")
		}
		if i.FileName() != upper {
			t.Fatalf("got file %v, expected %v", i.FileName(), upper)
		}
		if found {
			t.Fatal("got more than the expected files")
		}
		found = true
		return true
	})
}

func TestClusterConfigOnFolderAdd(t *testing.T) {
	testConfigChangeTriggersClusterConfigs(t, false, true, nil, func(wrapper config.Wrapper) {
		fcfg := newFolderConfig()
		fcfg.ID = "second"
		fcfg.Label = "second"
		fcfg.Devices = []config.FolderDeviceConfiguration{{
			DeviceID:     device2,
			IntroducedBy: protocol.EmptyDeviceID,
		}}
		setFolder(t, wrapper, fcfg)
	})
}

func TestClusterConfigOnFolderShare(t *testing.T) {
	testConfigChangeTriggersClusterConfigs(t, true, true, nil, func(cfg config.Wrapper) {
		fcfg := cfg.FolderList()[0]
		fcfg.Devices = []config.FolderDeviceConfiguration{{
			DeviceID:     device2,
			IntroducedBy: protocol.EmptyDeviceID,
		}}
		setFolder(t, cfg, fcfg)
	})
}

func TestClusterConfigOnFolderUnshare(t *testing.T) {
	testConfigChangeTriggersClusterConfigs(t, true, false, nil, func(cfg config.Wrapper) {
		fcfg := cfg.FolderList()[0]
		fcfg.Devices = nil
		setFolder(t, cfg, fcfg)
	})
}

func TestClusterConfigOnFolderRemove(t *testing.T) {
	testConfigChangeTriggersClusterConfigs(t, true, false, nil, func(cfg config.Wrapper) {
		rcfg := cfg.RawCopy()
		rcfg.Folders = nil
		replace(t, cfg, rcfg)
	})
}

func TestClusterConfigOnFolderPause(t *testing.T) {
	testConfigChangeTriggersClusterConfigs(t, true, false, nil, func(cfg config.Wrapper) {
		pauseFolder(t, cfg, cfg.FolderList()[0].ID, true)
	})
}

func TestClusterConfigOnFolderUnpause(t *testing.T) {
	testConfigChangeTriggersClusterConfigs(t, true, false, func(cfg config.Wrapper) {
		pauseFolder(t, cfg, cfg.FolderList()[0].ID, true)
	}, func(cfg config.Wrapper) {
		pauseFolder(t, cfg, cfg.FolderList()[0].ID, false)
	})
}

func TestAddFolderCompletion(t *testing.T) {
	// Empty folders are always 100% complete.
	comp := newFolderCompletion(db.Counts{}, db.Counts{}, 0, remoteFolderValid)
	comp.add(newFolderCompletion(db.Counts{}, db.Counts{}, 0, remoteFolderPaused))
	if comp.CompletionPct != 100 {
		t.Error(comp.CompletionPct)
	}

	// Completion is of the whole
	comp = newFolderCompletion(db.Counts{Bytes: 100}, db.Counts{}, 0, remoteFolderValid)             // 100% complete
	comp.add(newFolderCompletion(db.Counts{Bytes: 400}, db.Counts{Bytes: 50}, 0, remoteFolderValid)) // 82.5% complete
	if comp.CompletionPct != 90 {                                                                    // 100 * (1 - 50/500)
		t.Error(comp.CompletionPct)
	}
}

func TestScanDeletedROChangedOnSR(t *testing.T) {
	m, conn, fcfg, wCancel := setupModelWithConnection(t)
	ffs := fcfg.Filesystem(nil)
	defer wCancel()
	defer cleanupModelAndRemoveDir(m, ffs.URI())
	fcfg.Type = config.FolderTypeReceiveOnly
	setFolder(t, m.cfg, fcfg)

	name := "foo"

	writeFile(t, ffs, name, []byte(name))
	m.ScanFolders()

	file, ok := m.testCurrentFolderFile(fcfg.ID, name)
	if !ok {
		t.Fatal("file missing in db")
	}
	// A remote must have the file, otherwise the deletion below is
	// automatically resolved as not a ro-changed item.
	must(t, m.IndexUpdate(conn, fcfg.ID, []protocol.FileInfo{file}))

	must(t, ffs.Remove(name))
	m.ScanFolders()

	if receiveOnlyChangedSize(t, m, fcfg.ID).Deleted != 1 {
		t.Fatal("expected one receive only changed deleted item")
	}

	fcfg.Type = config.FolderTypeSendReceive
	setFolder(t, m.cfg, fcfg)
	m.ScanFolders()

	if receiveOnlyChangedSize(t, m, fcfg.ID).Deleted != 0 {
		t.Fatal("expected no receive only changed deleted item")
	}
	if localSize(t, m, fcfg.ID).Deleted != 1 {
		t.Fatal("expected one local deleted item")
	}
}

func testConfigChangeTriggersClusterConfigs(t *testing.T, expectFirst, expectSecond bool, pre func(config.Wrapper), fn func(config.Wrapper)) {
	t.Helper()
	wcfg, _, wcfgCancel := newDefaultCfgWrapper()
	defer wcfgCancel()
	m := setupModel(t, wcfg)
	defer cleanupModel(m)

	setDevice(t, wcfg, newDeviceConfiguration(wcfg.DefaultDevice(), device2, "device2"))

	if pre != nil {
		pre(wcfg)
	}

	cc1 := make(chan struct{}, 1)
	cc2 := make(chan struct{}, 1)
	fc1 := newFakeConnection(device1, m)
	fc1.ClusterConfigCalls(func(_ protocol.ClusterConfig) {
		cc1 <- struct{}{}
	})
	fc2 := newFakeConnection(device2, m)
	fc2.ClusterConfigCalls(func(_ protocol.ClusterConfig) {
		cc2 <- struct{}{}
	})
	m.AddConnection(fc1, protocol.Hello{})
	m.AddConnection(fc2, protocol.Hello{})

	// Initial CCs
	select {
	case <-cc1:
	default:
		t.Fatal("missing initial CC from device1")
	}
	select {
	case <-cc2:
	default:
		t.Fatal("missing initial CC from device2")
	}

	t.Log("Applying config change")

	fn(wcfg)

	timeout := time.NewTimer(time.Second)
	if expectFirst {
		select {
		case <-cc1:
		case <-timeout.C:
			t.Errorf("timed out before receiving cluste rconfig for first device")
		}
	}
	if expectSecond {
		select {
		case <-cc2:
		case <-timeout.C:
			t.Errorf("timed out before receiving cluste rconfig for second device")
		}
	}
}

// The end result of the tested scenario is that the global version entry has an
// empty version vector and is not deleted, while everything is actually deleted.
// That then causes these files to be considered as needed, while they are not.
// https://github.com/syncthing/syncthing/issues/6961
func TestIssue6961(t *testing.T) {
	wcfg, fcfg, wcfgCancel := newDefaultCfgWrapper()
	defer wcfgCancel()
	tfs := fcfg.Filesystem(nil)
	waiter, err := wcfg.Modify(func(cfg *config.Configuration) {
		cfg.SetDevice(newDeviceConfiguration(cfg.Defaults.Device, device2, "device2"))
		fcfg.Type = config.FolderTypeReceiveOnly
		fcfg.Devices = append(fcfg.Devices, config.FolderDeviceConfiguration{DeviceID: device2})
		cfg.SetFolder(fcfg)
	})
	must(t, err)
	waiter.Wait()
	// Always recalc/repair when opening a fileset.
	m := newModel(t, wcfg, myID, "syncthing", "dev", nil)
	m.db.Close()
	m.db, err = db.NewLowlevel(backend.OpenMemory(), m.evLogger, db.WithRecheckInterval(time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	m.ServeBackground()
	defer cleanupModelAndRemoveDir(m, tfs.URI())
	conn1 := addFakeConn(m, device1, fcfg.ID)
	conn2 := addFakeConn(m, device2, fcfg.ID)
	m.ScanFolders()

	name := "foo"
	version := protocol.Vector{}.Update(device1.Short())

	// Remote, valid and existing file
	must(t, m.Index(conn1, fcfg.ID, []protocol.FileInfo{{Name: name, Version: version, Sequence: 1}}))
	// Remote, invalid (receive-only) and existing file
	must(t, m.Index(conn2, fcfg.ID, []protocol.FileInfo{{Name: name, RawInvalid: true, Sequence: 1}}))
	// Create a local file
	if fd, err := tfs.OpenFile(name, fs.OptCreate, 0o666); err != nil {
		t.Fatal(err)
	} else {
		fd.Close()
	}
	if info, err := tfs.Lstat(name); err != nil {
		t.Fatal(err)
	} else {
		l.Infoln("intest", info.Mode)
	}
	m.ScanFolders()

	// Get rid of valid global
	waiter, err = wcfg.RemoveDevice(device1)
	if err != nil {
		t.Fatal(err)
	}
	waiter.Wait()

	// Delete the local file
	must(t, tfs.Remove(name))
	m.ScanFolders()

	// Drop the remote index, add some other file.
	must(t, m.Index(conn2, fcfg.ID, []protocol.FileInfo{{Name: "bar", RawInvalid: true, Sequence: 1}}))

	// Pause and unpause folder to create new db.FileSet and thus recalculate everything
	pauseFolder(t, wcfg, fcfg.ID, true)
	pauseFolder(t, wcfg, fcfg.ID, false)

	if comp := m.testCompletion(device2, fcfg.ID); comp.NeedDeletes != 0 {
		t.Error("Expected 0 needed deletes, got", comp.NeedDeletes)
	} else {
		t.Log(comp)
	}
}

func TestCompletionEmptyGlobal(t *testing.T) {
	m, conn, fcfg, wcfgCancel := setupModelWithConnection(t)
	defer wcfgCancel()
	defer cleanupModelAndRemoveDir(m, fcfg.Filesystem(nil).URI())
	files := []protocol.FileInfo{{Name: "foo", Version: protocol.Vector{}.Update(myID.Short()), Sequence: 1}}
	m.fmut.Lock()
	m.folderFiles[fcfg.ID].Update(protocol.LocalDeviceID, files)
	m.fmut.Unlock()
	files[0].Deleted = true
	files[0].Version = files[0].Version.Update(device1.Short())
	must(t, m.IndexUpdate(conn, fcfg.ID, files))
	comp := m.testCompletion(protocol.LocalDeviceID, fcfg.ID)
	if comp.CompletionPct != 95 {
		t.Error("Expected completion of 95%, got", comp.CompletionPct)
	}
}

func TestNeedMetaAfterIndexReset(t *testing.T) {
	w, fcfg, wCancel := newDefaultCfgWrapper()
	defer wCancel()
	addDevice2(t, w, fcfg)
	m := setupModel(t, w)
	defer cleanupModelAndRemoveDir(m, fcfg.Path)
	conn1 := addFakeConn(m, device1, fcfg.ID)
	conn2 := addFakeConn(m, device2, fcfg.ID)

	var seq int64 = 1
	files := []protocol.FileInfo{{Name: "foo", Size: 10, Version: protocol.Vector{}.Update(device1.Short()), Sequence: seq}}

	// Start with two remotes having one file, then both deleting it, then
	// only one adding it again.
	must(t, m.Index(conn1, fcfg.ID, files))
	must(t, m.Index(conn2, fcfg.ID, files))
	seq++
	files[0].SetDeleted(device2.Short())
	files[0].Sequence = seq
	must(t, m.IndexUpdate(conn1, fcfg.ID, files))
	must(t, m.IndexUpdate(conn2, fcfg.ID, files))
	seq++
	files[0].Deleted = false
	files[0].Size = 20
	files[0].Version = files[0].Version.Update(device1.Short())
	files[0].Sequence = seq
	must(t, m.IndexUpdate(conn1, fcfg.ID, files))

	if comp := m.testCompletion(device2, fcfg.ID); comp.NeedItems != 1 {
		t.Error("Expected one needed item for device2, got", comp.NeedItems)
	}

	// Pretend we had an index reset on device 1
	must(t, m.Index(conn1, fcfg.ID, files))
	if comp := m.testCompletion(device2, fcfg.ID); comp.NeedItems != 1 {
		t.Error("Expected one needed item for device2, got", comp.NeedItems)
	}
}

func TestCcCheckEncryption(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping on short testing - generating encryption tokens is slow")
	}

	w, fcfg, wCancel := newDefaultCfgWrapper()
	defer wCancel()
	m := setupModel(t, w)
	m.cancel()
	defer cleanupModel(m)

	pw := "foo"
	token := protocol.PasswordToken(m.keyGen, fcfg.ID, pw)
	m.folderEncryptionPasswordTokens[fcfg.ID] = token

	testCases := []struct {
		tokenRemote, tokenLocal             []byte
		isEncryptedRemote, isEncryptedLocal bool
		expectedErr                         error
	}{
		{
			tokenRemote: token,
			tokenLocal:  token,
			expectedErr: errEncryptionInvConfigRemote,
		},
		{
			isEncryptedRemote: true,
			isEncryptedLocal:  true,
			expectedErr:       errEncryptionInvConfigLocal,
		},
		{
			tokenRemote:       token,
			tokenLocal:        nil,
			isEncryptedRemote: false,
			isEncryptedLocal:  false,
			expectedErr:       errEncryptionNotEncryptedLocal,
		},
		{
			tokenRemote:       token,
			tokenLocal:        nil,
			isEncryptedRemote: true,
			isEncryptedLocal:  false,
			expectedErr:       nil,
		},
		{
			tokenRemote:       token,
			tokenLocal:        nil,
			isEncryptedRemote: false,
			isEncryptedLocal:  true,
			expectedErr:       nil,
		},
		{
			tokenRemote:       nil,
			tokenLocal:        token,
			isEncryptedRemote: true,
			isEncryptedLocal:  false,
			expectedErr:       nil,
		},
		{
			tokenRemote:       nil,
			tokenLocal:        token,
			isEncryptedRemote: false,
			isEncryptedLocal:  true,
			expectedErr:       nil,
		},
		{
			tokenRemote:       nil,
			tokenLocal:        token,
			isEncryptedRemote: false,
			isEncryptedLocal:  false,
			expectedErr:       errEncryptionNotEncryptedLocal,
		},
		{
			tokenRemote:       nil,
			tokenLocal:        nil,
			isEncryptedRemote: true,
			isEncryptedLocal:  false,
			expectedErr:       errEncryptionPlainForRemoteEncrypted,
		},
		{
			tokenRemote:       nil,
			tokenLocal:        nil,
			isEncryptedRemote: false,
			isEncryptedLocal:  true,
			expectedErr:       errEncryptionPlainForReceiveEncrypted,
		},
		{
			tokenRemote:       nil,
			tokenLocal:        nil,
			isEncryptedRemote: false,
			isEncryptedLocal:  false,
			expectedErr:       nil,
		},
	}

	for i, tc := range testCases {
		tfcfg := fcfg.Copy()
		if tc.isEncryptedLocal {
			tfcfg.Type = config.FolderTypeReceiveEncrypted
			m.folderEncryptionPasswordTokens[fcfg.ID] = token
		}
		dcfg := config.FolderDeviceConfiguration{DeviceID: device1}
		if tc.isEncryptedRemote {
			dcfg.EncryptionPassword = pw
		}

		deviceInfos := &clusterConfigDeviceInfo{
			remote: protocol.Device{ID: device1, EncryptionPasswordToken: tc.tokenRemote},
			local:  protocol.Device{ID: myID, EncryptionPasswordToken: tc.tokenLocal},
		}
		err := m.ccCheckEncryption(tfcfg, dcfg, deviceInfos, false)
		if err != tc.expectedErr {
			t.Errorf("Testcase %v: Expected error %v, got %v", i, tc.expectedErr, err)
		}

		if tc.expectedErr == nil {
			err := m.ccCheckEncryption(tfcfg, dcfg, deviceInfos, true)
			if tc.isEncryptedRemote || tc.isEncryptedLocal {
				if err != nil {
					t.Errorf("Testcase %v: Expected no error, got %v", i, err)
				}
			} else {
				if err != errEncryptionNotEncryptedUntrusted {
					t.Errorf("Testcase %v: Expected error %v, got %v", i, errEncryptionNotEncryptedUntrusted, err)
				}
			}
		}

		if err != nil || (!tc.isEncryptedRemote && !tc.isEncryptedLocal) {
			continue
		}

		if tc.isEncryptedLocal {
			m.folderEncryptionPasswordTokens[fcfg.ID] = []byte("notAMatch")
		} else {
			dcfg.EncryptionPassword = "notAMatch"
		}
		err = m.ccCheckEncryption(tfcfg, dcfg, deviceInfos, false)
		if err != errEncryptionPassword {
			t.Errorf("Testcase %v: Expected error %v, got %v", i, errEncryptionPassword, err)
		}
	}
}

func TestCCFolderNotRunning(t *testing.T) {
	// Create the folder, but don't start it.
	w, fcfg, wCancel := newDefaultCfgWrapper()
	defer wCancel()
	tfs := fcfg.Filesystem(nil)
	m := newModel(t, w, myID, "syncthing", "dev", nil)
	defer cleanupModelAndRemoveDir(m, tfs.URI())

	// A connection can happen before all the folders are started.
	cc, _ := m.generateClusterConfig(device1)
	if l := len(cc.Folders); l != 1 {
		t.Fatalf("Expected 1 folder in CC, got %v", l)
	}
	folder := cc.Folders[0]
	if id := folder.ID; id != fcfg.ID {
		t.Fatalf("Expected folder %v, got %v", fcfg.ID, id)
	}
	if l := len(folder.Devices); l != 2 {
		t.Fatalf("Expected 2 devices in CC, got %v", l)
	}
	local := folder.Devices[1]
	if local.ID != myID {
		local = folder.Devices[0]
	}
	if !folder.Paused && local.IndexID == 0 {
		t.Errorf("Folder isn't paused, but index-id is zero")
	}
}

func TestPendingFolder(t *testing.T) {
	w, _, wCancel := newDefaultCfgWrapper()
	defer wCancel()
	m := setupModel(t, w)
	defer cleanupModel(m)

	setDevice(t, w, config.DeviceConfiguration{DeviceID: device2})
	pfolder := "default"
	of := db.ObservedFolder{
		Time:  time.Now().Truncate(time.Second),
		Label: pfolder,
	}
	if err := m.db.AddOrUpdatePendingFolder(pfolder, of, device2); err != nil {
		t.Fatal(err)
	}
	deviceFolders, err := m.PendingFolders(protocol.EmptyDeviceID)
	if err != nil {
		t.Fatal(err)
	} else if pf, ok := deviceFolders[pfolder]; !ok {
		t.Errorf("folder %v not pending", pfolder)
	} else if _, ok := pf.OfferedBy[device2]; !ok {
		t.Errorf("folder %v not pending for device %v", pfolder, device2)
	} else if len(pf.OfferedBy) > 1 {
		t.Errorf("folder %v pending for too many devices %v", pfolder, pf.OfferedBy)
	}

	device3, err := protocol.DeviceIDFromString("AIBAEAQ-CAIBAEC-AQCAIBA-EAQCAIA-BAEAQCA-IBAEAQC-CAIBAEA-QCAIBA7")
	if err != nil {
		t.Fatal(err)
	}
	setDevice(t, w, config.DeviceConfiguration{DeviceID: device3})
	if err := m.db.AddOrUpdatePendingFolder(pfolder, of, device3); err != nil {
		t.Fatal(err)
	}
	deviceFolders, err = m.PendingFolders(device2)
	if err != nil {
		t.Fatal(err)
	} else if pf, ok := deviceFolders[pfolder]; !ok {
		t.Errorf("folder %v not pending when filtered", pfolder)
	} else if _, ok := pf.OfferedBy[device2]; !ok {
		t.Errorf("folder %v not pending for device %v when filtered", pfolder, device2)
	} else if _, ok := pf.OfferedBy[device3]; ok {
		t.Errorf("folder %v pending for device %v, but not filtered out", pfolder, device3)
	}

	waiter, err := w.RemoveDevice(device3)
	if err != nil {
		t.Fatal(err)
	}
	waiter.Wait()
	deviceFolders, err = m.PendingFolders(protocol.EmptyDeviceID)
	if err != nil {
		t.Fatal(err)
	} else if pf, ok := deviceFolders[pfolder]; !ok {
		t.Errorf("folder %v not pending", pfolder)
	} else if _, ok := pf.OfferedBy[device3]; ok {
		t.Errorf("folder %v pending for removed device %v", pfolder, device3)
	}

	waiter, err = w.RemoveFolder(pfolder)
	if err != nil {
		t.Fatal(err)
	}
	waiter.Wait()
	deviceFolders, err = m.PendingFolders(protocol.EmptyDeviceID)
	if err != nil {
		t.Fatal(err)
	} else if _, ok := deviceFolders[pfolder]; ok {
		t.Errorf("folder %v still pending after local removal", pfolder)
	}
}

func TestDeletedNotLocallyChangedReceiveOnly(t *testing.T) {
	deletedNotLocallyChanged(t, config.FolderTypeReceiveOnly)
}

func TestDeletedNotLocallyChangedReceiveEncrypted(t *testing.T) {
	deletedNotLocallyChanged(t, config.FolderTypeReceiveEncrypted)
}

func deletedNotLocallyChanged(t *testing.T, ft config.FolderType) {
	w, fcfg, wCancel := newDefaultCfgWrapper()
	tfs := fcfg.Filesystem(nil)
	fcfg.Type = ft
	setFolder(t, w, fcfg)
	defer wCancel()
	m := setupModel(t, w)
	defer cleanupModelAndRemoveDir(m, tfs.URI())

	name := "foo"
	writeFile(t, tfs, name, nil)
	must(t, m.ScanFolder(fcfg.ID))

	fi, ok, err := m.CurrentFolderFile(fcfg.ID, name)
	must(t, err)
	if !ok {
		t.Fatal("File hasn't been added")
	}
	if !fi.IsReceiveOnlyChanged() {
		t.Fatal("File isn't receive-only-changed")
	}

	must(t, tfs.Remove(name))
	must(t, m.ScanFolder(fcfg.ID))

	_, ok, err = m.CurrentFolderFile(fcfg.ID, name)
	must(t, err)
	if ok {
		t.Error("Expected file to be removed from db")
	}
}

func equalStringsInAnyOrder(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	sort.Strings(a)
	sort.Strings(b)
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// modtimeTruncatingFS is a FileSystem that returns modification times only
// to the closest two `trunc` interval.
type modtimeTruncatingFS struct {
	trunc time.Duration
	fs.Filesystem
}

func (f modtimeTruncatingFS) Lstat(name string) (fs.FileInfo, error) {
	fmt.Println("lstat", name)
	info, err := f.Filesystem.Lstat(name)
	return modtimeTruncatingFileInfo{trunc: f.trunc, FileInfo: info}, err
}

func (f modtimeTruncatingFS) Stat(name string) (fs.FileInfo, error) {
	fmt.Println("stat", name)
	info, err := f.Filesystem.Stat(name)
	return modtimeTruncatingFileInfo{trunc: f.trunc, FileInfo: info}, err
}

func (f modtimeTruncatingFS) Walk(root string, walkFn fs.WalkFunc) error {
	return f.Filesystem.Walk(root, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return walkFn(path, nil, err)
		}
		fmt.Println("walk", info.Name())
		return walkFn(path, modtimeTruncatingFileInfo{trunc: f.trunc, FileInfo: info}, nil)
	})
}

type modtimeTruncatingFileInfo struct {
	trunc time.Duration
	fs.FileInfo
}

func (fi modtimeTruncatingFileInfo) ModTime() time.Time {
	return fi.FileInfo.ModTime().Truncate(fi.trunc)
}
