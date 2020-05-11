// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"io/ioutil"
	"os"
	"testing"
	"time"

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
	myID, device1, device2 protocol.DeviceID
	defaultCfgWrapper      config.Wrapper
	defaultFolderConfig    config.FolderConfiguration
	defaultFs              fs.Filesystem
	defaultCfg             config.Configuration
	defaultAutoAcceptCfg   config.Configuration
)

func init() {
	myID, _ = protocol.DeviceIDFromString("ZNWFSWE-RWRV2BD-45BLMCV-LTDE2UR-4LJDW6J-R5BPWEB-TXD27XJ-IZF5RA4")
	device1, _ = protocol.DeviceIDFromString("AIR6LPZ-7K4PTTV-UXQSMUU-CPQ5YWH-OEDFIIQ-JUG777G-2YQXXR5-YD6AWQR")
	device2, _ = protocol.DeviceIDFromString("GYRZZQB-IRNPV4Z-T7TC52W-EQYJ3TT-FDQW6MW-DFLMU42-SSSU6EM-FBK2VAY")

	defaultFs = fs.NewFilesystem(fs.FilesystemTypeBasic, "testdata")

	defaultFolderConfig = testFolderConfig("testdata")

	defaultCfgWrapper = createTmpWrapper(config.New(myID))
	_, _ = defaultCfgWrapper.SetDevice(config.NewDeviceConfiguration(device1, "device1"))
	_, _ = defaultCfgWrapper.SetFolder(defaultFolderConfig)
	opts := defaultCfgWrapper.Options()
	opts.KeepTemporariesH = 1
	_, _ = defaultCfgWrapper.SetOptions(opts)

	defaultCfg = defaultCfgWrapper.RawCopy()

	defaultAutoAcceptCfg = config.Configuration{
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

func tmpDefaultWrapper() (config.Wrapper, config.FolderConfiguration) {
	w := createTmpWrapper(defaultCfgWrapper.RawCopy())
	fcfg := testFolderConfigTmp()
	_, _ = w.SetFolder(fcfg)
	return w, fcfg
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

func setupModelWithConnection() (*model, *fakeConnection, config.FolderConfiguration) {
	w, fcfg := tmpDefaultWrapper()
	m, fc := setupModelWithConnectionFromWrapper(w)
	return m, fc, fcfg
}

func setupModelWithConnectionFromWrapper(w config.Wrapper) (*model, *fakeConnection) {
	m := setupModel(w)

	fc := addFakeConn(m, device1)
	fc.folder = "default"

	_ = m.ScanFolder("default")

	return m, fc
}

func setupModel(w config.Wrapper) *model {
	db := db.NewLowlevel(backend.OpenMemory())
	m := newModel(w, myID, "syncthing", "dev", db, nil)
	m.ServeBackground()

	m.ScanFolders()

	return m
}

func newModel(cfg config.Wrapper, id protocol.DeviceID, clientName, clientVersion string, ldb *db.Lowlevel, protectedFiles []string) *model {
	evLogger := events.NewLogger()
	m := NewModel(cfg, id, clientName, clientVersion, ldb, protectedFiles, evLogger).(*model)
	go evLogger.Serve()
	return m
}

func cleanupModel(m *model) {
	m.Stop()
	m.db.Close()
	m.evLogger.Stop()
	os.Remove(m.cfg.ConfigPath())
}

func cleanupModelAndRemoveDir(m *model, dir string) {
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
func folderIgnoresAlwaysReload(m *model, fcfg config.FolderConfiguration) {
	m.removeFolder(fcfg)
	fset := db.NewFileSet(fcfg.ID, fcfg.Filesystem(), m.db)
	ignores := ignore.New(fcfg.Filesystem(), ignore.WithCache(true), ignore.WithChangeDetector(newAlwaysChanged()))
	m.fmut.Lock()
	m.addAndStartFolderLockedWithIgnores(fcfg, fset, ignores)
	m.fmut.Unlock()
}
