// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"bytes"
	"context"
	"io"
	"log"
	"os"
	"path"
	"syscall"
	"time"

	ffs "github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/syncthing/syncthing/lib/blockstorage"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/ignore"
	"github.com/syncthing/syncthing/lib/logger"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/scanner"
	"github.com/syncthing/syncthing/lib/semaphore"
	"github.com/syncthing/syncthing/lib/stats"
	"github.com/syncthing/syncthing/lib/sync"
	"github.com/syncthing/syncthing/lib/versioner"
	"golang.org/x/exp/constraints"
)

func init() {
	folderFactories[config.FolderTypeVirtual] = newVirtualFolder
	log.SetFlags(log.Lmicroseconds)
	log.Default().SetOutput(os.Stdout)
	log.Default().SetPrefix("TESTLOG ")
}

type virtualFolderSyncthingService struct {
	*folderBase
	blockCache   blockstorage.HashBlockStorageI
	mountService io.Closer

	backgroundDownloadPending chan struct{}
	backgroundDownloadQueue   jobQueue
}

func (vFSS *virtualFolderSyncthingService) GetBlockDataFromCacheOrDownload(
	snap *db.Snapshot,
	file protocol.FileInfo,
	block protocol.BlockInfo,
) ([]byte, bool) {
	data, ok := vFSS.blockCache.Get(block.Hash)
	if !ok {
		err := vFSS.pullBlockBase(func(blockData []byte) {
			data = blockData
		}, snap, file, block)

		if err != nil {
			return nil, false
		}

		vFSS.blockCache.Set(block.Hash, data)
	}

	return data, true
}

type syncthingVirtualFolderFuseAdapter struct {
	vFSS     *virtualFolderSyncthingService
	folderID string
	model    *model
	fset     *db.FileSet

	// ino mapping
	ino_mu      sync.Mutex
	next_ino_nr uint64
	ino_mapping map[string]uint64
}

func (r *syncthingVirtualFolderFuseAdapter) getInoOf(path string) uint64 {
	r.ino_mu.Lock()
	defer r.ino_mu.Unlock()
	ino, ok := r.ino_mapping[path]
	if !ok {
		ino = r.next_ino_nr
		r.next_ino_nr += 1
		r.ino_mapping[path] = ino
	}
	return ino
}

func (stf *syncthingVirtualFolderFuseAdapter) lookupFile(path string) (info *db.FileInfoTruncated, eno syscall.Errno) {
	snap, err := stf.fset.Snapshot()
	if err != nil {
		//stf..log()
		return nil, syscall.EFAULT
	}

	fi, ok := snap.GetGlobalTruncated(path)
	if !ok {
		return nil, syscall.ENOENT
	}

	return &fi, 0
}

func createNewVirtualFileInfo(creator protocol.ShortID, Permissions *uint32, name string) protocol.FileInfo {

	creationTime := time.Now()
	fi := protocol.FileInfo{}
	fi.Name = name
	// fi.Size =
	fi.ModifiedS = creationTime.Unix()
	fi.ModifiedBy = creator
	fi.Version = fi.Version.Update(creator)
	// fi.Sequence =
	fi.Blocks = append([]protocol.BlockInfo{}, protocol.BlockInfo{Size: 0})
	// fi.SymlinkTarget =
	// BlocksHash
	// Encrypted
	fi.Type = protocol.FileInfoTypeFile
	if Permissions != nil {
		fi.Permissions = *Permissions
	}
	fi.ModifiedNs = creationTime.Nanosecond()
	// fi.RawBlockSize
	// fi.Platform
	// fi.LocalFlags
	// fi.VersionHash
	fi.InodeChangeNs = creationTime.UnixNano()
	// EncryptionTrailerSize
	// Deleted
	// RawInvalid
	fi.NoPermissions = Permissions == nil

	return fi
}

func (stf *syncthingVirtualFolderFuseAdapter) createFile(
	Permissions *uint32, name string,
) (info *db.FileInfoTruncated, eno syscall.Errno) {

	fi := createNewVirtualFileInfo(stf.model.shortID, Permissions, name)
	stf.fset.UpdateOne(protocol.LocalDeviceID, &fi)

	snap, err := stf.fset.Snapshot()
	if err != nil {
		return nil, syscall.EAGAIN
	}

	db_fi, ok := snap.GetGlobalTruncated(name)
	if !ok {
		return nil, syscall.ENOENT
	}

	return &db_fi, 0
}

func calculateHashForBlock(ctx context.Context, blockData []byte,
) (bi protocol.BlockInfo, err error) {
	blockInfos, err := scanner.Blocks(
		ctx, bytes.NewReader(blockData), len(blockData), int64(len(blockData)), nil, true)
	if err != nil {
		return protocol.BlockInfo{}, err // Context done
	}
	if len(blockInfos) != 1 {
		panic("internal error: output length not as expected!")
	}
	return blockInfos[0], nil
}

func (stf *syncthingVirtualFolderFuseAdapter) writeFile(
	ctx context.Context, name string, offset uint64, inputData []byte,
) syscall.Errno {
	snap, err := stf.fset.Snapshot()
	if err != nil {
		//stf..log()
		return syscall.EFAULT
	}

	fi, ok := snap.GetGlobal(name)
	if !ok {
		return syscall.ENOENT
	}

	if fi.RawBlockSize == 0 {
		fi.RawBlockSize = protocol.MinBlockSize
	}

	if fi.Blocks == nil {
		fi.Blocks = []protocol.BlockInfo{}
	}

	inputPos := 0
	blockIdx := int(offset / uint64(fi.RawBlockSize))
	writeStartInBlock := int(offset % uint64(fi.RawBlockSize))

	if blockIdx >= (len(fi.Blocks) + 1 /* appending one block is OK */) {
		return syscall.EINVAL
	}

	for {
		writeEndInBlock := clamp(writeStartInBlock+len(inputData), writeStartInBlock, fi.RawBlockSize)
		writeLenInBlock := writeEndInBlock - writeStartInBlock

		ok := false
		var blockData = []byte{}
		if blockIdx < len(fi.Blocks) {
			bi := fi.Blocks[blockIdx]
			blockData, ok = stf.vFSS.blockCache.Get(bi.Hash)
		}
		if !ok {
			// allocate new block:
			blockData = make([]byte, writeEndInBlock)
		}

		inputPosNext := inputPos + writeLenInBlock
		copy(blockData[writeStartInBlock:writeEndInBlock], inputData[inputPos:inputPosNext])

		biNew, err := calculateHashForBlock(ctx, blockData)
		if err != nil {
			return syscall.ECONNABORTED
		}

		// offset needs to be corrected as not the full file was re-calculated
		biNew.Offset = int64(blockIdx) * int64(fi.RawBlockSize)
		if blockIdx < len(fi.Blocks) {
			fi.Blocks[blockIdx] = biNew
		} else {
			fi.Blocks = append(fi.Blocks, biNew)
		}
		writeEndInFile := biNew.Offset + int64(writeEndInBlock)
		if writeEndInFile > fi.Size {
			fi.Size = writeEndInFile
		}
		changeTime := time.Now()
		fi.ModifiedBy = stf.model.shortID
		fi.ModifiedS = changeTime.Unix()
		fi.ModifiedNs = changeTime.Nanosecond()
		fi.InodeChangeNs = changeTime.UnixNano()
		fi.Version = fi.Version.Update(fi.ModifiedBy)

		stf.vFSS.blockCache.Set(biNew.Hash, blockData)
		stf.fset.UpdateOne(protocol.LocalDeviceID, &fi)

		blockIdx += 1
		inputPos = inputPosNext
		writeStartInBlock = 0

		if inputPosNext >= len(inputData) {
			return ffs.OK
		}
	}
}

func (stf *syncthingVirtualFolderFuseAdapter) deleteFile(ctx context.Context, path string) syscall.Errno {

	snap, err := stf.fset.Snapshot()
	if err != nil {
		//stf..log()
		return syscall.EFAULT
	}

	fi, ok := snap.GetGlobal(path)
	if !ok {
		return syscall.ENOENT
	}

	fi.ModifiedBy = stf.model.shortID
	fi.Deleted = true
	fi.Size = 0
	fi.Blocks = []protocol.BlockInfo{}
	fi.Version = fi.Version.Update(stf.model.shortID)
	stf.fset.UpdateOne(protocol.LocalDeviceID, &fi)

	return 0
}

func newVirtualFolder(
	model *model,
	fset *db.FileSet,
	ignores *ignore.Matcher,
	cfg config.FolderConfiguration,
	ver versioner.Versioner,
	evLogger events.Logger,
	ioLimiter *semaphore.Semaphore,
) service {
	return &virtualFolderSyncthingService{
		folderBase:                newFolderBase(cfg, evLogger, model, fset),
		blockCache:                nil,
		backgroundDownloadPending: make(chan struct{}, 1),
		backgroundDownloadQueue:   *newJobQueue(),
	}
}

func (f *virtualFolderSyncthingService) RequestBackgroundDownload(filename string, size int64, modified time.Time) {
	wasNew := f.backgroundDownloadQueue.PushIfNew(filename, size, modified)
	if !wasNew {
		return
	}

	f.backgroundDownloadQueue.SortAccordingToConfig(f.Order)
	select {
	case f.backgroundDownloadPending <- struct{}{}:
	default:
	}
}

func (f *virtualFolderSyncthingService) Serve_backgroundDownloadTask() {
	for {

		select {
		case <-f.ctx.Done():
			return
		case <-f.backgroundDownloadPending:
		}

		for job, ok := f.backgroundDownloadQueue.Pop(); ok; job, ok = f.backgroundDownloadQueue.Pop() {
			func() {
				defer f.backgroundDownloadQueue.Done(job)

				snap, err := f.fset.Snapshot()
				if err != nil {
					return
				}
				fi, ok := snap.GetGlobal(job)
				if !ok {
					return
				}

				all_ok := true
				for _, bi := range fi.Blocks {
					_, ok := f.GetBlockDataFromCacheOrDownload(snap, fi, bi)
					all_ok = all_ok && ok
				}

				if !all_ok {
					return
				}

				f.fset.UpdateOne(protocol.LocalDeviceID, &fi)

				seq := f.fset.Sequence(protocol.LocalDeviceID)
				f.evLogger.Log(events.LocalIndexUpdated, map[string]interface{}{
					"folder":    f.ID,
					"items":     1,
					"filenames": append([]string(nil), fi.Name),
					"sequence":  seq,
					"version":   seq, // legacy for sequence
				})
			}()
		}
	}
}

func (f *virtualFolderSyncthingService) Serve(ctx context.Context) error {
	f.model.foldersRunning.Add(1)
	defer f.model.foldersRunning.Add(-1)

	f.ctx = ctx
	//f.blockCache = blockstorage.NewGoCloudUrlStorage(ctx, "mem://")

	myDir := f.Path + "Blobs"
	if err := os.MkdirAll(myDir, 0o777); err != nil {
		log.Fatal(err)
	}

	f.blockCache = blockstorage.NewGoCloudUrlStorage(ctx, "file://"+myDir+"?no_tmp_dir=yes")

	if f.mountService == nil {
		stVF := &syncthingVirtualFolderFuseAdapter{
			vFSS:        f,
			folderID:    f.ID,
			model:       f.model,
			fset:        f.fset,
			ino_mu:      sync.NewMutex(),
			next_ino_nr: 1,
			ino_mapping: make(map[string]uint64),
		}
		mount, err := NewVirtualFolderMount(f.Path, f.ID, f.Label, stVF)
		if err != nil {
			return err
		}

		f.mountService = mount
	}

	backgroundDownloadTasks := 4
	for i := 0; i < backgroundDownloadTasks; i++ {
		go f.Serve_backgroundDownloadTask()
	}

	for {
		select {
		case <-ctx.Done():
			f.mountService.Close()
			return nil

		case <-f.pullScheduled:
			continue

		}
	}
}

func (f *virtualFolderSyncthingService) Override()                 {}
func (f *virtualFolderSyncthingService) Revert()                   {}
func (f *virtualFolderSyncthingService) DelayScan(d time.Duration) {}
func (vf *virtualFolderSyncthingService) ScheduleScan() {
	vf.Scan([]string{})
}
func (f *virtualFolderSyncthingService) Jobs(page, per_page int) ([]string, []string, int) {
	return f.backgroundDownloadQueue.Jobs(page, per_page)
}
func (f *virtualFolderSyncthingService) BringToFront(filename string) {
	f.backgroundDownloadQueue.BringToFront(filename)
}

func (vf *virtualFolderSyncthingService) Scan(subs []string) error {
	snap, err := vf.fset.Snapshot()
	if err != nil {
		return err
	}

	snap.WithNeedTruncated(protocol.LocalDeviceID, func(f protocol.FileIntf) bool /* true to continue */ {
		if f.IsDirectory() {
			// no work to do for directories. directly take over:
			fi, ok := snap.GetGlobal(f.FileName())
			if ok {
				vf.fset.UpdateOne(protocol.LocalDeviceID, &fi)
			}
		} else {
			vf.RequestBackgroundDownload(f.FileName(), f.FileSize(), f.ModTime())
		}
		return true
	})

	return nil
}
func (f *virtualFolderSyncthingService) Errors() []FileError             { return []FileError{} }
func (f *virtualFolderSyncthingService) WatchError() error               { return nil }
func (f *virtualFolderSyncthingService) ScheduleForceRescan(path string) {}
func (f *virtualFolderSyncthingService) GetStatistics() (stats.FolderStatistics, error) {
	return stats.FolderStatistics{}, nil
}

type VirtualFolderDirStream struct {
	root     *syncthingVirtualFolderFuseAdapter
	dirPath  string
	children []*TreeEntry
	i        int
}

func (s *VirtualFolderDirStream) HasNext() bool {
	return s.i < len(s.children)
}
func (s *VirtualFolderDirStream) Next() (fuse.DirEntry, syscall.Errno) {
	if !s.HasNext() {
		return fuse.DirEntry{}, syscall.ENOENT
	}

	child := s.children[s.i]
	s.i += 1

	mode := syscall.S_IFREG
	switch child.Type {
	case protocol.FileInfoTypeDirectory:
		mode = syscall.S_IFDIR
	default:
		break
	}

	return fuse.DirEntry{
		Mode: uint32(mode),
		Name: child.Name,
		Ino:  s.root.getInoOf(path.Join(s.dirPath, child.Name)),
	}, 0
}
func (s *VirtualFolderDirStream) Close() {}

func (f *syncthingVirtualFolderFuseAdapter) readDir(path string) (stream ffs.DirStream, eno syscall.Errno) {

	//	snap, err := f.fset.Snapshot()
	//	if err != nil {
	//		return nil, syscall.EFAULT
	//	}
	//
	//	if path == "" {
	//		f.model.GlobalDirectoryTree()
	//	}
	//
	//	fi, ok := snap.GetGlobalTruncated(path)
	//	if !ok {
	//		return nil, syscall.ENOENT
	//	}
	//
	//	if !fi.IsDirectory() {
	//		return nil, syscall.ENOTDIR
	//	}

	children, err := f.model.GlobalDirectoryTree(f.folderID, path, 1, false)
	if err != nil {
		return nil, syscall.EFAULT
	}

	return &VirtualFolderDirStream{
		root:     f,
		dirPath:  path,
		children: children,
	}, 0
}

type VirtualFileReadResult struct {
	f           *syncthingVirtualFolderFuseAdapter
	snap        *db.Snapshot
	fi          *protocol.FileInfo
	offset      uint64
	maxToBeRead int
}

func clamp[T constraints.Ordered](a, min, max T) T {
	if min > max {
		panic("clamp: min > max is not allowed")
	}
	if a > max {
		return max
	}
	if a < min {
		return min
	}
	return a
}

func (vf *VirtualFileReadResult) readOneBlock(offset uint64, remainingToRead int) ([]byte, fuse.Status) {

	blockSize := vf.fi.BlockSize()
	blockIndex := int(offset / uint64(blockSize))

	logger.DefaultLogger.Infof(
		"VirtualFileReadResult readOneBlock(offset, len): %v, %v. bSize, bIdx: %v, %v",
		offset, remainingToRead, blockSize, blockIndex)

	if blockIndex >= len(vf.fi.Blocks) {
		return nil, 0
	}

	block := vf.fi.Blocks[blockIndex]

	rel_pos := int64(offset) - block.Offset
	if rel_pos < 0 {
		return nil, 0
	}

	inputData, ok := vf.f.vFSS.GetBlockDataFromCacheOrDownload(vf.snap, *vf.fi, block)
	if !ok {
		return nil, fuse.Status(syscall.EAGAIN)
	}

	remainingInBlock := clamp(len(inputData)-int(rel_pos), 0, len(inputData))
	maxToBeRead := clamp(remainingToRead, 0, vf.maxToBeRead)
	readAmount := clamp(remainingInBlock, 0, maxToBeRead)

	if readAmount != 0 {
		return inputData[rel_pos : rel_pos+int64(readAmount)], 0
	} else {
		return nil, 0
	}
}

func (vf *VirtualFileReadResult) Bytes(outBuf []byte) ([]byte, fuse.Status) {

	logger.DefaultLogger.Infof("VirtualFileReadResult Bytes(len): %v", len(outBuf))

	outBufSize := len(outBuf)
	initialReadData, status := vf.readOneBlock(vf.offset, outBufSize)
	if status != 0 {
		return nil, status
	}

	nextOutBufWriteBegin := len(initialReadData)
	if nextOutBufWriteBegin >= outBufSize {
		// done in one step
		return initialReadData, 0
	}

	copy(outBuf, initialReadData)

	for nextOutBufWriteBegin < outBufSize {
		remainingToBeRead := outBufSize - nextOutBufWriteBegin
		nextReadData, status := vf.readOneBlock(vf.offset+uint64(nextOutBufWriteBegin), remainingToBeRead)
		if status != 0 {
			return nil, status
		}
		if len(nextReadData) == 0 {
			break
		}
		readLen := copy(outBuf[nextOutBufWriteBegin:], nextReadData)
		nextOutBufWriteBegin += readLen
	}

	if nextOutBufWriteBegin != outBufSize {
		logger.DefaultLogger.Infof("Read incomplete: %d/%d", nextOutBufWriteBegin, len(outBuf))
	}

	if nextOutBufWriteBegin != 0 {
		vf.f.vFSS.RequestBackgroundDownload(vf.fi.Name, vf.fi.Size, vf.fi.ModTime())
		return outBuf[:nextOutBufWriteBegin], 0
	} else {
		return nil, 0
	}
}
func (vf *VirtualFileReadResult) Size() int {
	return clamp(vf.maxToBeRead, 0, int(vf.fi.Size-int64(vf.offset)))
}
func (vf *VirtualFileReadResult) Done() {}

func (f *syncthingVirtualFolderFuseAdapter) readFile(
	path string, buf []byte, off int64,
) (res fuse.ReadResult, errno syscall.Errno) {
	snap, err := f.fset.Snapshot()
	if err != nil {
		//stf..log()
		return nil, syscall.EFAULT
	}

	fi, ok := snap.GetGlobal(path)
	if !ok {
		return nil, syscall.ENOENT
	}

	return &VirtualFileReadResult{
		f:           f,
		snap:        snap,
		fi:          &fi,
		offset:      uint64(off),
		maxToBeRead: len(buf),
	}, 0
}

var _ = (virtualFolderServiceI)((*virtualFolderSyncthingService)(nil))

func (vf *virtualFolderSyncthingService) GetHashBlockData(hash []byte, response_data []byte) (int, error) {
	data, ok := vf.blockCache.Get(hash)
	if !ok {
		return 0, protocol.ErrNoSuchFile
	}
	n := copy(response_data, data)
	return n, nil
}
