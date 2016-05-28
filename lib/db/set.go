// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// Package db provides a set type to track local/remote files with newness
// checks. We must do a certain amount of normalization in here. We will get
// fed paths with either native or wire-format separators and encodings
// depending on who calls us. We transform paths to wire-format (NFC and
// slashes) on the way to the database, and transform to native format
// (varying separator and encoding) on the way back out.
package db

import (
	stdsync "sync"

	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/sync"
)

type FileSet struct {
	localVersion map[protocol.DeviceID]int64
	mutex        sync.Mutex
	folder       string
	db           *Instance
	blockmap     *BlockMap
	localSize    sizeTracker
	globalSize   sizeTracker
}

// FileIntf is the set of methods implemented by both protocol.FileInfo and
// protocol.FileInfoTruncated.
type FileIntf interface {
	Size() int64
	IsDeleted() bool
	IsInvalid() bool
	IsDirectory() bool
	IsSymlink() bool
	HasPermissionBits() bool
}

// The Iterator is called with either a protocol.FileInfo or a
// protocol.FileInfoTruncated (depending on the method) and returns true to
// continue iteration, false to stop.
type Iterator func(f FileIntf) bool

type sizeTracker struct {
	files   int
	deleted int
	bytes   int64
	mut     stdsync.Mutex
}

func (s *sizeTracker) addFile(f FileIntf) {
	if f.IsInvalid() {
		return
	}

	s.mut.Lock()
	if f.IsDeleted() {
		s.deleted++
	} else {
		s.files++
	}
	s.bytes += f.Size()
	s.mut.Unlock()
}

func (s *sizeTracker) removeFile(f FileIntf) {
	if f.IsInvalid() {
		return
	}

	s.mut.Lock()
	if f.IsDeleted() {
		s.deleted--
	} else {
		s.files--
	}
	s.bytes -= f.Size()
	if s.deleted < 0 || s.files < 0 {
		panic("bug: removed more than added")
	}
	s.mut.Unlock()
}

func (s *sizeTracker) Size() (files, deleted int, bytes int64) {
	s.mut.Lock()
	defer s.mut.Unlock()
	return s.files, s.deleted, s.bytes
}

func NewFileSet(folder string, db *Instance) *FileSet {
	var s = FileSet{
		localVersion: make(map[protocol.DeviceID]int64),
		folder:       folder,
		db:           db,
		blockmap:     NewBlockMap(db, db.folderIdx.ID([]byte(folder))),
		mutex:        sync.NewMutex(),
	}

	s.db.checkGlobals([]byte(folder), &s.globalSize)

	var deviceID protocol.DeviceID
	s.db.withAllFolderTruncated([]byte(folder), func(device []byte, f FileInfoTruncated) bool {
		copy(deviceID[:], device)
		if f.LocalVersion > s.localVersion[deviceID] {
			s.localVersion[deviceID] = f.LocalVersion
		}
		if deviceID == protocol.LocalDeviceID {
			s.localSize.addFile(f)
		}
		return true
	})
	l.Debugf("loaded localVersion for %q: %#v", folder, s.localVersion)
	clock(s.localVersion[protocol.LocalDeviceID])

	return &s
}

func (s *FileSet) Replace(device protocol.DeviceID, fs []protocol.FileInfo) {
	l.Debugf("%s Replace(%v, [%d])", s.folder, device, len(fs))
	normalizeFilenames(fs)
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.localVersion[device] = s.db.replace([]byte(s.folder), device[:], fs, &s.localSize, &s.globalSize)
	if len(fs) == 0 {
		// Reset the local version if all files were removed.
		s.localVersion[device] = 0
	}
	if device == protocol.LocalDeviceID {
		s.blockmap.Drop()
		s.blockmap.Add(fs)
	}
}

func (s *FileSet) Update(device protocol.DeviceID, fs []protocol.FileInfo) {
	l.Debugf("%s Update(%v, [%d])", s.folder, device, len(fs))
	normalizeFilenames(fs)
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if device == protocol.LocalDeviceID {
		discards := make([]protocol.FileInfo, 0, len(fs))
		updates := make([]protocol.FileInfo, 0, len(fs))
		for _, newFile := range fs {
			existingFile, ok := s.db.getFile([]byte(s.folder), device[:], []byte(newFile.Name))
			if !ok || !existingFile.Version.Equal(newFile.Version) {
				discards = append(discards, existingFile)
				updates = append(updates, newFile)
			}
		}
		s.blockmap.Discard(discards)
		s.blockmap.Update(updates)
	}
	if lv := s.db.updateFiles([]byte(s.folder), device[:], fs, &s.localSize, &s.globalSize); lv > s.localVersion[device] {
		s.localVersion[device] = lv
	}
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

func (s *FileSet) LocalVersion(device protocol.DeviceID) int64 {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.localVersion[device]
}

func (s *FileSet) LocalSize() (files, deleted int, bytes int64) {
	return s.localSize.Size()
}

func (s *FileSet) GlobalSize() (files, deleted int, bytes int64) {
	return s.globalSize.Size()
}

// DropFolder clears out all information related to the given folder from the
// database.
func DropFolder(db *Instance, folder string) {
	db.dropFolder([]byte(folder))
	bm := &BlockMap{
		db:     db,
		folder: db.folderIdx.ID([]byte(folder)),
	}
	bm.Drop()
	NewVirtualMtimeRepo(db, folder).Drop()
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
