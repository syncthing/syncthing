// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package features

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/model"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/test"
	"github.com/thejerf/suture"
)

// the feature 'single-global-folder-scanner'
// should make it possible to have only one active process of
// scanning and maybe hashing shared folders at a time
//
// in the global settings it can be switched on/off

func TestMain(m *testing.M) {
	tempDir := test.NewTemporaryDirectoryForTests()
	defer tempDir.Cleanup()

	os.Exit(m.Run())
}

// the default setting is to have it switched off
func Test_shouldBeSwitchedOffByDefault(t *testing.T) {
	options := createDefaultConfig().Options()

	assert.False(t, options.SingleGlobalFolderScanner, "Expected to be disabled by default")
}

// behaviour should be that if the instance has multiple folders
// all will be scanned in parallel
func Test_shouldRunInParallelByDefault(t *testing.T) {
	folderConfig1 := config.NewFolderConfiguration(protocol.LocalDeviceID, "id1", "label1", fs.FilesystemTypeBasic, "testdata1")
	folderConfig1.SetFilesystem(&testFilesystem{})

	folderConfig2 := config.NewFolderConfiguration(protocol.LocalDeviceID, "id2", "label2", fs.FilesystemTypeBasic, "testdata2")
	folderConfig2.SetFilesystem(&testFilesystem{})

	cfg := config.New(protocol.LocalDeviceID)
	cfg.Folders = append(cfg.Folders, folderConfig1)
	cfg.Folders = append(cfg.Folders, folderConfig2)

	mainService := suture.New("main", suture.Spec{
		Log: func(line string) {
			//
		},
	})
	mainService.ServeBackground()
	defer mainService.Stop()

	m := setUpModel(createConfig(cfg))
	mainService.Add(m)
	m.AddFolder(folderConfig1)
	m.AddFolder(folderConfig2)

	m.StartFolder(folderConfig1.ID)
	m.StartFolder(folderConfig2.ID)
	m.ScanFolders()

	//time.Sleep(1 * time.Second)
}

func createDefaultConfig() *config.Wrapper {
	cfg := config.New(protocol.LocalDeviceID)
	return createConfig(cfg)
}

func createConfig(cfg config.Configuration) *config.Wrapper {
	return config.Wrap("config.xml", cfg)
}

func setUpModel(cfg *config.Wrapper) *model.Model {
	db := db.OpenMemory()
	m := model.NewModel(cfg, protocol.LocalDeviceID, "syncthing", "dev", db, nil)
	return m
}

func setUpDefaultModel(cfg config.Configuration) *model.Model {
	config := createConfig(cfg)
	return setUpModel(config)
}

type testFilesystem struct {
	fs.Filesystem
	err    error
	fsType fs.FilesystemType
	uri    string
}

func (fs *testFilesystem) Chmod(name string, mode fs.FileMode) error                   { return fs.err }
func (fs *testFilesystem) Chtimes(name string, atime time.Time, mtime time.Time) error { return fs.err }
func (fs *testFilesystem) Create(name string) (fs.File, error)                         { return nil, fs.err }
func (fs *testFilesystem) CreateSymlink(name, target string) error                     { return fs.err }
func (fs *testFilesystem) DirNames(name string) ([]string, error)                      { return nil, fs.err }
func (fs *testFilesystem) Lstat(name string) (fs.FileInfo, error)                      { return &fakeInfo{name, 0}, fs.err }
func (fs *testFilesystem) Mkdir(name string, perm fs.FileMode) error                   { return fs.err }
func (fs *testFilesystem) MkdirAll(name string, perm fs.FileMode) error                { return fs.err }
func (fs *testFilesystem) Open(name string) (fs.File, error) {
	return &fakeFile{name, int64(5), 0}, nil
}
func (fs *testFilesystem) OpenFile(string, int, fs.FileMode) (fs.File, error) { return nil, fs.err }
func (fs *testFilesystem) ReadSymlink(name string) (string, error)            { return "", fs.err }
func (fs *testFilesystem) Remove(name string) error                           { return fs.err }
func (fs *testFilesystem) RemoveAll(name string) error                        { return fs.err }
func (fs *testFilesystem) Rename(oldname, newname string) error               { return fs.err }
func (fs *testFilesystem) Stat(name string) (fs.FileInfo, error)              { return &fakeInfo{name, 0}, nil }
func (fs *testFilesystem) SymlinksSupported() bool                            { return false }
func (fs *testFilesystem) Walk(root string, walkFn fs.WalkFunc) error         { return fs.err }
func (fs *testFilesystem) Unhide(name string) error                           { return fs.err }
func (fs *testFilesystem) Hide(name string) error                             { return fs.err }
func (fs *testFilesystem) Glob(pattern string) ([]string, error)              { return nil, fs.err }
func (fs *testFilesystem) SyncDir(name string) error                          { return fs.err }
func (fs *testFilesystem) Roots() ([]string, error)                           { return nil, fs.err }
func (f *testFilesystem) Usage(name string) (fs.Usage, error) {
	return fs.Usage{int64(0), int64(0)}, nil
}

//func (fs *testFilesystem) Usage(name string) (fs.Usage, error)                         { return nil, fs.err }
func (fs *testFilesystem) Type() fs.FilesystemType { return fs.fsType }
func (fs *testFilesystem) URI() string             { return fs.uri }
func (fs *testFilesystem) Watch(path string, ignore fs.Matcher, ctx context.Context, ignorePerms bool) (<-chan fs.Event, error) {
	return nil, fs.err
}

type fakeInfo struct {
	name string
	size int64
}

func (f fakeInfo) Name() string       { return f.name }
func (f fakeInfo) Mode() fs.FileMode  { return 0755 }
func (f fakeInfo) Size() int64        { return f.size }
func (f fakeInfo) ModTime() time.Time { return time.Unix(1234567890, 0) }
func (f fakeInfo) IsDir() bool        { return strings.Contains(filepath.Base(f.name), "dir") || f.name == "." }
func (f fakeInfo) IsRegular() bool    { return !f.IsDir() }
func (f fakeInfo) IsSymlink() bool    { return false }

type fakeFile struct {
	name       string
	size       int64
	readOffset int64
}

func (f *fakeFile) Name() string {
	return f.name
}

func (f *fakeFile) Read(bs []byte) (int, error) {
	remaining := f.size - f.readOffset
	if remaining == 0 {
		return 0, io.EOF
	}
	if remaining < int64(len(bs)) {
		f.readOffset = f.size
		return int(remaining), nil
	}
	f.readOffset += int64(len(bs))
	return len(bs), nil
}

var errNotSupp = errors.New("not supported")

func (f *fakeFile) Stat() (fs.FileInfo, error) {
	return fakeInfo{f.name, f.size}, nil
}

func (f *fakeFile) Write([]byte) (int, error)          { return 0, errNotSupp }
func (f *fakeFile) WriteAt([]byte, int64) (int, error) { return 0, errNotSupp }
func (f *fakeFile) Close() error                       { return nil }
func (f *fakeFile) Truncate(size int64) error          { return errNotSupp }
func (f *fakeFile) ReadAt([]byte, int64) (int, error)  { return 0, errNotSupp }
func (f *fakeFile) Seek(int64, int) (int64, error)     { return 0, errNotSupp }
func (f *fakeFile) Sync() error                        { return nil }
