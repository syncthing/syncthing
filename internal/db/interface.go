// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package db

import (
	"iter"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/thejerf/suture/v4"
)

type DBService interface {
	suture.Service

	// Starts maintenance asynchronously, if not already running
	StartMaintenance()
}

type DB interface {
	// Create a service that performs database maintenance periodically (no
	// more often than the requested interval)
	Service(maintenanceInterval time.Duration) DBService

	// Basics
	Update(folder string, device protocol.DeviceID, fs []protocol.FileInfo) error
	Close() error

	// Single files
	GetDeviceFile(folder string, device protocol.DeviceID, file string) (protocol.FileInfo, bool, error)
	GetGlobalAvailability(folder, file string) ([]protocol.DeviceID, error)
	GetGlobalFile(folder string, file string) (protocol.FileInfo, bool, error)

	// File iterators
	//
	// n.b. there is a slight inconsistency in the return types where some
	// return a FileInfo iterator and some a FileMetadata iterator. The
	// latter is more lightweight, and the discrepancy depends on how the
	// functions tend to be used. We can introduce more variations as
	// required.
	AllGlobalFiles(folder string) (iter.Seq[FileMetadata], func() error)
	AllGlobalFilesPrefix(folder string, prefix string) (iter.Seq[FileMetadata], func() error)
	AllLocalFiles(folder string, device protocol.DeviceID) (iter.Seq[protocol.FileInfo], func() error)
	AllLocalFilesBySequence(folder string, device protocol.DeviceID, startSeq int64, limit int) (iter.Seq[protocol.FileInfo], func() error)
	AllLocalFilesWithPrefix(folder string, device protocol.DeviceID, prefix string) (iter.Seq[protocol.FileInfo], func() error)
	AllLocalFilesWithBlocksHash(folder string, h []byte) (iter.Seq[FileMetadata], func() error)
	AllNeededGlobalFiles(folder string, device protocol.DeviceID, order config.PullOrder, limit, offset int) (iter.Seq[protocol.FileInfo], func() error)
	AllLocalBlocksWithHash(folder string, hash []byte) (iter.Seq[BlockMapEntry], func() error)

	// Cleanup
	DropAllFiles(folder string, device protocol.DeviceID) error
	DropDevice(device protocol.DeviceID) error
	DropFilesNamed(folder string, device protocol.DeviceID, names []string) error
	DropFolder(folder string) error

	// Various metadata
	GetDeviceSequence(folder string, device protocol.DeviceID) (int64, error)
	ListFolders() ([]string, error)
	ListDevicesForFolder(folder string) ([]protocol.DeviceID, error)
	RemoteSequences(folder string) (map[protocol.DeviceID]int64, error)

	// Counts
	CountGlobal(folder string) (Counts, error)
	CountLocal(folder string, device protocol.DeviceID) (Counts, error)
	CountNeed(folder string, device protocol.DeviceID) (Counts, error)
	CountReceiveOnlyChanged(folder string) (Counts, error)

	// Index IDs
	DropAllIndexIDs() error
	GetIndexID(folder string, device protocol.DeviceID) (protocol.IndexID, error)
	SetIndexID(folder string, device protocol.DeviceID, id protocol.IndexID) error

	// MtimeFS
	DeleteMtime(folder, name string) error
	GetMtime(folder, name string) (ondisk, virtual time.Time)
	PutMtime(folder, name string, ondisk, virtual time.Time) error

	KV
}

// Generic KV store
type KV interface {
	GetKV(key string) ([]byte, error)
	PutKV(key string, val []byte) error
	DeleteKV(key string) error
	PrefixKV(prefix string) (iter.Seq[KeyValue], func() error)
}

type BlockMapEntry struct {
	BlocklistHash []byte
	Offset        int64
	BlockIndex    int
	Size          int
	FileName      string
}

type KeyValue struct {
	Key   string
	Value []byte
}

type FileMetadata struct {
	Name       string
	Sequence   int64
	ModNanos   int64
	Size       int64
	LocalFlags protocol.FlagLocal
	Type       protocol.FileInfoType
	Deleted    bool
}

func (f *FileMetadata) ModTime() time.Time {
	return time.Unix(0, f.ModNanos)
}

func (f *FileMetadata) IsReceiveOnlyChanged() bool {
	return f.LocalFlags&protocol.FlagLocalReceiveOnly != 0
}

func (f *FileMetadata) IsDirectory() bool {
	return f.Type == protocol.FileInfoTypeDirectory
}

func (f *FileMetadata) ShouldConflict() bool {
	return f.LocalFlags&protocol.LocalConflictFlags != 0
}

func (f *FileMetadata) IsInvalid() bool {
	return f.LocalFlags.IsInvalid()
}
