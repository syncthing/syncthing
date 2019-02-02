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
	"github.com/syndtr/goleveldb/leveldb/util"
)

type FileSet struct {
	folder string
	fs     fs.Filesystem
	db     *instance
	meta   *metadataTracker

	updateMutex sync.Mutex // protects database updates and the corresponding metadata changes
}

// FileIntf is the set of methods implemented by both protocol.FileInfo and
// FileInfoTruncated.
type FileIntf interface {
	FileSize() int64
	FileName() string
	FileLocalFlags() uint32
	IsDeleted() bool
	IsInvalid() bool
	IsIgnored() bool
	IsUnsupported() bool
	MustRescan() bool
	IsReceiveOnlyChanged() bool
	IsDirectory() bool
	IsSymlink() bool
	ShouldConflict() bool
	HasPermissionBits() bool
	SequenceNo() int64
	BlockSize() int
	FileVersion() protocol.Vector
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

func NewFileSet(folder string, fs fs.Filesystem, ll *Lowlevel) *FileSet {
	db := newInstance(ll)

	var s = FileSet{
		folder:      folder,
		fs:          fs,
		db:          db,
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

	// do not modify fs in place, it is still used in outer scope
	fs = append([]protocol.FileInfo(nil), fs...)

	normalizeFilenames(fs)

	s.updateMutex.Lock()
	defer s.updateMutex.Unlock()

	defer s.meta.toDB(s.db, []byte(s.folder))

	if device == protocol.LocalDeviceID {
		// For the local device we have a bunch of metadata to track.
		s.db.updateLocalFiles([]byte(s.folder), fs, s.meta)
		return
	}
	// Easy case, just update the files and we're done.
	s.db.updateRemoteFiles([]byte(s.folder), device[:], fs, s.meta)
}

func (s *FileSet) WithNeed(device protocol.DeviceID, fn Iterator) {
	l.Debugf("%s WithNeed(%v)", s.folder, device)
	s.db.withNeed([]byte(s.folder), device[:], false, nativeFileIterator(fn))
}

func (s *FileSet) WithNeedTruncated(device protocol.DeviceID, fn Iterator) {
	l.Debugf("%s WithNeedTruncated(%v)", s.folder, device)
	s.db.withNeed([]byte(s.folder), device[:], true, nativeFileIterator(fn))
}

func (s *FileSet) WithHave(device protocol.DeviceID, fn Iterator) {
	l.Debugf("%s WithHave(%v)", s.folder, device)
	s.db.withHave([]byte(s.folder), device[:], nil, false, nativeFileIterator(fn))
}

func (s *FileSet) WithHaveTruncated(device protocol.DeviceID, fn Iterator) {
	l.Debugf("%s WithHaveTruncated(%v)", s.folder, device)
	s.db.withHave([]byte(s.folder), device[:], nil, true, nativeFileIterator(fn))
}

func (s *FileSet) WithHaveSequence(startSeq int64, fn Iterator) {
	l.Debugf("%s WithHaveSequence(%v)", s.folder, startSeq)
	s.db.withHaveSequence([]byte(s.folder), startSeq, nativeFileIterator(fn))
}

// Except for an item with a path equal to prefix, only children of prefix are iterated.
// E.g. for prefix "dir", "dir/file" is iterated, but "dir.file" is not.
func (s *FileSet) WithPrefixedHaveTruncated(device protocol.DeviceID, prefix string, fn Iterator) {
	l.Debugf(`%s WithPrefixedHaveTruncated(%v, "%v")`, s.folder, device, prefix)
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

// Except for an item with a path equal to prefix, only children of prefix are iterated.
// E.g. for prefix "dir", "dir/file" is iterated, but "dir.file" is not.
func (s *FileSet) WithPrefixedGlobalTruncated(prefix string, fn Iterator) {
	l.Debugf(`%s WithPrefixedGlobalTruncated("%v")`, s.folder, prefix)
	s.db.withGlobal([]byte(s.folder), []byte(osutil.NormalizedFilename(prefix)), true, nativeFileIterator(fn))
}

func (s *FileSet) Get(device protocol.DeviceID, file string) (protocol.FileInfo, bool) {
	f, ok := s.db.getFileDirty([]byte(s.folder), device[:], []byte(osutil.NormalizedFilename(file)))
	f.Name = osutil.NativeFilename(f.Name)
	return f, ok
}

func (s *FileSet) GetGlobal(file string) (protocol.FileInfo, bool) {
	fi, ok := s.db.getGlobalDirty([]byte(s.folder), []byte(osutil.NormalizedFilename(file)), false)
	if !ok {
		return protocol.FileInfo{}, false
	}
	f := fi.(protocol.FileInfo)
	f.Name = osutil.NativeFilename(f.Name)
	return f, true
}

func (s *FileSet) GetGlobalTruncated(file string) (FileInfoTruncated, bool) {
	fi, ok := s.db.getGlobalDirty([]byte(s.folder), []byte(osutil.NormalizedFilename(file)), true)
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
	return s.meta.Sequence(device)
}

func (s *FileSet) LocalSize() Counts {
	local := s.meta.Counts(protocol.LocalDeviceID, 0)
	recvOnlyChanged := s.meta.Counts(protocol.LocalDeviceID, protocol.FlagLocalReceiveOnly)
	return local.Add(recvOnlyChanged)
}

func (s *FileSet) ReceiveOnlyChangedSize() Counts {
	return s.meta.Counts(protocol.LocalDeviceID, protocol.FlagLocalReceiveOnly)
}

func (s *FileSet) GlobalSize() Counts {
	global := s.meta.Counts(protocol.GlobalDeviceID, 0)
	recvOnlyChanged := s.meta.Counts(protocol.GlobalDeviceID, protocol.FlagLocalReceiveOnly)
	return global.Add(recvOnlyChanged)
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
	prefix := s.db.keyer.GenerateMtimesKey(nil, []byte(s.folder))
	kv := NewNamespacedKV(s.db.Lowlevel, string(prefix))
	return fs.NewMtimeFS(s.fs, kv)
}

func (s *FileSet) ListDevices() []protocol.DeviceID {
	return s.meta.devices()
}

// DropFolder clears out all information related to the given folder from the
// database.
func DropFolder(ll *Lowlevel, folder string) {
	db := newInstance(ll)
	db.dropFolder([]byte(folder))
	db.dropMtimes([]byte(folder))
	db.dropFolderMeta([]byte(folder))

	// Also clean out the folder ID mapping.
	db.folderIdx.Delete([]byte(folder))
}

// DropDeltaIndexIDs removes all delta index IDs from the database.
// This will cause a full index transmission on the next connection.
func DropDeltaIndexIDs(db *Lowlevel) {
	dbi := db.NewIterator(util.BytesPrefix([]byte{KeyTypeIndexID}), nil)
	defer dbi.Release()
	for dbi.Next() {
		db.Delete(dbi.Key(), nil)
	}
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
