// Copyright (C) 2014 The Syncthing Authors.
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

// Package files provides a set type to track local/remote files with newness
// checks. We must do a certain amount of normalization in here. We will get
// fed paths with either native or wire-format separators and encodings
// depending on who calls us. We transform paths to wire-format (NFC and
// slashes) on the way to the database, and transform to native format
// (varying separator and encoding) on the way back out.
package files

import (
	"sync"

	"github.com/syncthing/syncthing/internal/lamport"
	"github.com/syncthing/syncthing/internal/osutil"
	"github.com/syncthing/syncthing/internal/protocol"
	"github.com/syndtr/goleveldb/leveldb"
)

type Set struct {
	localVersion map[protocol.DeviceID]uint64
	mutex        sync.Mutex
	folder       string
	db           *leveldb.DB
	blockmap     *BlockMap
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

func NewSet(folder string, db *leveldb.DB) *Set {
	var s = Set{
		localVersion: make(map[protocol.DeviceID]uint64),
		folder:       folder,
		db:           db,
		blockmap:     NewBlockMap(db, folder),
	}

	ldbCheckGlobals(db, []byte(folder))

	var deviceID protocol.DeviceID
	ldbWithAllFolderTruncated(db, []byte(folder), func(device []byte, f FileInfoTruncated) bool {
		copy(deviceID[:], device)
		if f.LocalVersion > s.localVersion[deviceID] {
			s.localVersion[deviceID] = f.LocalVersion
		}
		lamport.Default.Tick(f.Version)
		return true
	})
	if debug {
		l.Debugf("loaded localVersion for %q: %#v", folder, s.localVersion)
	}
	clock(s.localVersion[protocol.LocalDeviceID])

	return &s
}

func (s *Set) Replace(device protocol.DeviceID, fs []protocol.FileInfo) {
	if debug {
		l.Debugf("%s Replace(%v, [%d])", s.folder, device, len(fs))
	}
	normalizeFilenames(fs)
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.localVersion[device] = ldbReplace(s.db, []byte(s.folder), device[:], fs)
	if len(fs) == 0 {
		// Reset the local version if all files were removed.
		s.localVersion[device] = 0
	}
	if device == protocol.LocalDeviceID {
		s.blockmap.Drop()
		s.blockmap.Add(fs)
	}
}

func (s *Set) ReplaceWithDelete(device protocol.DeviceID, fs []protocol.FileInfo) {
	if debug {
		l.Debugf("%s ReplaceWithDelete(%v, [%d])", s.folder, device, len(fs))
	}
	normalizeFilenames(fs)
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if lv := ldbReplaceWithDelete(s.db, []byte(s.folder), device[:], fs); lv > s.localVersion[device] {
		s.localVersion[device] = lv
	}
	if device == protocol.LocalDeviceID {
		s.blockmap.Drop()
		s.blockmap.Add(fs)
	}
}

func (s *Set) Update(device protocol.DeviceID, fs []protocol.FileInfo) {
	if debug {
		l.Debugf("%s Update(%v, [%d])", s.folder, device, len(fs))
	}
	normalizeFilenames(fs)
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if device == protocol.LocalDeviceID {
		discards := make([]protocol.FileInfo, 0, len(fs))
		updates := make([]protocol.FileInfo, 0, len(fs))
		for _, newFile := range fs {
			existingFile, ok := ldbGet(s.db, []byte(s.folder), device[:], []byte(newFile.Name))
			if !ok || existingFile.Version <= newFile.Version {
				discards = append(discards, existingFile)
				updates = append(updates, newFile)
			}
		}
		s.blockmap.Discard(discards)
		s.blockmap.Update(updates)
	}
	if lv := ldbUpdate(s.db, []byte(s.folder), device[:], fs); lv > s.localVersion[device] {
		s.localVersion[device] = lv
	}
}

func (s *Set) WithNeed(device protocol.DeviceID, fn Iterator) {
	if debug {
		l.Debugf("%s WithNeed(%v)", s.folder, device)
	}
	ldbWithNeed(s.db, []byte(s.folder), device[:], false, nativeFileIterator(fn))
}

func (s *Set) WithNeedTruncated(device protocol.DeviceID, fn Iterator) {
	if debug {
		l.Debugf("%s WithNeedTruncated(%v)", s.folder, device)
	}
	ldbWithNeed(s.db, []byte(s.folder), device[:], true, nativeFileIterator(fn))
}

func (s *Set) WithHave(device protocol.DeviceID, fn Iterator) {
	if debug {
		l.Debugf("%s WithHave(%v)", s.folder, device)
	}
	ldbWithHave(s.db, []byte(s.folder), device[:], false, nativeFileIterator(fn))
}

func (s *Set) WithHaveTruncated(device protocol.DeviceID, fn Iterator) {
	if debug {
		l.Debugf("%s WithHaveTruncated(%v)", s.folder, device)
	}
	ldbWithHave(s.db, []byte(s.folder), device[:], true, nativeFileIterator(fn))
}

func (s *Set) WithGlobal(fn Iterator) {
	if debug {
		l.Debugf("%s WithGlobal()", s.folder)
	}
	ldbWithGlobal(s.db, []byte(s.folder), false, nativeFileIterator(fn))
}

func (s *Set) WithGlobalTruncated(fn Iterator) {
	if debug {
		l.Debugf("%s WithGlobalTruncated()", s.folder)
	}
	ldbWithGlobal(s.db, []byte(s.folder), true, nativeFileIterator(fn))
}

func (s *Set) Get(device protocol.DeviceID, file string) (protocol.FileInfo, bool) {
	f, ok := ldbGet(s.db, []byte(s.folder), device[:], []byte(osutil.NormalizedFilename(file)))
	f.Name = osutil.NativeFilename(f.Name)
	return f, ok
}

func (s *Set) GetGlobal(file string) (protocol.FileInfo, bool) {
	f, ok := ldbGetGlobal(s.db, []byte(s.folder), []byte(osutil.NormalizedFilename(file)))
	f.Name = osutil.NativeFilename(f.Name)
	return f, ok
}

func (s *Set) Availability(file string) []protocol.DeviceID {
	return ldbAvailability(s.db, []byte(s.folder), []byte(osutil.NormalizedFilename(file)))
}

func (s *Set) LocalVersion(device protocol.DeviceID) uint64 {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.localVersion[device]
}

// ListFolders returns the folder IDs seen in the database.
func ListFolders(db *leveldb.DB) []string {
	return ldbListFolders(db)
}

// DropFolder clears out all information related to the given folder from the
// database.
func DropFolder(db *leveldb.DB, folder string) {
	ldbDropFolder(db, []byte(folder))
	bm := &BlockMap{
		db:     db,
		folder: folder,
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
