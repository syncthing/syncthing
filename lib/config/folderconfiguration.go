// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package config

import (
	"fmt"
	"runtime"

	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/protocol"
)

type FolderConfiguration struct {
	ID                    string                      `xml:"id,attr" json:"id"`
	Label                 string                      `xml:"label,attr" json:"label"`
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
	Pullers               int                         `xml:"pullers" json:"pullers"` // Defines how many blocks are fetched at the same time, possibly between separate copier routines.
	Hashers               int                         `xml:"hashers" json:"hashers"` // Less than one sets the value to the number of cores. These are CPU bound due to hashing.
	Order                 PullOrder                   `xml:"order" json:"order"`
	IgnoreDelete          bool                        `xml:"ignoreDelete" json:"ignoreDelete"`
	ScanProgressIntervalS int                         `xml:"scanProgressIntervalS" json:"scanProgressIntervalS"` // Set to a negative value to disable. Value of 0 will get replaced with value of 2 (default value)
	PullerSleepS          int                         `xml:"pullerSleepS" json:"pullerSleepS"`
	PullerPauseS          int                         `xml:"pullerPauseS" json:"pullerPauseS"`
	MaxConflicts          int                         `xml:"maxConflicts" json:"maxConflicts"`
	DisableSparseFiles    bool                        `xml:"disableSparseFiles" json:"disableSparseFiles"`
	DisableTempIndexes    bool                        `xml:"disableTempIndexes" json:"disableTempIndexes"`
	Paused                bool                        `xml:"paused" json:"paused"`
	WeakHashThresholdPct  int                         `xml:"weakHashThresholdPct" json:"weakHashThresholdPct"` // Use weak hash if more than X percent of the file has changed. Set to -1 to always use weak hash.

	cachedFilesystem fs.Filesystem

	DeprecatedReadOnly       bool    `xml:"ro,attr,omitempty" json:"-"`
	DeprecatedMinDiskFreePct float64 `xml:"minDiskFreePct,omitempty" json:"-"`
}

type FolderDeviceConfiguration struct {
	DeviceID     protocol.DeviceID `xml:"id,attr" json:"deviceID"`
	IntroducedBy protocol.DeviceID `xml:"introducedBy,attr" json:"introducedBy"`
}

func NewFolderConfiguration(id string, fsType fs.FilesystemType, path string) FolderConfiguration {
	f := FolderConfiguration{
		ID:             id,
		FilesystemType: fsType,
		Path:           path,
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

func (f *FolderConfiguration) CreateMarker() error {
	if !f.HasMarker() {
		permBits := fs.FileMode(0777)
		if runtime.GOOS == "windows" {
			// Windows has no umask so we must chose a safer set of bits to
			// begin with.
			permBits = 0700
		}
		fs := f.Filesystem()
		err := fs.Mkdir(".stfolder", permBits)
		if err != nil {
			return err
		}
		if dir, err := fs.Open("."); err != nil {
			l.Debugln("folder marker: open . failed:", err)
		} else if err := dir.Sync(); err != nil {
			l.Debugln("folder marker: fsync . failed:", err)
		}
		fs.Hide(".stfolder")
	}

	return nil
}

func (f *FolderConfiguration) HasMarker() bool {
	_, err := f.Filesystem().Stat(".stfolder")
	return err == nil
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
