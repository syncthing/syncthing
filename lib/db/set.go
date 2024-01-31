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
	"fmt"

	"github.com/syncthing/syncthing/lib/db/backend"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/sync"
)

type FileSet struct {
	folder string
	db     *Lowlevel
	meta   *metadataTracker

	updateMutex sync.Mutex // protects database updates and the corresponding metadata changes
}

// The Iterator is called with either a protocol.FileInfo or a
// FileInfoTruncated (depending on the method) and returns true to
// continue iteration, false to stop.
type Iterator func(f protocol.FileIntf) bool

func NewFileSet(folder string, db *Lowlevel) (*FileSet, error) {
	select {
	case <-db.oneFileSetCreated:
	default:
		close(db.oneFileSetCreated)
	}
	meta, err := db.loadMetadataTracker(folder)
	if err != nil {
		db.handleFailure(err)
		return nil, err
	}
	s := &FileSet{
		folder:      folder,
		db:          db,
		meta:        meta,
		updateMutex: sync.NewMutex(),
	}
	if id := s.IndexID(protocol.LocalDeviceID); id == 0 {
		// No index ID set yet. We create one now.
		id = protocol.NewIndexID()
		err := s.db.setIndexID(protocol.LocalDeviceID[:], []byte(s.folder), id)
		if err != nil && !backend.IsClosed(err) {
			fatalError(err, fmt.Sprintf("%s Creating new IndexID", s.folder), s.db)
		}
	}
	return s, nil
}

func (s *FileSet) Drop(device protocol.DeviceID) {
	opStr := fmt.Sprintf("%s Drop(%v)", s.folder, device)
	l.Debugf(opStr)

	s.updateMutex.Lock()
	defer s.updateMutex.Unlock()

	if err := s.db.dropDeviceFolder(device[:], []byte(s.folder), s.meta); backend.IsClosed(err) {
		return
	} else if err != nil {
		fatalError(err, opStr, s.db)
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
		fatalError(err, opStr, s.db)
	}
	defer t.close()

	if err := s.meta.toDB(t, []byte(s.folder)); backend.IsClosed(err) {
		return
	} else if err != nil {
		fatalError(err, opStr, s.db)
	}
	if err := t.Commit(); backend.IsClosed(err) {
		return
	} else if err != nil {
		fatalError(err, opStr, s.db)
	}
}

func (s *FileSet) Update(device protocol.DeviceID, fs []protocol.FileInfo) {
	opStr := fmt.Sprintf("%s Update(%v, [%d])", s.folder, device, len(fs))
	l.Debugf(opStr)

	// do not modify fs in place, it is still used in outer scope
	fs = append([]protocol.FileInfo(nil), fs...)

	// If one file info is present multiple times, only keep the last.
	// Updating the same file multiple times is problematic, because the
	// previous updates won't yet be represented in the db when we update it
	// again. Additionally even if that problem was taken care of, it would
	// be pointless because we remove the previously added file info again
	// right away.
	fs = normalizeFilenamesAndDropDuplicates(fs)

	s.updateMutex.Lock()
	defer s.updateMutex.Unlock()

	if device == protocol.LocalDeviceID {
		// For the local device we have a bunch of metadata to track.
		if err := s.db.updateLocalFiles([]byte(s.folder), fs, s.meta); err != nil && !backend.IsClosed(err) {
			fatalError(err, opStr, s.db)
		}
		return
	}
	// Easy case, just update the files and we're done.
	if err := s.db.updateRemoteFiles([]byte(s.folder), device[:], fs, s.meta); err != nil && !backend.IsClosed(err) {
		fatalError(err, opStr, s.db)
	}
}

func (s *FileSet) RemoveLocalItems(items []string) {
	opStr := fmt.Sprintf("%s RemoveLocalItems([%d])", s.folder, len(items))
	l.Debugf(opStr)

	s.updateMutex.Lock()
	defer s.updateMutex.Unlock()

	for i := range items {
		items[i] = osutil.NormalizedFilename(items[i])
	}

	if err := s.db.removeLocalFiles([]byte(s.folder), items, s.meta); err != nil && !backend.IsClosed(err) {
		fatalError(err, opStr, s.db)
	}
}

type Snapshot struct {
	folder     string
	t          readOnlyTransaction
	meta       *countsMap
	fatalError func(error, string)
}

func (s *FileSet) Snapshot() (*Snapshot, error) {
	opStr := fmt.Sprintf("%s Snapshot()", s.folder)
	l.Debugf(opStr)
	t, err := s.db.newReadOnlyTransaction()
	if err != nil {
		s.db.handleFailure(err)
		return nil, err
	}
	return &Snapshot{
		folder: s.folder,
		t:      t,
		meta:   s.meta.Snapshot(),
		fatalError: func(err error, opStr string) {
			fatalError(err, opStr, s.db)
		},
	}, nil
}

func (s *Snapshot) Release() {
	s.t.close()
}

func (s *Snapshot) WithNeed(device protocol.DeviceID, fn Iterator) {
	opStr := fmt.Sprintf("%s WithNeed(%v)", s.folder, device)
	l.Debugf(opStr)
	if err := s.t.withNeed([]byte(s.folder), device[:], false, nativeFileIterator(fn)); err != nil && !backend.IsClosed(err) {
		s.fatalError(err, opStr)
	}
}

func (s *Snapshot) WithNeedTruncated(device protocol.DeviceID, fn Iterator) {
	opStr := fmt.Sprintf("%s WithNeedTruncated(%v)", s.folder, device)
	l.Debugf(opStr)
	if err := s.t.withNeed([]byte(s.folder), device[:], true, nativeFileIterator(fn)); err != nil && !backend.IsClosed(err) {
		s.fatalError(err, opStr)
	}
}

func (s *Snapshot) WithHave(device protocol.DeviceID, fn Iterator) {
	opStr := fmt.Sprintf("%s WithHave(%v)", s.folder, device)
	l.Debugf(opStr)
	if err := s.t.withHave([]byte(s.folder), device[:], nil, false, nativeFileIterator(fn)); err != nil && !backend.IsClosed(err) {
		s.fatalError(err, opStr)
	}
}

func (s *Snapshot) WithHaveTruncated(device protocol.DeviceID, fn Iterator) {
	opStr := fmt.Sprintf("%s WithHaveTruncated(%v)", s.folder, device)
	l.Debugf(opStr)
	if err := s.t.withHave([]byte(s.folder), device[:], nil, true, nativeFileIterator(fn)); err != nil && !backend.IsClosed(err) {
		s.fatalError(err, opStr)
	}
}

func (s *Snapshot) WithHaveSequence(startSeq int64, fn Iterator) {
	opStr := fmt.Sprintf("%s WithHaveSequence(%v)", s.folder, startSeq)
	l.Debugf(opStr)
	if err := s.t.withHaveSequence([]byte(s.folder), startSeq, nativeFileIterator(fn)); err != nil && !backend.IsClosed(err) {
		s.fatalError(err, opStr)
	}
}

// Except for an item with a path equal to prefix, only children of prefix are iterated.
// E.g. for prefix "dir", "dir/file" is iterated, but "dir.file" is not.
func (s *Snapshot) WithPrefixedHaveTruncated(device protocol.DeviceID, prefix string, fn Iterator) {
	opStr := fmt.Sprintf(`%s WithPrefixedHaveTruncated(%v, "%v")`, s.folder, device, prefix)
	l.Debugf(opStr)
	if err := s.t.withHave([]byte(s.folder), device[:], []byte(osutil.NormalizedFilename(prefix)), true, nativeFileIterator(fn)); err != nil && !backend.IsClosed(err) {
		s.fatalError(err, opStr)
	}
}

func (s *Snapshot) WithGlobal(fn Iterator) {
	opStr := fmt.Sprintf("%s WithGlobal()", s.folder)
	l.Debugf(opStr)
	if err := s.t.withGlobal([]byte(s.folder), nil, false, nativeFileIterator(fn)); err != nil && !backend.IsClosed(err) {
		s.fatalError(err, opStr)
	}
}

func (s *Snapshot) WithGlobalTruncated(fn Iterator) {
	opStr := fmt.Sprintf("%s WithGlobalTruncated()", s.folder)
	l.Debugf(opStr)
	if err := s.t.withGlobal([]byte(s.folder), nil, true, nativeFileIterator(fn)); err != nil && !backend.IsClosed(err) {
		s.fatalError(err, opStr)
	}
}

// Except for an item with a path equal to prefix, only children of prefix are iterated.
// E.g. for prefix "dir", "dir/file" is iterated, but "dir.file" is not.
func (s *Snapshot) WithPrefixedGlobalTruncated(prefix string, fn Iterator) {
	opStr := fmt.Sprintf(`%s WithPrefixedGlobalTruncated("%v")`, s.folder, prefix)
	l.Debugf(opStr)
	if err := s.t.withGlobal([]byte(s.folder), []byte(osutil.NormalizedFilename(prefix)), true, nativeFileIterator(fn)); err != nil && !backend.IsClosed(err) {
		s.fatalError(err, opStr)
	}
}

func (s *Snapshot) Get(device protocol.DeviceID, file string) (protocol.FileInfo, bool) {
	opStr := fmt.Sprintf("%s Get(%v)", s.folder, file)
	l.Debugf(opStr)
	f, ok, err := s.t.getFile([]byte(s.folder), device[:], []byte(osutil.NormalizedFilename(file)))
	if backend.IsClosed(err) {
		return protocol.FileInfo{}, false
	} else if err != nil {
		s.fatalError(err, opStr)
	}
	f.Name = osutil.NativeFilename(f.Name)
	return f, ok
}

func (s *Snapshot) GetGlobal(file string) (protocol.FileInfo, bool) {
	opStr := fmt.Sprintf("%s GetGlobal(%v)", s.folder, file)
	l.Debugf(opStr)
	_, fi, ok, err := s.t.getGlobal(nil, []byte(s.folder), []byte(osutil.NormalizedFilename(file)), false)
	if backend.IsClosed(err) {
		return protocol.FileInfo{}, false
	} else if err != nil {
		s.fatalError(err, opStr)
	}
	if !ok {
		return protocol.FileInfo{}, false
	}
	f := fi.(protocol.FileInfo)
	f.Name = osutil.NativeFilename(f.Name)
	return f, true
}

func (s *Snapshot) GetGlobalTruncated(file string) (FileInfoTruncated, bool) {
	opStr := fmt.Sprintf("%s GetGlobalTruncated(%v)", s.folder, file)
	l.Debugf(opStr)
	_, fi, ok, err := s.t.getGlobal(nil, []byte(s.folder), []byte(osutil.NormalizedFilename(file)), true)
	if backend.IsClosed(err) {
		return FileInfoTruncated{}, false
	} else if err != nil {
		s.fatalError(err, opStr)
	}
	if !ok {
		return FileInfoTruncated{}, false
	}
	f := fi.(FileInfoTruncated)
	f.Name = osutil.NativeFilename(f.Name)
	return f, true
}

func (s *Snapshot) Availability(file string) []protocol.DeviceID {
	opStr := fmt.Sprintf("%s Availability(%v)", s.folder, file)
	l.Debugf(opStr)
	av, err := s.t.availability([]byte(s.folder), []byte(osutil.NormalizedFilename(file)))
	if backend.IsClosed(err) {
		return nil
	} else if err != nil {
		s.fatalError(err, opStr)
	}
	return av
}

func (s *Snapshot) DebugGlobalVersions(file string) VersionList {
	opStr := fmt.Sprintf("%s DebugGlobalVersions(%v)", s.folder, file)
	l.Debugf(opStr)
	vl, err := s.t.getGlobalVersions(nil, []byte(s.folder), []byte(osutil.NormalizedFilename(file)))
	if backend.IsClosed(err) || backend.IsNotFound(err) {
		return VersionList{}
	} else if err != nil {
		s.fatalError(err, opStr)
	}
	return vl
}

func (s *Snapshot) Sequence(device protocol.DeviceID) int64 {
	return s.meta.Counts(device, 0).Sequence
}

// RemoteSequences returns a map of the sequence numbers seen for each
// remote device sharing this folder.
func (s *Snapshot) RemoteSequences() map[protocol.DeviceID]int64 {
	res := make(map[protocol.DeviceID]int64)
	for _, device := range s.meta.devices() {
		switch device {
		case protocol.EmptyDeviceID, protocol.LocalDeviceID, protocol.GlobalDeviceID:
			continue
		default:
			if seq := s.Sequence(device); seq > 0 {
				res[device] = seq
			}
		}
	}

	return res
}

func (s *Snapshot) LocalSize() Counts {
	local := s.meta.Counts(protocol.LocalDeviceID, 0)
	return local.Add(s.ReceiveOnlyChangedSize())
}

func (s *Snapshot) ReceiveOnlyChangedSize() Counts {
	return s.meta.Counts(protocol.LocalDeviceID, protocol.FlagLocalReceiveOnly)
}

func (s *Snapshot) GlobalSize() Counts {
	return s.meta.Counts(protocol.GlobalDeviceID, 0)
}

func (s *Snapshot) NeedSize(device protocol.DeviceID) Counts {
	return s.meta.Counts(device, needFlag)
}

func (s *Snapshot) WithBlocksHash(hash []byte, fn Iterator) {
	opStr := fmt.Sprintf(`%s WithBlocksHash("%x")`, s.folder, hash)
	l.Debugf(opStr)
	if err := s.t.withBlocksHash([]byte(s.folder), hash, nativeFileIterator(fn)); err != nil && !backend.IsClosed(err) {
		s.fatalError(err, opStr)
	}
}

func (s *FileSet) Sequence(device protocol.DeviceID) int64 {
	return s.meta.Sequence(device)
}

func (s *FileSet) IndexID(device protocol.DeviceID) protocol.IndexID {
	opStr := fmt.Sprintf("%s IndexID(%v)", s.folder, device)
	l.Debugf(opStr)
	id, err := s.db.getIndexID(device[:], []byte(s.folder))
	if backend.IsClosed(err) {
		return 0
	} else if err != nil {
		fatalError(err, opStr, s.db)
	}
	return id
}

func (s *FileSet) SetIndexID(device protocol.DeviceID, id protocol.IndexID) {
	if device == protocol.LocalDeviceID {
		panic("do not explicitly set index ID for local device")
	}
	opStr := fmt.Sprintf("%s SetIndexID(%v, %v)", s.folder, device, id)
	l.Debugf(opStr)
	if err := s.db.setIndexID(device[:], []byte(s.folder), id); err != nil && !backend.IsClosed(err) {
		fatalError(err, opStr, s.db)
	}
}

func (s *FileSet) MtimeOption() fs.Option {
	opStr := fmt.Sprintf("%s MtimeOption()", s.folder)
	l.Debugf(opStr)
	prefix, err := s.db.keyer.GenerateMtimesKey(nil, []byte(s.folder))
	if backend.IsClosed(err) {
		return nil
	} else if err != nil {
		fatalError(err, opStr, s.db)
	}
	kv := NewNamespacedKV(s.db, string(prefix))
	return fs.NewMtimeOption(kv)
}

func (s *FileSet) ListDevices() []protocol.DeviceID {
	return s.meta.devices()
}

func (s *FileSet) RepairSequence() (int, error) {
	s.updateAndGCMutexLock() // Ensures consistent locking order
	defer s.updateMutex.Unlock()
	defer s.db.gcMut.RUnlock()
	return s.db.repairSequenceGCLocked(s.folder, s.meta)
}

func (s *FileSet) updateAndGCMutexLock() {
	s.updateMutex.Lock()
	s.db.gcMut.RLock()
}

// DropFolder clears out all information related to the given folder from the
// database.
func DropFolder(db *Lowlevel, folder string) {
	opStr := fmt.Sprintf("DropFolder(%v)", folder)
	l.Debugf(opStr)
	droppers := []func([]byte) error{
		db.dropFolder,
		db.dropMtimes,
		db.dropFolderMeta,
		db.dropFolderIndexIDs,
		db.folderIdx.Delete,
	}
	for _, drop := range droppers {
		if err := drop([]byte(folder)); backend.IsClosed(err) {
			return
		} else if err != nil {
			fatalError(err, opStr, db)
		}
	}
}

// DropDeltaIndexIDs removes all delta index IDs from the database.
// This will cause a full index transmission on the next connection.
// Must be called before using FileSets, i.e. before NewFileSet is called for
// the first time.
func DropDeltaIndexIDs(db *Lowlevel) {
	select {
	case <-db.oneFileSetCreated:
		panic("DropDeltaIndexIDs must not be called after NewFileSet for the same Lowlevel")
	default:
	}
	opStr := "DropDeltaIndexIDs"
	l.Debugf(opStr)
	err := db.dropIndexIDs()
	if backend.IsClosed(err) {
		return
	} else if err != nil {
		fatalError(err, opStr, db)
	}
}

func normalizeFilenamesAndDropDuplicates(fs []protocol.FileInfo) []protocol.FileInfo {
	positions := make(map[string]int, len(fs))
	for i, f := range fs {
		norm := osutil.NormalizedFilename(f.Name)
		if pos, ok := positions[norm]; ok {
			fs[pos] = protocol.FileInfo{}
		}
		positions[norm] = i
		fs[i].Name = norm
	}
	for i := 0; i < len(fs); {
		if fs[i].Name == "" {
			fs = append(fs[:i], fs[i+1:]...)
			continue
		}
		i++
	}
	return fs
}

func nativeFileIterator(fn Iterator) Iterator {
	return func(fi protocol.FileIntf) bool {
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

func fatalError(err error, opStr string, db *Lowlevel) {
	db.checkErrorForRepair(err)
	l.Warnf("Fatal error: %v: %v", opStr, err)
	panic(ldbPathRe.ReplaceAllString(err.Error(), "$1 x: "))
}
