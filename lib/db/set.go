// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// Package db provides a set type to track local/remote files with newness
// checks. We must do a certain amount of normalization in here. We will get
// fed paths with either native or wire-format separators and encodings
// depending on who calls us. We transform paths to wire-format (NFC and
// slashes) on the way to the database, and transform to native format
// (varying separator and encoding) on the way back out.
package db

import (
	"os"
	"time"

	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/sync"
)

type FileSet struct {
	folder   string
	fs       fs.Filesystem
	db       *Instance
	blockmap *BlockMap
	meta     *metadataTracker

	updateMutex sync.Mutex // protects database updates and the corresponding metadata changes
}

// FileIntf is the set of methods implemented by both protocol.FileInfo and
// FileInfoTruncated.
type FileIntf interface {
	FileSize() int64
	FileName() string
	IsDeleted() bool
	IsInvalid() bool
	IsDirectory() bool
	IsSymlink() bool
	HasPermissionBits() bool
	SequenceNo() int64
}

// The Iterator is called with either a protocol.FileInfo or a
// FileInfoTruncated (depending on the method) and returns true to
// continue iteration, false to stop.
type Iterator func(f FileIntf) bool

var databaseRecheckInterval = 30 * 24 * time.Hour

func init() {
	if dur, err := time.ParseDuration(os.Getenv("STRECHECKDBEVERY")); err == nil {
		databaseRecheckInterval = dur
	}
}

func NewFileSet(folder string, fs fs.Filesystem, db *Instance) *FileSet {
	var s = FileSet{
		folder:      folder,
		fs:          fs,
		db:          db,
		blockmap:    NewBlockMap(db, db.folderIdx.ID([]byte(folder))),
		meta:        newMetadataTracker(),
		updateMutex: sync.NewMutex(),
	}

	if err := s.meta.fromDB(db, []byte(folder)); err != nil {
		l.Infof("No stored folder metadata for %q: recalculating", folder)
		s.recalcCounts()
	} else if age := time.Since(s.meta.Created()); age > databaseRecheckInterval {
		l.Infof("Stored folder metadata for %q is %v old; recalculating", folder, age)
		s.recalcCounts()
	}

	return &s
}

func (s *FileSet) recalcCounts() {
	s.meta = newMetadataTracker()

	s.db.checkGlobals([]byte(s.folder), s.meta)

	var deviceID protocol.DeviceID
	s.db.withAllFolderTruncated([]byte(s.folder), func(device []byte, f FileInfoTruncated) bool {
		copy(deviceID[:], device)
		s.meta.addFile(deviceID, f)
		return true
	})

	s.meta.SetCreated()
	s.meta.toDB(s.db, []byte(s.folder))
}

func (s *FileSet) Drop(device protocol.DeviceID) {
	l.Debugf("%s Drop(%v)", s.folder, device)

	s.updateMutex.Lock()
	defer s.updateMutex.Unlock()

	s.db.dropDeviceFolder(device[:], []byte(s.folder), s.meta)

	if device == protocol.LocalDeviceID {
		s.blockmap.Drop()
		s.meta.resetCounts(device)
		// We deliberately do not reset the sequence number here. Dropping
		// all files for the local device ID only happens in testing - which
		// expects the sequence to be retained, like an old Replace() of all
		// files would do. However, if we ever did it "in production" we
		// would anyway want to retain the sequence for delta indexes to be
		// happy.
	} else {
		// Here, on the other hand, we want to make sure that any file
		// announced from the remote is newer than our current sequence
		// number.
		s.meta.resetAll(device)
	}

	s.meta.toDB(s.db, []byte(s.folder))
}

func (s *FileSet) Update(device protocol.DeviceID, fs []protocol.FileInfo) {
	l.Debugf("%s Update(%v, [%d])", s.folder, device, len(fs))
	normalizeFilenames(fs)

	s.updateMutex.Lock()
	defer s.updateMutex.Unlock()

	if device == protocol.LocalDeviceID {
		discards := make([]protocol.FileInfo, 0, len(fs))
		updates := make([]protocol.FileInfo, 0, len(fs))
		// db.UpdateFiles will sort unchanged files out -> save one db lookup
		// filter slice according to https://github.com/golang/go/wiki/SliceTricks#filtering-without-allocating
		oldFs := fs
		fs = fs[:0]
		for _, nf := range oldFs {
			ef, ok := s.db.getFile([]byte(s.folder), device[:], []byte(nf.Name))
			if ok && ef.Version.Equal(nf.Version) && ef.Invalid == nf.Invalid {
				continue
			}

			nf.Sequence = s.meta.nextSeq(protocol.LocalDeviceID)
			fs = append(fs, nf)

			if ok {
				discards = append(discards, ef)
			}
			updates = append(updates, nf)
		}
		s.blockmap.Discard(discards)
		s.blockmap.Update(updates)
	}

	s.db.updateFiles([]byte(s.folder), device[:], fs, s.meta)
	s.meta.toDB(s.db, []byte(s.folder))
}

func (s *FileSet) WithNeed(device protocol.DeviceID, fn Iterator) {
	l.Debugf("%s WithNeed(%v)", s.folder, device)
	s.db.withNeed([]byte(s.folder), device[:], false, false, nativeFileIterator(fn))
}

func (s *FileSet) WithNeedTruncated(device protocol.DeviceID, fn Iterator) {
	l.Debugf("%s WithNeedTruncated(%v)", s.folder, device)
	s.db.withNeed([]byte(s.folder), device[:], true, false, nativeFileIterator(fn))
}

// WithNeedOrInvalid considers all invalid files as needed, regardless of their version
// (e.g. for pulling when ignore patterns changed)
func (s *FileSet) WithNeedOrInvalid(device protocol.DeviceID, fn Iterator) {
	l.Debugf("%s WithNeedExcludingInvalid(%v)", s.folder, device)
	s.db.withNeed([]byte(s.folder), device[:], false, true, nativeFileIterator(fn))
}

func (s *FileSet) WithNeedOrInvalidTruncated(device protocol.DeviceID, fn Iterator) {
	l.Debugf("%s WithNeedExcludingInvalidTruncated(%v)", s.folder, device)
	s.db.withNeed([]byte(s.folder), device[:], true, true, nativeFileIterator(fn))
}

func (s *FileSet) WithHave(device protocol.DeviceID, fn Iterator) {
	l.Debugf("%s WithHave(%v)", s.folder, device)
	s.db.withHave([]byte(s.folder), device[:], nil, false, nativeFileIterator(fn))
}

func (s *FileSet) WithHaveTruncated(device protocol.DeviceID, fn Iterator) {
	l.Debugf("%s WithHaveTruncated(%v)", s.folder, device)
	s.db.withHave([]byte(s.folder), device[:], nil, true, nativeFileIterator(fn))
}

func (s *FileSet) WithPrefixedHaveTruncated(device protocol.DeviceID, prefix string, fn Iterator) {
	l.Debugf("%s WithPrefixedHaveTruncated(%v)", s.folder, device)
	s.db.withHave([]byte(s.folder), device[:], []byte(osutil.NormalizedFilename(prefix)), true, nativeFileIterator(fn))
}
func (s *FileSet) WithGlobal(fn Iterator) {
	l.Debugf("%s WithGlobal()", s.folder)
	s.db.withGlobal([]byte(s.folder), nil, false, nativeFileIterator(fn))
}

func (s *FileSet) WithGlobalTruncated(fn Iterator) {
	l.Debugf("%s WithGlobalTruncated()", s.folder)
	s.db.withGlobal([]byte(s.folder), nil, true, nativeFileIterator(fn))
}

func (s *FileSet) WithPrefixedGlobalTruncated(prefix string, fn Iterator) {
	l.Debugf("%s WithPrefixedGlobalTruncated()", s.folder, prefix)
	s.db.withGlobal([]byte(s.folder), []byte(osutil.NormalizedFilename(prefix)), true, nativeFileIterator(fn))
}

func (s *FileSet) Get(device protocol.DeviceID, file string) (protocol.FileInfo, bool) {
	f, ok := s.db.getFile([]byte(s.folder), device[:], []byte(osutil.NormalizedFilename(file)))
	f.Name = osutil.NativeFilename(f.Name)
	return f, ok
}

func (s *FileSet) GetGlobal(file string) (protocol.FileInfo, bool) {
	fi, ok := s.db.getGlobal([]byte(s.folder), []byte(osutil.NormalizedFilename(file)), false)
	if !ok {
		return protocol.FileInfo{}, false
	}
	f := fi.(protocol.FileInfo)
	f.Name = osutil.NativeFilename(f.Name)
	return f, true
}

func (s *FileSet) GetGlobalTruncated(file string) (FileInfoTruncated, bool) {
	fi, ok := s.db.getGlobal([]byte(s.folder), []byte(osutil.NormalizedFilename(file)), true)
	if !ok {
		return FileInfoTruncated{}, false
	}
	f := fi.(FileInfoTruncated)
	f.Name = osutil.NativeFilename(f.Name)
	return f, true
}

func (s *FileSet) Availability(file string) []protocol.DeviceID {
	return s.db.availability([]byte(s.folder), []byte(osutil.NormalizedFilename(file)))
}

func (s *FileSet) Sequence(device protocol.DeviceID) int64 {
	return s.meta.Size(device).Sequence
}

func (s *FileSet) LocalSize() Counts {
	return s.meta.Size(protocol.LocalDeviceID)
}

func (s *FileSet) GlobalSize() Counts {
	return s.meta.Size(globalDeviceID)
}

func (s *FileSet) IndexID(device protocol.DeviceID) protocol.IndexID {
	id := s.db.getIndexID(device[:], []byte(s.folder))
	if id == 0 && device == protocol.LocalDeviceID {
		// No index ID set yet. We create one now.
		id = protocol.NewIndexID()
		s.db.setIndexID(device[:], []byte(s.folder), id)
	}
	return id
}

func (s *FileSet) SetIndexID(device protocol.DeviceID, id protocol.IndexID) {
	if device == protocol.LocalDeviceID {
		panic("do not explicitly set index ID for local device")
	}
	s.db.setIndexID(device[:], []byte(s.folder), id)
}

func (s *FileSet) MtimeFS() *fs.MtimeFS {
	prefix := s.db.mtimesKey([]byte(s.folder))
	kv := NewNamespacedKV(s.db, string(prefix))
	return fs.NewMtimeFS(s.fs, kv)
}

func (s *FileSet) ListDevices() []protocol.DeviceID {
	return s.meta.devices()
}

// DropFolder clears out all information related to the given folder from the
// database.
func DropFolder(db *Instance, folder string) {
	db.dropFolder([]byte(folder))
	db.dropMtimes([]byte(folder))
	db.dropFolderMeta([]byte(folder))
	bm := &BlockMap{
		db:     db,
		folder: db.folderIdx.ID([]byte(folder)),
	}
	bm.Drop()
}

func normalizeFilenames(fs []protocol.FileInfo) {
	for i := range fs {
		fs[i].Name = osutil.NormalizedFilename(fs[i].Name)
	}
}

func nativeFileIterator(fn Iterator) Iterator {
	return func(fi FileIntf) bool {
		switch f := fi.(type) {
		case protocol.FileInfo:
			f.Name = osutil.NativeFilename(f.Name)
			return fn(f)
		case FileInfoTruncated:
			f.Name = osutil.NativeFilename(f.Name)
			return fn(f)
		default:
			panic("unknown interface type")
		}
	}
}
