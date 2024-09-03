// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"context"
	"io"
	"log"
	"os"
	"path"
	"syscall"
	"time"

	ffs "github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/ignore"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/semaphore"
	"github.com/syncthing/syncthing/lib/stats"
	"github.com/syncthing/syncthing/lib/sync"
	"github.com/syncthing/syncthing/lib/versioner"
)

func init() {
	folderFactories[config.FolderTypeVirtual] = newVirtualFolder
	log.SetFlags(log.Lmicroseconds)
	log.Default().SetOutput(os.Stdout)
	log.Default().SetPrefix("TESTLOG ")
}

type virtualFolderSyncthingService struct {
	*folderBase
	mountService io.Closer
}

type syncthingVirtualFolderFuseAdapter struct {
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
		folderBase: newFolderBase(cfg, evLogger, model, fset),
	}
}

func (f *virtualFolderSyncthingService) Serve(ctx context.Context) error {
	f.model.foldersRunning.Add(1)
	defer f.model.foldersRunning.Add(-1)

	if f.mountService == nil {
		stVF := &syncthingVirtualFolderFuseAdapter{
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

func (f *virtualFolderSyncthingService) BringToFront(string)       {}
func (f *virtualFolderSyncthingService) Override()                 {}
func (f *virtualFolderSyncthingService) Revert()                   {}
func (f *virtualFolderSyncthingService) DelayScan(d time.Duration) {}
func (f *virtualFolderSyncthingService) ScheduleScan()             {}
func (f *virtualFolderSyncthingService) Jobs(page, perpage int) ([]string, []string, int) {
	return []string{}, []string{}, 0
}
func (f *virtualFolderSyncthingService) Scan(subs []string) error        { return nil }
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
