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
	stdsync "sync"
	"sync/atomic"

	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/sync"
)

type FileSet struct {
	sequence   int64 // Our local sequence number
	folder     string
	fs         fs.Filesystem
	db         *Instance
	blockmap   *BlockMap
	localSize  sizeTracker
	globalSize sizeTracker

	remoteSequence map[protocol.DeviceID]int64 // Highest seen sequence numbers for other devices
	updateMutex    sync.Mutex                  // protects remoteSequence and database updates
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
}

// The Iterator is called with either a protocol.FileInfo or a
// FileInfoTruncated (depending on the method) and returns true to
// continue iteration, false to stop.
type Iterator func(f FileIntf) bool

type Counts struct {
	Files       int
	Directories int
	Symlinks    int
	Deleted     int
	Bytes       int64
}

type sizeTracker struct {
	Counts
	mut stdsync.Mutex
}

func (s *sizeTracker) addFile(f FileIntf) {
	if f.IsInvalid() {
		return
	}

	s.mut.Lock()
	switch {
	case f.IsDeleted():
		s.Deleted++
	case f.IsDirectory() && !f.IsSymlink():
		s.Directories++
	case f.IsSymlink():
		s.Symlinks++
	default:
		s.Files++
	}
	s.Bytes += f.FileSize()
	s.mut.Unlock()
}

func (s *sizeTracker) removeFile(f FileIntf) {
	if f.IsInvalid() {
		return
	}

	s.mut.Lock()
	switch {
	case f.IsDeleted():
		s.Deleted--
	case f.IsDirectory() && !f.IsSymlink():
		s.Directories--
	case f.IsSymlink():
		s.Symlinks--
	default:
		s.Files--
	}
	s.Bytes -= f.FileSize()
	if s.Deleted < 0 || s.Files < 0 || s.Directories < 0 || s.Symlinks < 0 {
		panic("bug: removed more than added")
	}
	s.mut.Unlock()
}

func (s *sizeTracker) Size() Counts {
	s.mut.Lock()
	defer s.mut.Unlock()
	return s.Counts
}

func NewFileSet(folder string, fs fs.Filesystem, db *Instance) *FileSet {
	var s = FileSet{
		remoteSequence: make(map[protocol.DeviceID]int64),
		folder:         folder,
		fs:             fs,
		db:             db,
		blockmap:       NewBlockMap(db, db.folderIdx.ID([]byte(folder))),
		updateMutex:    sync.NewMutex(),
	}

	s.db.checkGlobals([]byte(folder), &s.globalSize)

	var deviceID protocol.DeviceID
	s.db.withAllFolderTruncated([]byte(folder), func(device []byte, f FileInfoTruncated) bool {
		copy(deviceID[:], device)
		if deviceID == protocol.LocalDeviceID {
			if f.Sequence > s.sequence {
				s.sequence = f.Sequence
			}
			s.localSize.addFile(f)
		} else if f.Sequence > s.remoteSequence[deviceID] {
			s.remoteSequence[deviceID] = f.Sequence
		}
		return true
	})
	l.Debugf("loaded sequence for %q: %#v", folder, s.sequence)

	return &s
}

func (s *FileSet) Replace(device protocol.DeviceID, fs []protocol.FileInfo) {
	l.Debugf("%s Replace(%v, [%d])", s.folder, device, len(fs))
	normalizeFilenames(fs)

	s.updateMutex.Lock()
	defer s.updateMutex.Unlock()

	if device == protocol.LocalDeviceID {
		if len(fs) == 0 {
			s.sequence = 0
		} else {
			// Always overwrite Sequence on updated files to ensure
			// correct ordering. The caller is supposed to leave it set to
			// zero anyhow.
			for i := range fs {
				fs[i].Sequence = atomic.AddInt64(&s.sequence, 1)
			}
		}
	} else {
		s.remoteSequence[device] = maxSequence(fs)
	}
	s.db.replace([]byte(s.folder), device[:], fs, &s.localSize, &s.globalSize)
	if device == protocol.LocalDeviceID {
		s.blockmap.Drop()
		s.blockmap.Add(fs)
	}
}

func (s *FileSet) Update(device protocol.DeviceID, fs []protocol.FileInfo) {
	l.Debugf("%s Update(%v, [%d])", s.folder, device, len(fs))
	normalizeFilenames(fs)

	s.updateMutex.Lock()
	defer s.updateMutex.Unlock()

	if device == protocol.LocalDeviceID {
		discards := make([]protocol.FileInfo, 0, len(fs))
		updates := make([]protocol.FileInfo, 0, len(fs))
		for i, newFile := range fs {
			fs[i].Sequence = atomic.AddInt64(&s.sequence, 1)
			existingFile, ok := s.db.getFile([]byte(s.folder), device[:], []byte(newFile.Name))
			if !ok || !existingFile.Version.Equal(newFile.Version) {
				discards = append(discards, existingFile)
				updates = append(updates, newFile)
			}
		}
		s.blockmap.Discard(discards)
		s.blockmap.Update(updates)
	} else {
		s.remoteSequence[device] = maxSequence(fs)
	}
	s.db.updateFiles([]byte(s.folder), device[:], fs, &s.localSize, &s.globalSize)
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
	if device == protocol.LocalDeviceID {
		return atomic.LoadInt64(&s.sequence)
	}

	s.updateMutex.Lock()
	defer s.updateMutex.Unlock()
	return s.remoteSequence[device]
}

func (s *FileSet) LocalSize() Counts {
	return s.localSize.Size()
}

func (s *FileSet) GlobalSize() Counts {
	return s.globalSize.Size()
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
	s.updateMutex.Lock()
	devices := make([]protocol.DeviceID, 0, len(s.remoteSequence))
	for id, seq := range s.remoteSequence {
		if seq > 0 {
			devices = append(devices, id)
		}
	}
	s.updateMutex.Unlock()
	return devices
}

// maxSequence returns the highest of the Sequence numbers found in
// the given slice of FileInfos. This should really be the Sequence of
// the last item, but Syncthing v0.14.0 and other implementations may not
// implement update sorting....
func maxSequence(fs []protocol.FileInfo) int64 {
	var max int64
	for _, f := range fs {
		if f.Sequence > max {
			max = f.Sequence
		}
	}
	return max
}

// DropFolder clears out all information related to the given folder from the
// database.
func DropFolder(db *Instance, folder string) {
	db.dropFolder([]byte(folder))
	db.dropMtimes([]byte(folder))
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
