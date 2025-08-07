// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package protocol

import (
	"fmt"
	"log/slog"

	"github.com/syncthing/syncthing/internal/gen/bep"
)

type Compression bep.Compression

const (
	CompressionMetadata = Compression(bep.Compression_COMPRESSION_METADATA)
	CompressionNever    = Compression(bep.Compression_COMPRESSION_NEVER)
	CompressionAlways   = Compression(bep.Compression_COMPRESSION_ALWAYS)
)

type FolderType bep.FolderType

const (
	FolderTypeSendReceive      = FolderType(bep.FolderType_FOLDER_TYPE_SEND_RECEIVE)
	FolderTypeSendOnly         = FolderType(bep.FolderType_FOLDER_TYPE_SEND_ONLY)
	FolderTypeReceiveOnly      = FolderType(bep.FolderType_FOLDER_TYPE_RECEIVE_ONLY)
	FolderTypeReceiveEncrypted = FolderType(bep.FolderType_FOLDER_TYPE_RECEIVE_ENCRYPTED)
)

type FolderStopReason bep.FolderStopReason

const (
	FolderStopReasonRunning = FolderStopReason(bep.FolderStopReason_FOLDER_STOP_REASON_RUNNING)
	FolderStopReasonPaused  = FolderStopReason(bep.FolderStopReason_FOLDER_STOP_REASON_PAUSED)
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
	ID         string
	Label      string
	Type       FolderType
	StopReason FolderStopReason
	Devices    []Device
}

func (f *Folder) toWire() *bep.Folder {
	devices := make([]*bep.Device, len(f.Devices))
	for i, d := range f.Devices {
		devices[i] = d.toWire()
	}
	return &bep.Folder{
		Id:         f.ID,
		Label:      f.Label,
		Type:       bep.FolderType(f.Type),
		StopReason: bep.FolderStopReason(f.StopReason),
		Devices:    devices,
	}
}

func folderFromWire(w *bep.Folder) Folder {
	devices := make([]Device, len(w.Devices))
	for i, d := range w.Devices {
		devices[i] = deviceFromWire(d)
	}
	return Folder{
		ID:         w.Id,
		Label:      w.Label,
		Type:       FolderType(w.Type),
		StopReason: FolderStopReason(w.StopReason),
		Devices:    devices,
	}
}

func (f Folder) Description() string {
	// used by logging stuff
	if f.Label == "" {
		return f.ID
	}
	return fmt.Sprintf("%q (%s)", f.Label, f.ID)
}

func (f Folder) LogAttr() slog.Attr {
	if f.Label == "" || f.Label == f.ID {
		return slog.Group("folder", slog.String("id", f.ID))
	}
	return slog.Group("folder", slog.String("label", f.Label), slog.String("id", f.ID))
}

func (f Folder) IsRunning() bool {
	switch f.StopReason {
	case FolderStopReasonPaused:
		return false
	default:
		return true
	}
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
		Compression:              bep.Compression(d.Compression),
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
		Compression:              Compression(w.Compression),
		CertName:                 w.CertName,
		MaxSequence:              w.MaxSequence,
		Introducer:               w.Introducer,
		IndexID:                  IndexID(w.IndexId),
		SkipIntroductionRemovals: w.SkipIntroductionRemovals,
		EncryptionPasswordToken:  w.EncryptionPasswordToken,
	}
}
