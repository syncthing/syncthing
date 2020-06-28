// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// Package versioner implements common interfaces for file versioning and a
// simple default versioning scheme.
package versioner

import (
	"errors"
	"fmt"
	"time"

	"github.com/syncthing/syncthing/lib/config"
)

type Versioner interface {
	Archive(filePath string) error
	GetVersions() (map[string][]FileVersion, error)
	Restore(filePath string, versionTime time.Time) error
}

type FileVersion struct {
	VersionTime time.Time `json:"versionTime"`
	ModTime     time.Time `json:"modTime"`
	Size        int64     `json:"size"`
}

type factory func(cfg config.FolderConfiguration) Versioner

var factories = make(map[string]factory)

var ErrRestorationNotSupported = errors.New("version restoration not supported with the current versioner")

const (
	TimeFormat = "20060102-150405"
	timeGlob   = "[0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9]-[0-9][0-9][0-9][0-9][0-9][0-9]" // glob pattern matching TimeFormat
)

func New(cfg config.FolderConfiguration) (Versioner, error) {
	fac, ok := factories[cfg.Versioning.Type]
	if !ok {
		return nil, fmt.Errorf("requested versioning type %q does not exist", cfg.Type)
	}

	return fac(cfg), nil
}
