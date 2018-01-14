// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package features

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/model"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/thejerf/suture"
)

// the feature 'single-global-scanner'
// should make it possible to have only one active process of
// scanning and maybe hashing shared folders at a time
//
// in the global settings it can be switched on/off

// the default setting is to have it switched off
func Test_shouldBeSwitchedOffByDefault(t *testing.T) {
	options := createDefaultConfig().Options()

	assert.False(t, options.SingleGlobalScanner, "Expected to be disabled by default")
}

// behaviour should be that if the instance has multiple folders
// all will be scanned in parallel
func Test_filesystemShouldWalkInParallel(t *testing.T) {
	ctx, _ := context.WithTimeout(context.Background(), time.Millisecond*20000)

	cfg := createConfigWithFolders(50)

	model := setUpModel(createWrapper(cfg))

	mainService := startMainService()
	defer mainService.Stop()
	mainService.Add(model)

	testFilesystem := startFolders(ctx, cfg, model)
	model.ScanFolders()

	assert.True(t, testFilesystem.max > int32(1))
}

// behaviour should be that if the instance has multiple folders
// only one at a time will be scanned and walked
func Test_filesystemShouldNotWalkInParallel(t *testing.T) {
	ctx, _ := context.WithTimeout(context.Background(), time.Millisecond*20000)

	cfg := createConfigWithFolders(50)
	cfg.Options.SingleGlobalScanner = true

	model := setUpModel(createWrapper(cfg))

	mainService := startMainService()
	defer mainService.Stop()
	mainService.Add(model)

	testFilesystem := startFolders(ctx, cfg, model)

	model.ScanFolders()

	assert.Equal(t, int32(1), testFilesystem.max)
}

func startFolders(ctx context.Context, cfg config.Configuration, m *model.Model) *testFilesystem {
	testFilesystem := newTestFilesystem(ctx)
	for _, cfg := range cfg.Folders {
		cfg.SetFilesystem(testFilesystem)
		m.AddFolder(cfg)
		m.StartFolder(cfg.ID)
	}
	return testFilesystem
}

func startMainService() *suture.Supervisor {
	mainService := suture.New("main", suture.Spec{
		Log: func(line string) {
			//
		},
	})
	mainService.ServeBackground()
	return mainService
}
func createConfigWithFolders(number uint) config.Configuration {
	cfg := config.New(protocol.LocalDeviceID)

	f := func(label string, id uint) string {
		return fmt.Sprintf("%s%d", label, id)
	}

	for i := uint(0); i < number; i++ {
		folderName := f("testdata", i)
		os.Mkdir(folderName, 0777)
		folderConfig1 := config.NewFolderConfiguration(protocol.LocalDeviceID, f("id", i), f("label", i), fs.FilesystemTypeBasic, folderName)
		cfg.Folders = append(cfg.Folders, folderConfig1)
	}
	return cfg
}

func createDefaultConfig() *config.Wrapper {
	cfg := config.New(protocol.LocalDeviceID)
	return createWrapper(cfg)
}

func createWrapper(cfg config.Configuration) *config.Wrapper {
	return config.Wrap("config.xml", cfg)
}

func setUpModel(cfg *config.Wrapper) *model.Model {
	db := db.OpenMemory()
	m := model.NewModel(cfg, protocol.LocalDeviceID, "syncthing", "dev", db, nil)
	return m
}

type testFilesystem struct {
	fs.BasicFilesystem

	counter int32
	max     int32
	count   chan int32
}

func newTestFilesystem(ctx context.Context) *testFilesystem {
	root, _ := os.Getwd()
	filesystem := fs.NewBasicFilesystem(root)
	l := &testFilesystem{
		count:           make(chan int32),
		BasicFilesystem: *filesystem,
	}

	go func() {
		for {
			select {
			case step := <-l.count:
				counter := atomic.LoadInt32(&l.counter)
				counter += step
				atomic.StoreInt32(&l.counter, counter)

				max := atomic.LoadInt32(&l.max)
				if counter > max {
					atomic.StoreInt32(&l.max, counter)
				}
				fmt.Println("max: ", max)
			case <-ctx.Done():
				return
			}
		}
	}()

	return l
}

func (fs *testFilesystem) Walk(root string, walkFn fs.WalkFunc) error {
	fs.count <- 1
	defer func() { fs.count <- -1 }()

	// work to have a chance to keep them busy at the same time
	time.Sleep(time.Millisecond * 100)
	return nil
}
