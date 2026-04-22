// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// Package versioner implements common interfaces for file versioning and a
// simple default versioning scheme.
package versioner

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/syncthing/syncthing/lib/config"
)

type Versioner interface {
	Archive(filePath string) error
	GetVersions() (map[string][]FileVersion, error)
	Restore(filePath string, versionTime time.Time) error
	Clean(context.Context) error
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
		return nil, fmt.Errorf("requested versioning type %q does not exist", cfg.Versioning.Type)
	}

	return &versionerWithErrorContext{
		Versioner: fac(cfg),
		vtype:     cfg.Versioning.Type,
	}, nil
}

type versionerWithErrorContext struct {
	Versioner

	vtype string
}

func (v *versionerWithErrorContext) wrapError(err error, op string) error {
	if err != nil {
		return fmt.Errorf("%s versioner: %v: %w", v.vtype, op, err)
	}
	return nil
}

func (v *versionerWithErrorContext) Archive(filePath string) error {
	return v.wrapError(v.Versioner.Archive(filePath), "archive")
}

func (v *versionerWithErrorContext) GetVersions() (map[string][]FileVersion, error) {
	versions, err := v.Versioner.GetVersions()
	return versions, v.wrapError(err, "get versions")
}

func (v *versionerWithErrorContext) Restore(filePath string, versionTime time.Time) error {
	return v.wrapError(v.Versioner.Restore(filePath, versionTime), "restore")
}

func (v *versionerWithErrorContext) Clean(ctx context.Context) error {
	return v.wrapError(v.Versioner.Clean(ctx), "clean")
}
