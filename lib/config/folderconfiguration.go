// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/protocol"
)

type FolderConfiguration struct {
	ID                    string                      `xml:"id,attr" json:"id"`
	Label                 string                      `xml:"label,attr" json:"label"`
	RawPath               string                      `xml:"path,attr" json:"path"`
	Type                  FolderType                  `xml:"type,attr" json:"type"`
	Devices               []FolderDeviceConfiguration `xml:"device" json:"devices"`
	RescanIntervalS       int                         `xml:"rescanIntervalS,attr" json:"rescanIntervalS"`
	IgnorePerms           bool                        `xml:"ignorePerms,attr" json:"ignorePerms"`
	AutoNormalize         bool                        `xml:"autoNormalize,attr" json:"autoNormalize"`
	MinDiskFreePct        float64                     `xml:"minDiskFreePct" json:"minDiskFreePct"`
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

	Invalid    string `xml:"-" json:"invalid"` // Set at runtime when there is an error, not saved
	cachedPath string

	DeprecatedReadOnly bool `xml:"ro,attr,omitempty" json:"-"`
}

type FolderDeviceConfiguration struct {
	DeviceID protocol.DeviceID `xml:"id,attr" json:"deviceID"`
}

func NewFolderConfiguration(id, path string) FolderConfiguration {
	f := FolderConfiguration{
		ID:      id,
		RawPath: path,
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

func (f FolderConfiguration) Path() string {
	// This is intentionally not a pointer method, because things like
	// cfg.Folders["default"].Path() should be valid.

	if f.cachedPath == "" {
		l.Infoln("bug: uncached path call (should only happen in tests)")
		return f.cleanedPath()
	}
	return f.cachedPath
}

func (f *FolderConfiguration) CreateMarker() error {
	if !f.HasMarker() {
		marker := filepath.Join(f.Path(), ".stfolder")
		fd, err := os.Create(marker)
		if err != nil {
			return err
		}
		fd.Close()
		osutil.HideFile(marker)
	}

	return nil
}

func (f *FolderConfiguration) HasMarker() bool {
	_, err := os.Stat(filepath.Join(f.Path(), ".stfolder"))
	if err != nil {
		return false
	}
	return true
}

func (f *FolderConfiguration) DeviceIDs() []protocol.DeviceID {
	deviceIDs := make([]protocol.DeviceID, len(f.Devices))
	for i, n := range f.Devices {
		deviceIDs[i] = n.DeviceID
	}
	return deviceIDs
}

func (f *FolderConfiguration) prepare() {
	if len(f.RawPath) == 0 {
		f.Invalid = "no directory configured"
		return
	}

	// The reason it's done like this:
	// C:          ->  C:\            ->  C:\        (issue that this is trying to fix)
	// C:\somedir  ->  C:\somedir\    ->  C:\somedir
	// C:\somedir\ ->  C:\somedir\\   ->  C:\somedir
	// This way in the tests, we get away without OS specific separators
	// in the test configs.
	f.RawPath = filepath.Dir(f.RawPath + string(filepath.Separator))

	// If we're not on Windows, we want the path to end with a slash to
	// penetrate symlinks. On Windows, paths must not end with a slash.
	if runtime.GOOS != "windows" && f.RawPath[len(f.RawPath)-1] != filepath.Separator {
		f.RawPath = f.RawPath + string(filepath.Separator)
	}

	f.cachedPath = f.cleanedPath()

	if f.ID == "" {
		f.ID = "default"
	}

	if f.RescanIntervalS > MaxRescanIntervalS {
		f.RescanIntervalS = MaxRescanIntervalS
	} else if f.RescanIntervalS < 0 {
		f.RescanIntervalS = 0
	}

	if f.Versioning.Params == nil {
		f.Versioning.Params = make(map[string]string)
	}
}

func (f *FolderConfiguration) cleanedPath() string {
	cleaned := f.RawPath

	// Attempt tilde expansion; leave unchanged in case of error
	if path, err := osutil.ExpandTilde(cleaned); err == nil {
		cleaned = path
	}

	// Attempt absolutification; leave unchanged in case of error
	if !filepath.IsAbs(cleaned) {
		// Abs() looks like a fairly expensive syscall on Windows, while
		// IsAbs() is a whole bunch of string mangling. I think IsAbs() may be
		// somewhat faster in the general case, hence the outer if...
		if path, err := filepath.Abs(cleaned); err == nil {
			cleaned = path
		}
	}

	// Attempt to enable long filename support on Windows. We may still not
	// have an absolute path here if the previous steps failed.
	if runtime.GOOS == "windows" && filepath.IsAbs(cleaned) && !strings.HasPrefix(f.RawPath, `\\`) {
		return `\\?\` + cleaned
	}

	return cleaned
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
