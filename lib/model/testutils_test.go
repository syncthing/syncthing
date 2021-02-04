// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"context"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/thejerf/suture/v4"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/db/backend"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/ignore"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/rand"
)

var (
	myID, device1, device2  protocol.DeviceID
	defaultCfgWrapper       config.Wrapper
	defaultCfgWrapperCancel context.CancelFunc
	defaultFolderConfig     config.FolderConfiguration
	defaultFs               fs.Filesystem
	defaultCfg              config.Configuration
	defaultAutoAcceptCfg    config.Configuration
)

func init() {
	myID, _ = protocol.DeviceIDFromString("ZNWFSWE-RWRV2BD-45BLMCV-LTDE2UR-4LJDW6J-R5BPWEB-TXD27XJ-IZF5RA4")
	device1, _ = protocol.DeviceIDFromString("AIR6LPZ-7K4PTTV-UXQSMUU-CPQ5YWH-OEDFIIQ-JUG777G-2YQXXR5-YD6AWQR")
	device2, _ = protocol.DeviceIDFromString("GYRZZQB-IRNPV4Z-T7TC52W-EQYJ3TT-FDQW6MW-DFLMU42-SSSU6EM-FBK2VAY")

	defaultFolderConfig = testFolderConfig("testdata")
	defaultFs = defaultFolderConfig.Filesystem()

	defaultCfgWrapper, defaultCfgWrapperCancel = createTmpWrapper(config.New(myID))
	waiter, _ := defaultCfgWrapper.Modify(func(cfg *config.Configuration) {
		cfg.SetDevice(config.NewDeviceConfiguration(device1, "device1"))
		cfg.SetFolder(defaultFolderConfig)
		cfg.Options.KeepTemporariesH = 1
	})
	waiter.Wait()

	defaultCfg = defaultCfgWrapper.RawCopy()

	defaultAutoAcceptCfg = config.Configuration{
		Version: config.CurrentVersion,
		Devices: []config.DeviceConfiguration{
			{
				DeviceID: myID, // self
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
			DefaultFolderPath: ".",
		},
	}
}

func createTmpWrapper(cfg config.Configuration) (config.Wrapper, context.CancelFunc) {
	tmpFile, err := ioutil.TempFile("", "syncthing-testConfig-")
	if err != nil {
		panic(err)
	}
	wrapper := config.Wrap(tmpFile.Name(), cfg, myID, events.NoopLogger)
	tmpFile.Close()
	if cfgService, ok := wrapper.(suture.Service); ok {
		ctx, cancel := context.WithCancel(context.Background())
		go cfgService.Serve(ctx)
		return wrapper, cancel
	}
	return wrapper, func() {}
}

func tmpDefaultWrapper() (config.Wrapper, config.FolderConfiguration, context.CancelFunc) {
	w, cancel := createTmpWrapper(defaultCfgWrapper.RawCopy())
	fcfg := testFolderConfigTmp()
	_, _ = w.Modify(func(cfg *config.Configuration) {
		cfg.SetFolder(fcfg)
	})
	return w, fcfg, cancel
}

func testFolderConfigTmp() config.FolderConfiguration {
	tmpDir := createTmpDir()
	return testFolderConfig(tmpDir)
}

func testFolderConfig(path string) config.FolderConfiguration {
	cfg := config.NewFolderConfiguration(myID, "default", "default", fs.FilesystemTypeBasic, path)
	cfg.FSWatcherEnabled = false
	cfg.Devices = append(cfg.Devices, config.FolderDeviceConfiguration{DeviceID: device1})
	return cfg
}

func testFolderConfigFake() config.FolderConfiguration {
	cfg := config.NewFolderConfiguration(myID, "default", "default", fs.FilesystemTypeFake, rand.String(32)+"?content=true")
	cfg.FSWatcherEnabled = false
	cfg.Devices = append(cfg.Devices, config.FolderDeviceConfiguration{DeviceID: device1})
	return cfg
}

func setupModelWithConnection(t testing.TB) (*testModel, *fakeConnection, config.FolderConfiguration, context.CancelFunc) {
	t.Helper()
	w, fcfg, cancel := tmpDefaultWrapper()
	m, fc := setupModelWithConnectionFromWrapper(t, w)
	return m, fc, fcfg, cancel
}

func setupModelWithConnectionFromWrapper(t testing.TB, w config.Wrapper) (*testModel, *fakeConnection) {
	t.Helper()
	m := setupModel(t, w)

	fc := addFakeConn(m, device1)
	fc.folder = "default"

	_ = m.ScanFolder("default")

	return m, fc
}

func setupModel(t testing.TB, w config.Wrapper) *testModel {
	t.Helper()
	m := newModel(t, w, myID, "syncthing", "dev", nil)
	m.ServeBackground()
	<-m.started

	m.ScanFolders()

	return m
}

type testModel struct {
	*model
	cancel   context.CancelFunc
	evCancel context.CancelFunc
	stopped  chan struct{}
}

func newModel(t testing.TB, cfg config.Wrapper, id protocol.DeviceID, clientName, clientVersion string, protectedFiles []string) *testModel {
	t.Helper()
	evLogger := events.NewLogger()
	ldb, err := db.NewLowlevel(backend.OpenMemory(), evLogger)
	if err != nil {
		t.Fatal(err)
	}
	m := NewModel(cfg, id, clientName, clientVersion, ldb, protectedFiles, evLogger).(*model)
	ctx, cancel := context.WithCancel(context.Background())
	go evLogger.Serve(ctx)
	return &testModel{
		model:    m,
		evCancel: cancel,
		stopped:  make(chan struct{}),
	}
}

func (m *testModel) ServeBackground() {
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	go func() {
		m.model.Serve(ctx)
		close(m.stopped)
	}()
	<-m.started
}

func cleanupModel(m *testModel) {
	if m.cancel != nil {
		m.cancel()
		<-m.stopped
	}
	m.evCancel()
	m.db.Close()
	os.Remove(m.cfg.ConfigPath())
}

func cleanupModelAndRemoveDir(m *testModel, dir string) {
	cleanupModel(m)
	os.RemoveAll(dir)
}

func createTmpDir() string {
	tmpDir, err := ioutil.TempDir("", "syncthing_testFolder-")
	if err != nil {
		panic("Failed to create temporary testing dir")
	}
	return tmpDir
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

func localSize(t *testing.T, m Model, folder string) db.Counts {
	t.Helper()
	snap := dbSnapshot(t, m, folder)
	defer snap.Release()
	return snap.LocalSize()
}

func globalSize(t *testing.T, m Model, folder string) db.Counts {
	t.Helper()
	snap := dbSnapshot(t, m, folder)
	defer snap.Release()
	return snap.GlobalSize()
}

func receiveOnlyChangedSize(t *testing.T, m Model, folder string) db.Counts {
	t.Helper()
	snap := dbSnapshot(t, m, folder)
	defer snap.Release()
	return snap.ReceiveOnlyChangedSize()
}

func needSize(t *testing.T, m Model, folder string) db.Counts {
	t.Helper()
	snap := dbSnapshot(t, m, folder)
	defer snap.Release()
	return snap.NeedSize(protocol.LocalDeviceID)
}

func dbSnapshot(t *testing.T, m Model, folder string) *db.Snapshot {
	t.Helper()
	snap, err := m.DBSnapshot(folder)
	if err != nil {
		t.Fatal(err)
	}
	return snap
}

// Reach in and update the ignore matcher to one that always does
// reloads when asked to, instead of checking file mtimes. This is
// because we will be changing the files on disk often enough that the
// mtimes will be unreliable to determine change status.
func folderIgnoresAlwaysReload(t testing.TB, m *testModel, fcfg config.FolderConfiguration) {
	t.Helper()
	m.removeFolder(fcfg)
	fset := newFileSet(t, fcfg.ID, fcfg.Filesystem(), m.db)
	ignores := ignore.New(fcfg.Filesystem(), ignore.WithChangeDetector(newAlwaysChanged()))
	m.fmut.Lock()
	m.addAndStartFolderLockedWithIgnores(fcfg, fset, ignores)
	m.fmut.Unlock()
}

func basicClusterConfig(local, remote protocol.DeviceID, folders ...string) protocol.ClusterConfig {
	var cc protocol.ClusterConfig
	for _, folder := range folders {
		cc.Folders = append(cc.Folders, protocol.Folder{
			ID: folder,
			Devices: []protocol.Device{
				{
					ID: local,
				},
				{
					ID: remote,
				},
			},
		})
	}
	return cc
}

func localIndexUpdate(m *testModel, folder string, fs []protocol.FileInfo) {
	m.fmut.RLock()
	fset := m.folderFiles[folder]
	m.fmut.RUnlock()

	fset.Update(protocol.LocalDeviceID, fs)
	seq := fset.Sequence(protocol.LocalDeviceID)
	filenames := make([]string, len(fs))
	for i, file := range fs {
		filenames[i] = file.Name
	}
	m.evLogger.Log(events.LocalIndexUpdated, map[string]interface{}{
		"folder":    folder,
		"items":     len(fs),
		"filenames": filenames,
		"sequence":  seq,
		"version":   seq, // legacy for sequence
	})
}

func newFileSet(t testing.TB, folder string, fs fs.Filesystem, ldb *db.Lowlevel) *db.FileSet {
	t.Helper()
	fset, err := db.NewFileSet(folder, fs, ldb)
	if err != nil {
		t.Fatal(err)
	}
	return fset
}

func replace(t testing.TB, w config.Wrapper, to config.Configuration) {
	t.Helper()
	waiter, err := w.Modify(func(cfg *config.Configuration) {
		*cfg = to
	})
	if err != nil {
		t.Fatal(err)
	}
	waiter.Wait()
}

func pauseFolder(t testing.TB, w config.Wrapper, id string, paused bool) {
	t.Helper()
	waiter, err := w.Modify(func(cfg *config.Configuration) {
		_, i, _ := cfg.Folder(id)
		cfg.Folders[i].Paused = paused
	})
	if err != nil {
		t.Fatal(err)
	}
	waiter.Wait()
}

func setFolder(t testing.TB, w config.Wrapper, fcfg config.FolderConfiguration) {
	t.Helper()
	waiter, err := w.Modify(func(cfg *config.Configuration) {
		cfg.SetFolder(fcfg)
	})
	if err != nil {
		t.Fatal(err)
	}
	waiter.Wait()
}

func pauseDevice(t testing.TB, w config.Wrapper, id protocol.DeviceID, paused bool) {
	t.Helper()
	waiter, err := w.Modify(func(cfg *config.Configuration) {
		_, i, _ := cfg.Device(id)
		cfg.Devices[i].Paused = paused
	})
	if err != nil {
		t.Fatal(err)
	}
	waiter.Wait()
}

func setDevice(t testing.TB, w config.Wrapper, device config.DeviceConfiguration) {
	t.Helper()
	waiter, err := w.Modify(func(cfg *config.Configuration) {
		cfg.SetDevice(device)
	})
	if err != nil {
		t.Fatal(err)
	}
	waiter.Wait()
}

func addDevice2(t testing.TB, w config.Wrapper, fcfg config.FolderConfiguration) {
	waiter, err := w.Modify(func(cfg *config.Configuration) {
		cfg.SetDevice(config.NewDeviceConfiguration(device2, "device2"))
		fcfg.Devices = append(fcfg.Devices, config.FolderDeviceConfiguration{DeviceID: device2})
		cfg.SetFolder(fcfg)
	})
	must(t, err)
	waiter.Wait()
}
