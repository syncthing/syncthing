// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package config

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/disk"

	"github.com/syncthing/syncthing/lib/build"
	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/ext"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/sync"
)

var (
	ErrPathNotDirectory = errors.New("folder path not a directory")
	ErrPathMissing      = errors.New("folder path missing")
	ErrMarkerMissing    = errors.New("folder marker missing (this indicates potential data loss, search docs/forum to get information about how to proceed)")
	ErrExceedsFree      = errors.New("sync would exceed free usage allowance")
	ErrDisabled         = errors.New("sync has been disabled")
)

var (
	externallyDisabledMut = sync.NewMutex()
	ExternallyDisabled    = os.Getenv("STEXTDISABLED") != ""
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
	path := f.Path
	if build.IsIOS {
		path = ext.Callback.ExtAccessPath(f.Path)
	}
	return fs.NewFilesystem(f.FilesystemType, path, opts...)
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

	ffs := f.Filesystem(nil)

	// Create the marker as a directory
	err := ffs.Mkdir(DefaultMarkerName, 0o755)
	if err != nil {
		return err
	}

	// Create a file inside it, reducing the risk of the marker directory
	// being removed by automated cleanup tools.
	markerFile := filepath.Join(DefaultMarkerName, f.markerFilename())
	if err := fs.WriteFile(ffs, markerFile, f.markerContents(), 0o644); err != nil {
		return err
	}

	// Sync & hide the containing directory
	if dir, err := ffs.Open("."); err != nil {
		l.Debugln("folder marker: open . failed:", err)
	} else if err := dir.Sync(); err != nil {
		l.Debugln("folder marker: fsync . failed:", err)
	}
	ffs.Hide(DefaultMarkerName)

	return nil
}

func (f *FolderConfiguration) RemoveMarker() error {
	ffs := f.Filesystem(nil)
	_ = ffs.Remove(filepath.Join(DefaultMarkerName, f.markerFilename()))
	return ffs.Remove(DefaultMarkerName)
}

func (f *FolderConfiguration) markerFilename() string {
	h := sha256.Sum256([]byte(f.ID))
	return fmt.Sprintf("syncthing-folder-%x.txt", h[:3])
}

func (f *FolderConfiguration) markerContents() []byte {
	var buf bytes.Buffer
	buf.WriteString("# This directory is a Syncthing folder marker.\n# Do not delete.\n\n")
	fmt.Fprintf(&buf, "folderID: %s\n", f.ID)
	fmt.Fprintf(&buf, "created: %s\n", time.Now().Format(time.RFC3339))
	return buf.Bytes()
}

// CheckPath returns nil if the folder root exists and contains the marker file
func (f *FolderConfiguration) CheckPath() error {
	return f.checkFilesystemPath(f.Filesystem(nil), ".")
}

func (f *FolderConfiguration) checkFilesystemPath(ffs fs.Filesystem, path string) error {
	fi, err := ffs.Stat(path)
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

	_, err = ffs.Stat(filepath.Join(path, f.MarkerName))
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
	permBits := fs.FileMode(0o777)
	if build.IsWindows {
		// Windows has no umask so we must chose a safer set of bits to
		// begin with.
		permBits = 0o700
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

func (f *FolderConfiguration) prepare(myID protocol.DeviceID, existingDevices map[protocol.DeviceID]*DeviceConfiguration) {
	// Ensure that
	// - any loose devices are not present in the wrong places
	// - there are no duplicate devices
	// - we are part of the devices
	// - folder is not shared in trusted mode with an untrusted device
	f.Devices = ensureExistingDevices(f.Devices, existingDevices)
	f.Devices = ensureNoDuplicateFolderDevices(f.Devices)
	f.Devices = ensureDevicePresent(f.Devices, myID)
	f.Devices = ensureNoUntrustedTrustingSharing(f, f.Devices, existingDevices)

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
	} else if f.FSWatcherDelayS < 0.01 {
		f.FSWatcherDelayS = 0.01
	}

	if build.IsIOS {
		f.FSWatcherEnabled = false
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
	copyMatchingTag(&blank, &copy, "restart", func(v string) bool {
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

func SetExternallyDisabled(isDisabled bool) {
	externallyDisabledMut.Lock()
	ExternallyDisabled = isDisabled
	externallyDisabledMut.Unlock()
}

func (f *FolderConfiguration) CheckAvailableSpace(req uint64) error {
	externallyDisabledMut.Lock()
	disabled := ExternallyDisabled
	externallyDisabledMut.Unlock()
	if disabled {
		return ErrDisabled
	}

	if ext.Callback == nil || !ext.Callback.ExtCheckAvailableSpace(req) {
		return ErrExceedsFree
	}

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
