// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package protocol

import (
	"fmt"

	"github.com/syncthing/syncthing/internal/gen/bep"
)

type Compression = bep.Compression

const (
	CompressionMetadata = bep.Compression_COMPRESSION_METADATA
	CompressionNever    = bep.Compression_COMPRESSION_NEVER
	CompressionAlways   = bep.Compression_COMPRESSION_ALWAYS
)

type ClusterConfig struct {
	Folders   []Folder
	Secondary bool
}

func (c *ClusterConfig) toWire() *bep.ClusterConfig {
	folders := make([]*bep.Folder, len(c.Folders))
	for i, f := range c.Folders {
		folders[i] = f.toWire()
	}
	return &bep.ClusterConfig{
		Folders:   folders,
		Secondary: c.Secondary,
	}
}

func clusterConfigFromWire(w *bep.ClusterConfig) *ClusterConfig {
	if w == nil {
		return nil
	}
	c := &ClusterConfig{
		Secondary: w.Secondary,
	}
	c.Folders = make([]Folder, len(w.Folders))
	for i, f := range w.Folders {
		c.Folders[i] = folderFromWire(f)
	}
	return c
}

type Folder struct {
	ID                 string
	Label              string
	ReadOnly           bool
	IgnorePermissions  bool
	IgnoreDelete       bool
	DisableTempIndexes bool
	Paused             bool
	Devices            []Device
}

func (f *Folder) toWire() *bep.Folder {
	devices := make([]*bep.Device, len(f.Devices))
	for i, d := range f.Devices {
		devices[i] = d.toWire()
	}
	return &bep.Folder{
		Id:                 f.ID,
		Label:              f.Label,
		ReadOnly:           f.ReadOnly,
		IgnorePermissions:  f.IgnorePermissions,
		IgnoreDelete:       f.IgnoreDelete,
		DisableTempIndexes: f.DisableTempIndexes,
		Paused:             f.Paused,
		Devices:            devices,
	}
}

func folderFromWire(w *bep.Folder) Folder {
	devices := make([]Device, len(w.Devices))
	for i, d := range w.Devices {
		devices[i] = deviceFromWire(d)
	}
	return Folder{
		ID:                 w.Id,
		Label:              w.Label,
		ReadOnly:           w.ReadOnly,
		IgnorePermissions:  w.IgnorePermissions,
		IgnoreDelete:       w.IgnoreDelete,
		DisableTempIndexes: w.DisableTempIndexes,
		Paused:             w.Paused,
		Devices:            devices,
	}
}

func (f Folder) Description() string {
	// used by logging stuff
	if f.Label == "" {
		return f.ID
	}
	return fmt.Sprintf("%q (%s)", f.Label, f.ID)
}

type Device struct {
	ID                       DeviceID
	Name                     string
	Addresses                []string
	Compression              Compression
	CertName                 string
	MaxSequence              int64
	Introducer               bool
	IndexID                  IndexID
	SkipIntroductionRemovals bool
	EncryptionPasswordToken  []byte
}

func (d *Device) toWire() *bep.Device {
	return &bep.Device{
		Id:                       d.ID[:],
		Name:                     d.Name,
		Addresses:                d.Addresses,
		Compression:              d.Compression,
		CertName:                 d.CertName,
		MaxSequence:              d.MaxSequence,
		Introducer:               d.Introducer,
		IndexId:                  uint64(d.IndexID),
		SkipIntroductionRemovals: d.SkipIntroductionRemovals,
		EncryptionPasswordToken:  d.EncryptionPasswordToken,
	}
}

func deviceFromWire(w *bep.Device) Device {
	return Device{
		ID:                       DeviceID(w.Id),
		Name:                     w.Name,
		Addresses:                w.Addresses,
		Compression:              w.Compression,
		CertName:                 w.CertName,
		MaxSequence:              w.MaxSequence,
		Introducer:               w.Introducer,
		IndexID:                  IndexID(w.IndexId),
		SkipIntroductionRemovals: w.SkipIntroductionRemovals,
		EncryptionPasswordToken:  w.EncryptionPasswordToken,
	}
}
