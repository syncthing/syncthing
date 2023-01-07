// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package config

import (
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/disk"

	"github.com/syncthing/syncthing/lib/build"
	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/util"
)

var (
	ErrPathNotDirectory = errors.New("folder path not a directory")
	ErrPathMissing      = errors.New("folder path missing")
	ErrMarkerMissing    = errors.New("folder marker missing (this indicates potential data loss, search docs/forum to get information about how to proceed)")
)

const (
	DefaultMarkerName          = ".stfolder"
	EncryptionTokenName        = "syncthing-encryption_password_token"
	maxConcurrentWritesDefault = 2
	maxConcurrentWritesLimit   = 64
)

func (f FolderConfiguration) Copy() FolderConfiguration {
	c := f
	c.Devices = make([]FolderDeviceConfiguration, len(f.Devices))
	copy(c.Devices, f.Devices)
	c.Versioning = f.Versioning.Copy()
	return c
}

// Filesystem creates a filesystem for the path and options of this folder.
// The fset parameter may be nil, in which case no mtime handling on top of
// the filesystem is provided.
func (f FolderConfiguration) Filesystem(fset *db.FileSet) fs.Filesystem {
	// This is intentionally not a pointer method, because things like
	// cfg.Folders["default"].Filesystem(nil) should be valid.
	opts := make([]fs.Option, 0, 3)
	if f.FilesystemType == fs.FilesystemTypeBasic && f.JunctionsAsDirs {
		opts = append(opts, new(fs.OptionJunctionsAsDirs))
	}
	if !f.CaseSensitiveFS {
		opts = append(opts, new(fs.OptionDetectCaseConflicts))
	}
	if fset != nil {
		opts = append(opts, fset.MtimeOption())
	}
	return fs.NewFilesystem(f.FilesystemType, f.Path, opts...)
}

func (f FolderConfiguration) ModTimeWindow() time.Duration {
	dur := time.Duration(f.RawModTimeWindowS) * time.Second
	if f.RawModTimeWindowS < 1 && build.IsAndroid {
		if usage, err := disk.Usage(f.Filesystem(nil).URI()); err != nil {
			dur = 2 * time.Second
			l.Debugf(`Detecting FS at "%v" on android: Setting mtime window to 2s: err == "%v"`, f.Path, err)
		} else if strings.HasPrefix(strings.ToLower(usage.Fstype), "ext2") || strings.HasPrefix(strings.ToLower(usage.Fstype), "ext3") || strings.HasPrefix(strings.ToLower(usage.Fstype), "ext4") {
			l.Debugf(`Detecting FS at %v on android: Leaving mtime window at 0: usage.Fstype == "%v"`, f.Path, usage.Fstype)
		} else {
			dur = 2 * time.Second
			l.Debugf(`Detecting FS at "%v" on android: Setting mtime window to 2s: usage.Fstype == "%v"`, f.Path, usage.Fstype)
		}
	}
	return dur
}

func (f *FolderConfiguration) CreateMarker() error {
	if err := f.CheckPath(); err != ErrMarkerMissing {
		return err
	}
	if f.MarkerName != DefaultMarkerName {
		// Folder uses a non-default marker so we shouldn't mess with it.
		// Pretend we created it and let the subsequent health checks sort
		// out the actual situation.
		return nil
	}

	permBits := fs.FileMode(0777)
	if build.IsWindows {
		// Windows has no umask so we must chose a safer set of bits to
		// begin with.
		permBits = 0700
	}
	fs := f.Filesystem(nil)
	err := fs.Mkdir(DefaultMarkerName, permBits)
	if err != nil {
		return err
	}
	if dir, err := fs.Open("."); err != nil {
		l.Debugln("folder marker: open . failed:", err)
	} else if err := dir.Sync(); err != nil {
		l.Debugln("folder marker: fsync . failed:", err)
	}
	fs.Hide(DefaultMarkerName)

	return nil
}

// CheckPath returns nil if the folder root exists and contains the marker file
func (f *FolderConfiguration) CheckPath() error {
	fi, err := f.Filesystem(nil).Stat(".")
	if err != nil {
		if !fs.IsNotExist(err) {
			return err
		}
		return ErrPathMissing
	}

	// Users might have the root directory as a symlink or reparse point.
	// Furthermore, OneDrive bullcrap uses a magic reparse point to the cloudz...
	// Yet it's impossible for this to happen, as filesystem adds a trailing
	// path separator to the root, so even if you point the filesystem at a file
	// Stat ends up calling stat on C:\dir\file\ which, fails with "is not a directory"
	// in the error check above, and we don't even get to here.
	if !fi.IsDir() && !fi.IsSymlink() {
		return ErrPathNotDirectory
	}

	_, err = f.Filesystem(nil).Stat(f.MarkerName)
	if err != nil {
		if !fs.IsNotExist(err) {
			return err
		}
		return ErrMarkerMissing
	}

	return nil
}

func (f *FolderConfiguration) CreateRoot() (err error) {
	// Directory permission bits. Will be filtered down to something
	// sane by umask on Unixes.
	permBits := fs.FileMode(0777)
	if build.IsWindows {
		// Windows has no umask so we must chose a safer set of bits to
		// begin with.
		permBits = 0700
	}

	filesystem := f.Filesystem(nil)

	if _, err = filesystem.Stat("."); fs.IsNotExist(err) {
		err = filesystem.MkdirAll(".", permBits)
	}

	return err
}

func (f FolderConfiguration) Description() string {
	if f.Label == "" {
		return f.ID
	}
	return fmt.Sprintf("%q (%s)", f.Label, f.ID)
}

func (f *FolderConfiguration) DeviceIDs() []protocol.DeviceID {
	deviceIDs := make([]protocol.DeviceID, len(f.Devices))
	for i, n := range f.Devices {
		deviceIDs[i] = n.DeviceID
	}
	return deviceIDs
}

func (f *FolderConfiguration) prepare(myID protocol.DeviceID, existingDevices map[protocol.DeviceID]bool) {
	// Ensure that
	// - any loose devices are not present in the wrong places
	// - there are no duplicate devices
	// - we are part of the devices
	f.Devices = ensureExistingDevices(f.Devices, existingDevices)
	f.Devices = ensureNoDuplicateFolderDevices(f.Devices)
	f.Devices = ensureDevicePresent(f.Devices, myID)

	sort.Slice(f.Devices, func(a, b int) bool {
		return f.Devices[a].DeviceID.Compare(f.Devices[b].DeviceID) == -1
	})

	if f.RescanIntervalS > MaxRescanIntervalS {
		f.RescanIntervalS = MaxRescanIntervalS
	} else if f.RescanIntervalS < 0 {
		f.RescanIntervalS = 0
	}

	if f.FSWatcherDelayS <= 0 {
		f.FSWatcherEnabled = false
		f.FSWatcherDelayS = 10
	}

	if f.Versioning.CleanupIntervalS > MaxRescanIntervalS {
		f.Versioning.CleanupIntervalS = MaxRescanIntervalS
	} else if f.Versioning.CleanupIntervalS < 0 {
		f.Versioning.CleanupIntervalS = 0
	}

	if f.WeakHashThresholdPct == 0 {
		f.WeakHashThresholdPct = 25
	}

	if f.MarkerName == "" {
		f.MarkerName = DefaultMarkerName
	}

	if f.MaxConcurrentWrites <= 0 {
		f.MaxConcurrentWrites = maxConcurrentWritesDefault
	} else if f.MaxConcurrentWrites > maxConcurrentWritesLimit {
		f.MaxConcurrentWrites = maxConcurrentWritesLimit
	}

	if f.Type == FolderTypeReceiveEncrypted {
		f.DisableTempIndexes = true
		f.IgnorePerms = true
	}
}

// RequiresRestartOnly returns a copy with only the attributes that require
// restart on change.
func (f FolderConfiguration) RequiresRestartOnly() FolderConfiguration {
	copy := f

	// Manual handling for things that are not taken care of by the tag
	// copier, yet should not cause a restart.

	blank := FolderConfiguration{}
	util.CopyMatchingTag(&blank, &copy, "restart", func(v string) bool {
		if len(v) > 0 && v != "false" {
			panic(fmt.Sprintf(`unexpected tag value: %s. expected untagged or "false"`, v))
		}
		return v == "false"
	})
	return copy
}

func (f *FolderConfiguration) Device(device protocol.DeviceID) (FolderDeviceConfiguration, bool) {
	for _, dev := range f.Devices {
		if dev.DeviceID == device {
			return dev, true
		}
	}
	return FolderDeviceConfiguration{}, false
}

func (f *FolderConfiguration) SharedWith(device protocol.DeviceID) bool {
	_, ok := f.Device(device)
	return ok
}

func (f *FolderConfiguration) CheckAvailableSpace(req uint64) error {
	val := f.MinDiskFree.BaseValue()
	if val <= 0 {
		return nil
	}
	fs := f.Filesystem(nil)
	usage, err := fs.Usage(".")
	if err != nil {
		return nil
	}
	if err := checkAvailableSpace(req, f.MinDiskFree, usage); err != nil {
		return fmt.Errorf("insufficient space in folder %v (%v): %w", f.Description(), fs.URI(), err)
	}
	return nil
}

func (f XattrFilter) Permit(s string) bool {
	if len(f.Entries) == 0 {
		return true
	}

	for _, entry := range f.Entries {
		if ok, _ := path.Match(entry.Match, s); ok {
			return entry.Permit
		}
	}
	return false
}

func (f XattrFilter) GetMaxSingleEntrySize() int {
	return f.MaxSingleEntrySize
}

func (f XattrFilter) GetMaxTotalSize() int {
	return f.MaxTotalSize
}
