// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package config

import (
	"errors"
	"fmt"
	"runtime"

	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/util"
	"github.com/syncthing/syncthing/lib/versioner"
)

var (
	ErrPathNotDirectory = errors.New("folder path not a directory")
	ErrPathMissing      = errors.New("folder path missing")
	ErrMarkerMissing    = errors.New("folder marker missing")
)

const DefaultMarkerName = ".stfolder"

type FolderConfiguration struct {
	ID                    string                      `xml:"id,attr" json:"id"`
	Label                 string                      `xml:"label,attr" json:"label" restart:"false"`
	FilesystemType        fs.FilesystemType           `xml:"filesystemType" json:"filesystemType"`
	Path                  string                      `xml:"path,attr" json:"path"`
	Type                  FolderType                  `xml:"type,attr" json:"type"`
	Devices               []FolderDeviceConfiguration `xml:"device" json:"devices"`
	RescanIntervalS       int                         `xml:"rescanIntervalS,attr" json:"rescanIntervalS"`
	FSWatcherEnabled      bool                        `xml:"fsWatcherEnabled,attr" json:"fsWatcherEnabled"`
	FSWatcherDelayS       int                         `xml:"fsWatcherDelayS,attr" json:"fsWatcherDelayS"`
	IgnorePerms           bool                        `xml:"ignorePerms,attr" json:"ignorePerms"`
	AutoNormalize         bool                        `xml:"autoNormalize,attr" json:"autoNormalize"`
	MinDiskFree           Size                        `xml:"minDiskFree" json:"minDiskFree"`
	Versioning            VersioningConfiguration     `xml:"versioning" json:"versioning"`
	Copiers               int                         `xml:"copiers" json:"copiers"` // This defines how many files are handled concurrently.
	PullerMaxPendingKiB   int                         `xml:"pullerMaxPendingKiB" json:"pullerMaxPendingKiB"`
	PullerMaxQueueLength  int                         `xml:"pullerMaxQueueLength" json:"pullerMaxQueueLength"`
	Hashers               int                         `xml:"hashers" json:"hashers"` // Less than one sets the value to the number of cores. These are CPU bound due to hashing.
	Order                 PullOrder                   `xml:"order" json:"order"`
	IgnoreDelete          bool                        `xml:"ignoreDelete" json:"ignoreDelete"`
	ScanProgressIntervalS int                         `xml:"scanProgressIntervalS" json:"scanProgressIntervalS"` // Set to a negative value to disable. Value of 0 will get replaced with value of 2 (default value)
	PullerPauseS          int                         `xml:"pullerPauseS" json:"pullerPauseS"`
	MaxConflicts          int                         `xml:"maxConflicts" json:"maxConflicts"`
	DisableSparseFiles    bool                        `xml:"disableSparseFiles" json:"disableSparseFiles"`
	DisableTempIndexes    bool                        `xml:"disableTempIndexes" json:"disableTempIndexes"`
	Paused                bool                        `xml:"paused" json:"paused"`
	WeakHashThresholdPct  int                         `xml:"weakHashThresholdPct" json:"weakHashThresholdPct"` // Use weak hash if more than X percent of the file has changed. Set to -1 to always use weak hash.
	MarkerName            string                      `xml:"markerName" json:"markerName"`
	UseLargeBlocks        bool                        `xml:"useLargeBlocks" json:"useLargeBlocks"`

	cachedFilesystem fs.Filesystem

	DeprecatedReadOnly       bool    `xml:"ro,attr,omitempty" json:"-"`
	DeprecatedMinDiskFreePct float64 `xml:"minDiskFreePct,omitempty" json:"-"`
	DeprecatedPullers        int     `xml:"pullers,omitempty" json:"-"`
}

type FolderDeviceConfiguration struct {
	DeviceID     protocol.DeviceID `xml:"id,attr" json:"deviceID"`
	IntroducedBy protocol.DeviceID `xml:"introducedBy,attr" json:"introducedBy"`
}

func NewFolderConfiguration(myID protocol.DeviceID, id, label string, fsType fs.FilesystemType, path string) FolderConfiguration {
	f := FolderConfiguration{
		ID:               id,
		Label:            label,
		RescanIntervalS:  3600,
		FSWatcherEnabled: true,
		FSWatcherDelayS:  10,
		MinDiskFree:      Size{Value: 1, Unit: "%"},
		Devices:          []FolderDeviceConfiguration{{DeviceID: myID}},
		AutoNormalize:    true,
		MaxConflicts:     -1,
		FilesystemType:   fsType,
		Path:             path,
	}
	f.prepare()
	return f
}

func (f FolderConfiguration) Copy() FolderConfiguration {
	c := f
	c.Devices = make([]FolderDeviceConfiguration, len(f.Devices))
	copy(c.Devices, f.Devices)
	c.Versioning = f.Versioning.Copy()
	return c
}

func (f FolderConfiguration) Filesystem() fs.Filesystem {
	// This is intentionally not a pointer method, because things like
	// cfg.Folders["default"].Filesystem() should be valid.
	if f.cachedFilesystem == nil && f.Path != "" {
		l.Infoln("bug: uncached filesystem call (should only happen in tests)")
		return fs.NewFilesystem(f.FilesystemType, f.Path)
	}
	return f.cachedFilesystem
}

func (f FolderConfiguration) Versioner() versioner.Versioner {
	if f.Versioning.Type == "" {
		return nil
	}
	versionerFactory, ok := versioner.Factories[f.Versioning.Type]
	if !ok {
		l.Fatalf("Requested versioning type %q that does not exist", f.Versioning.Type)
	}

	return versionerFactory(f.ID, f.Filesystem(), f.Versioning.Params)
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
	if runtime.GOOS == "windows" {
		// Windows has no umask so we must chose a safer set of bits to
		// begin with.
		permBits = 0700
	}
	fs := f.Filesystem()
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
	fi, err := f.Filesystem().Stat(".")
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

	_, err = f.Filesystem().Stat(f.MarkerName)
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
	if runtime.GOOS == "windows" {
		// Windows has no umask so we must chose a safer set of bits to
		// begin with.
		permBits = 0700
	}

	filesystem := f.Filesystem()

	if _, err = filesystem.Stat("."); fs.IsNotExist(err) {
		if err = filesystem.MkdirAll(".", permBits); err != nil {
			l.Warnf("Creating directory for %v: %v", f.Description(), err)
		}
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

func (f *FolderConfiguration) prepare() {
	if f.Path != "" {
		f.cachedFilesystem = fs.NewFilesystem(f.FilesystemType, f.Path)
	}

	if f.RescanIntervalS > MaxRescanIntervalS {
		f.RescanIntervalS = MaxRescanIntervalS
	} else if f.RescanIntervalS < 0 {
		f.RescanIntervalS = 0
	}

	if f.FSWatcherDelayS <= 0 {
		f.FSWatcherEnabled = false
		f.FSWatcherDelayS = 10
	}

	if f.Versioning.Params == nil {
		f.Versioning.Params = make(map[string]string)
	}

	if f.WeakHashThresholdPct == 0 {
		f.WeakHashThresholdPct = 25
	}

	if f.MarkerName == "" {
		f.MarkerName = DefaultMarkerName
	}
}

// RequiresRestartOnly returns a copy with only the attributes that require
// restart on change.
func (f FolderConfiguration) RequiresRestartOnly() FolderConfiguration {
	copy := f

	// Manual handling for things that are not taken care of by the tag
	// copier, yet should not cause a restart.
	copy.cachedFilesystem = nil

	blank := FolderConfiguration{}
	util.CopyMatchingTag(&blank, &copy, "restart", func(v string) bool {
		if len(v) > 0 && v != "false" {
			panic(fmt.Sprintf(`unexpected tag value: %s. expected untagged or "false"`, v))
		}
		return v == "false"
	})
	return copy
}

type FolderDeviceConfigurationList []FolderDeviceConfiguration

func (l FolderDeviceConfigurationList) Less(a, b int) bool {
	return l[a].DeviceID.Compare(l[b].DeviceID) == -1
}

func (l FolderDeviceConfigurationList) Swap(a, b int) {
	l[a], l[b] = l[b], l[a]
}

func (l FolderDeviceConfigurationList) Len() int {
	return len(l)
}

func (f *FolderConfiguration) CheckFreeSpace() (err error) {
	return checkFreeSpace(f.MinDiskFree, f.Filesystem())
}

type FolderConfigurationList []FolderConfiguration

func (l FolderConfigurationList) Len() int {
	return len(l)
}

func (l FolderConfigurationList) Less(a, b int) bool {
	return l[a].ID < l[b].ID
}

func (l FolderConfigurationList) Swap(a, b int) {
	l[a], l[b] = l[b], l[a]
}
