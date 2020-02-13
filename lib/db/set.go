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

	"github.com/syncthing/syncthing/lib/db/backend"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/sync"
)

type FileSet struct {
	folder string
	fs     fs.Filesystem
	db     *Lowlevel
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
	FileType() protocol.FileInfoType
	FilePermissions() uint32
	FileModifiedBy() protocol.ShortID
	ModTime() time.Time
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

func NewFileSet(folder string, fs fs.Filesystem, db *Lowlevel) *FileSet {
	var s = &FileSet{
		folder:      folder,
		fs:          fs,
		db:          db,
		meta:        newMetadataTracker(),
		updateMutex: sync.NewMutex(),
	}

	recalc := func() *FileSet {
		if err := s.recalcMeta(); backend.IsClosed(err) {
			return nil
		} else if err != nil {
			panic(err)
		}
		return s
	}

	if err := s.meta.fromDB(db, []byte(folder)); err != nil {
		l.Infof("No stored folder metadata for %q; recalculating", folder)
		return recalc()
	}

	if metaOK := s.verifyLocalSequence(); !metaOK {
		l.Infof("Stored folder metadata for %q is out of date after crash; recalculating", folder)
		return recalc()
	}

	if age := time.Since(s.meta.Created()); age > databaseRecheckInterval {
		l.Infof("Stored folder metadata for %q is %v old; recalculating", folder, age)
		return recalc()
	}

	return s
}

func (s *FileSet) recalcMeta() error {
	s.meta = newMetadataTracker()

	if err := s.db.checkGlobals([]byte(s.folder), s.meta); err != nil {
		return err
	}

	t, err := s.db.newReadWriteTransaction()
	if err != nil {
		return err
	}
	defer t.close()

	var deviceID protocol.DeviceID
	err = t.withAllFolderTruncated([]byte(s.folder), func(device []byte, f FileInfoTruncated) bool {
		copy(deviceID[:], device)
		s.meta.addFile(deviceID, f)
		return true
	})
	if err != nil {
		return err
	}

	s.meta.SetCreated()
	if err := s.meta.toDB(t, []byte(s.folder)); err != nil {
		return err
	}
	return t.Commit()
}

// Verify the local sequence number from actual sequence entries. Returns
// true if it was all good, or false if a fixup was necessary.
func (s *FileSet) verifyLocalSequence() bool {
	// Walk the sequence index from the current (supposedly) highest
	// sequence number and raise the alarm if we get anything. This recovers
	// from the occasion where we have written sequence entries to disk but
	// not yet written new metadata to disk.
	//
	// Note that we can have the same thing happen for remote devices but
	// there it's not a problem -- we'll simply advertise a lower sequence
	// number than we've actually seen and receive some duplicate updates
	// and then be in sync again.

	curSeq := s.meta.Sequence(protocol.LocalDeviceID)

	snap := s.Snapshot()
	ok := true
	snap.WithHaveSequence(curSeq+1, func(fi FileIntf) bool {
		ok = false // we got something, which we should not have
		return false
	})
	snap.Release()

	return ok
}

func (s *FileSet) Drop(device protocol.DeviceID) {
	l.Debugf("%s Drop(%v)", s.folder, device)

	s.updateMutex.Lock()
	defer s.updateMutex.Unlock()

	if err := s.db.dropDeviceFolder(device[:], []byte(s.folder), s.meta); backend.IsClosed(err) {
		return
	} else if err != nil {
		panic(err)
	}

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

	t, err := s.db.newReadWriteTransaction()
	if backend.IsClosed(err) {
		return
	} else if err != nil {
		panic(err)
	}
	defer t.close()

	if err := s.meta.toDB(t, []byte(s.folder)); backend.IsClosed(err) {
		return
	} else if err != nil {
		panic(err)
	}
	if err := t.Commit(); backend.IsClosed(err) {
		return
	} else if err != nil {
		panic(err)
	}
}

func (s *FileSet) Update(device protocol.DeviceID, fs []protocol.FileInfo) {
	l.Debugf("%s Update(%v, [%d])", s.folder, device, len(fs))

	// do not modify fs in place, it is still used in outer scope
	fs = append([]protocol.FileInfo(nil), fs...)

	normalizeFilenames(fs)

	s.updateMutex.Lock()
	defer s.updateMutex.Unlock()

	if device == protocol.LocalDeviceID {
		// For the local device we have a bunch of metadata to track.
		if err := s.db.updateLocalFiles([]byte(s.folder), fs, s.meta); err != nil && !backend.IsClosed(err) {
			panic(err)
		}
		return
	}
	// Easy case, just update the files and we're done.
	if err := s.db.updateRemoteFiles([]byte(s.folder), device[:], fs, s.meta); err != nil && !backend.IsClosed(err) {
		panic(err)
	}
}

type Snapshot struct {
	folder string
	t      readOnlyTransaction
	meta   *countsMap
}

func (s *FileSet) Snapshot() *Snapshot {
	t, err := s.db.newReadOnlyTransaction()
	if err != nil {
		panic(err)
	}
	return &Snapshot{
		folder: s.folder,
		t:      t,
		meta:   s.meta.Snapshot(),
	}
}

func (s *Snapshot) Release() {
	s.t.close()
}

func (s *Snapshot) WithNeed(device protocol.DeviceID, fn Iterator) {
	l.Debugf("%s WithNeed(%v)", s.folder, device)
	if err := s.t.withNeed([]byte(s.folder), device[:], false, nativeFileIterator(fn)); err != nil && !backend.IsClosed(err) {
		panic(err)
	}
}

func (s *Snapshot) WithNeedTruncated(device protocol.DeviceID, fn Iterator) {
	l.Debugf("%s WithNeedTruncated(%v)", s.folder, device)
	if err := s.t.withNeed([]byte(s.folder), device[:], true, nativeFileIterator(fn)); err != nil && !backend.IsClosed(err) {
		panic(err)
	}
}

func (s *Snapshot) WithHave(device protocol.DeviceID, fn Iterator) {
	l.Debugf("%s WithHave(%v)", s.folder, device)
	if err := s.t.withHave([]byte(s.folder), device[:], nil, false, nativeFileIterator(fn)); err != nil && !backend.IsClosed(err) {
		panic(err)
	}
}

func (s *Snapshot) WithHaveTruncated(device protocol.DeviceID, fn Iterator) {
	l.Debugf("%s WithHaveTruncated(%v)", s.folder, device)
	if err := s.t.withHave([]byte(s.folder), device[:], nil, true, nativeFileIterator(fn)); err != nil && !backend.IsClosed(err) {
		panic(err)
	}
}

func (s *Snapshot) WithHaveSequence(startSeq int64, fn Iterator) {
	l.Debugf("%s WithHaveSequence(%v)", s.folder, startSeq)
	if err := s.t.withHaveSequence([]byte(s.folder), startSeq, nativeFileIterator(fn)); err != nil && !backend.IsClosed(err) {
		panic(err)
	}
}

// Except for an item with a path equal to prefix, only children of prefix are iterated.
// E.g. for prefix "dir", "dir/file" is iterated, but "dir.file" is not.
func (s *Snapshot) WithPrefixedHaveTruncated(device protocol.DeviceID, prefix string, fn Iterator) {
	l.Debugf(`%s WithPrefixedHaveTruncated(%v, "%v")`, s.folder, device, prefix)
	if err := s.t.withHave([]byte(s.folder), device[:], []byte(osutil.NormalizedFilename(prefix)), true, nativeFileIterator(fn)); err != nil && !backend.IsClosed(err) {
		panic(err)
	}
}

func (s *Snapshot) WithGlobal(fn Iterator) {
	l.Debugf("%s WithGlobal()", s.folder)
	if err := s.t.withGlobal([]byte(s.folder), nil, false, nativeFileIterator(fn)); err != nil && !backend.IsClosed(err) {
		panic(err)
	}
}

func (s *Snapshot) WithGlobalTruncated(fn Iterator) {
	l.Debugf("%s WithGlobalTruncated()", s.folder)
	if err := s.t.withGlobal([]byte(s.folder), nil, true, nativeFileIterator(fn)); err != nil && !backend.IsClosed(err) {
		panic(err)
	}
}

// Except for an item with a path equal to prefix, only children of prefix are iterated.
// E.g. for prefix "dir", "dir/file" is iterated, but "dir.file" is not.
func (s *Snapshot) WithPrefixedGlobalTruncated(prefix string, fn Iterator) {
	l.Debugf(`%s WithPrefixedGlobalTruncated("%v")`, s.folder, prefix)
	if err := s.t.withGlobal([]byte(s.folder), []byte(osutil.NormalizedFilename(prefix)), true, nativeFileIterator(fn)); err != nil && !backend.IsClosed(err) {
		panic(err)
	}
}

func (s *Snapshot) Get(device protocol.DeviceID, file string) (protocol.FileInfo, bool) {
	f, ok, err := s.t.getFile([]byte(s.folder), device[:], []byte(osutil.NormalizedFilename(file)))
	if backend.IsClosed(err) {
		return protocol.FileInfo{}, false
	} else if err != nil {
		panic(err)
	}
	f.Name = osutil.NativeFilename(f.Name)
	return f, ok
}

func (s *Snapshot) GetGlobal(file string) (protocol.FileInfo, bool) {
	_, fi, ok, err := s.t.getGlobal(nil, []byte(s.folder), []byte(osutil.NormalizedFilename(file)), false)
	if backend.IsClosed(err) {
		return protocol.FileInfo{}, false
	} else if err != nil {
		panic(err)
	}
	if !ok {
		return protocol.FileInfo{}, false
	}
	f := fi.(protocol.FileInfo)
	f.Name = osutil.NativeFilename(f.Name)
	return f, true
}

func (s *Snapshot) GetGlobalTruncated(file string) (FileInfoTruncated, bool) {
	_, fi, ok, err := s.t.getGlobal(nil, []byte(s.folder), []byte(osutil.NormalizedFilename(file)), true)
	if backend.IsClosed(err) {
		return FileInfoTruncated{}, false
	} else if err != nil {
		panic(err)
	}
	if !ok {
		return FileInfoTruncated{}, false
	}
	f := fi.(FileInfoTruncated)
	f.Name = osutil.NativeFilename(f.Name)
	return f, true
}

func (s *Snapshot) Availability(file string) []protocol.DeviceID {
	av, err := s.t.availability([]byte(s.folder), []byte(osutil.NormalizedFilename(file)))
	if backend.IsClosed(err) {
		return nil
	} else if err != nil {
		panic(err)
	}
	return av
}

func (s *Snapshot) Sequence(device protocol.DeviceID) int64 {
	return s.meta.Counts(device, 0).Sequence
}

// RemoteSequence returns the change version for the given folder, as
// sent by remote peers. This is guaranteed to increment if the contents of
// the remote or global folder has changed.
func (s *Snapshot) RemoteSequence() int64 {
	var ver int64

	for _, device := range s.meta.devices() {
		ver += s.Sequence(device)
	}

	return ver
}

func (s *Snapshot) LocalSize() Counts {
	local := s.meta.Counts(protocol.LocalDeviceID, 0)
	return local.Add(s.ReceiveOnlyChangedSize())
}

func (s *Snapshot) ReceiveOnlyChangedSize() Counts {
	return s.meta.Counts(protocol.LocalDeviceID, protocol.FlagLocalReceiveOnly)
}

func (s *Snapshot) GlobalSize() Counts {
	global := s.meta.Counts(protocol.GlobalDeviceID, 0)
	recvOnlyChanged := s.meta.Counts(protocol.GlobalDeviceID, protocol.FlagLocalReceiveOnly)
	return global.Add(recvOnlyChanged)
}

func (s *Snapshot) NeedSize() Counts {
	var result Counts
	s.WithNeedTruncated(protocol.LocalDeviceID, func(f FileIntf) bool {
		switch {
		case f.IsDeleted():
			result.Deleted++
		case f.IsDirectory():
			result.Directories++
		case f.IsSymlink():
			result.Symlinks++
		default:
			result.Files++
			result.Bytes += f.FileSize()
		}
		return true
	})
	return result
}

// LocalChangedFiles returns a paginated list of currently needed files in
// progress, queued, and to be queued on next puller iteration, as well as the
// total number of files currently needed.
func (s *Snapshot) LocalChangedFiles(page, perpage int) []FileInfoTruncated {
	if s.ReceiveOnlyChangedSize().TotalItems() == 0 {
		return nil
	}

	files := make([]FileInfoTruncated, 0, perpage)

	skip := (page - 1) * perpage
	get := perpage

	s.WithHaveTruncated(protocol.LocalDeviceID, func(f FileIntf) bool {
		if !f.IsReceiveOnlyChanged() {
			return true
		}
		if skip > 0 {
			skip--
			return true
		}
		ft := f.(FileInfoTruncated)
		files = append(files, ft)
		get--
		return get > 0
	})

	return files
}

// RemoteNeedFolderFiles returns paginated list of currently needed files in
// progress, queued, and to be queued on next puller iteration, as well as the
// total number of files currently needed.
func (s *Snapshot) RemoteNeedFolderFiles(device protocol.DeviceID, page, perpage int) []FileInfoTruncated {
	files := make([]FileInfoTruncated, 0, perpage)
	skip := (page - 1) * perpage
	get := perpage
	s.WithNeedTruncated(device, func(f FileIntf) bool {
		if skip > 0 {
			skip--
			return true
		}
		files = append(files, f.(FileInfoTruncated))
		get--
		return get > 0
	})
	return files
}

func (s *FileSet) Sequence(device protocol.DeviceID) int64 {
	return s.meta.Sequence(device)
}

func (s *FileSet) IndexID(device protocol.DeviceID) protocol.IndexID {
	id, err := s.db.getIndexID(device[:], []byte(s.folder))
	if backend.IsClosed(err) {
		return 0
	} else if err != nil {
		panic(err)
	}
	if id == 0 && device == protocol.LocalDeviceID {
		// No index ID set yet. We create one now.
		id = protocol.NewIndexID()
		err := s.db.setIndexID(device[:], []byte(s.folder), id)
		if backend.IsClosed(err) {
			return 0
		} else if err != nil {
			panic(err)
		}
	}
	return id
}

func (s *FileSet) SetIndexID(device protocol.DeviceID, id protocol.IndexID) {
	if device == protocol.LocalDeviceID {
		panic("do not explicitly set index ID for local device")
	}
	if err := s.db.setIndexID(device[:], []byte(s.folder), id); err != nil && !backend.IsClosed(err) {
		panic(err)
	}
}

func (s *FileSet) MtimeFS() *fs.MtimeFS {
	prefix, err := s.db.keyer.GenerateMtimesKey(nil, []byte(s.folder))
	if backend.IsClosed(err) {
		return nil
	} else if err != nil {
		panic(err)
	}
	kv := NewNamespacedKV(s.db, string(prefix))
	return fs.NewMtimeFS(s.fs, kv)
}

func (s *FileSet) ListDevices() []protocol.DeviceID {
	return s.meta.devices()
}

// DropFolder clears out all information related to the given folder from the
// database.
func DropFolder(db *Lowlevel, folder string) {
	droppers := []func([]byte) error{
		db.dropFolder,
		db.dropMtimes,
		db.dropFolderMeta,
		db.folderIdx.Delete,
	}
	for _, drop := range droppers {
		if err := drop([]byte(folder)); backend.IsClosed(err) {
			return
		} else if err != nil {
			panic(err)
		}
	}
}

// DropDeltaIndexIDs removes all delta index IDs from the database.
// This will cause a full index transmission on the next connection.
func DropDeltaIndexIDs(db *Lowlevel) {
	dbi, err := db.NewPrefixIterator([]byte{KeyTypeIndexID})
	if backend.IsClosed(err) {
		return
	} else if err != nil {
		panic(err)
	}
	defer dbi.Release()
	for dbi.Next() {
		if err := db.Delete(dbi.Key()); err != nil && !backend.IsClosed(err) {
			panic(err)
		}
	}
	if err := dbi.Error(); err != nil && !backend.IsClosed(err) {
		panic(err)
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
