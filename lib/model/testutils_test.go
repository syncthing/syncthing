// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"io/ioutil"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/protocol"
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

func setupModelWithConnection() (*model, *fakeConnection, config.FolderConfiguration, config.Wrapper) {
	w, fcfg := tmpDefaultWrapper()
	m, fc := setupModelWithConnectionFromWrapper(w)
	return m, fc, fcfg, w
}

func setupModelWithConnectionFromWrapper(w config.Wrapper) (*model, *fakeConnection) {
	m := setupModel(w)

	fc := addFakeConn(m, device1)
	fc.folder = "default"

	_ = m.ScanFolder("default")

	return m, fc
}

func setupModel(w config.Wrapper) *model {
	db := db.OpenMemory()
	m := newModel(w, myID, "syncthing", "dev", db, nil)
	m.ServeBackground()
	for id, cfg := range w.Folders() {
		if !cfg.Paused {
			m.AddFolder(cfg)
			m.StartFolder(id)
		}
	}

	m.ScanFolders()

	return m
}

func createTmpDir() string {
	tmpDir, err := ioutil.TempDir("", "syncthing_testFolder-")
	if err != nil {
		panic("Failed to create temporary testing dir")
	}
	return tmpDir
}
