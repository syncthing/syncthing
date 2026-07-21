// Copyright (C) 2026 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/syncthing/syncthing/internal/slogutil"
	"github.com/syncthing/syncthing/lib/protocol"
)

const FilesystemTypeRemote FilesystemType = "remote"

var (
	errRemoteNotImpl        = errors.New("remotefs: not implemented")
	errRemoteReadOnly       = fmt.Errorf("remotefs: %w (read-only filesystem)", os.ErrPermission)
	errRemoteNoAvailability = fmt.Errorf("remotefs: %w (no remote device has this file available)", os.ErrNotExist)
)

// GlobalIndexer is the subset of db.DB we need.
type GlobalIndexer interface {
	GetGlobalFile(folder string, file string) (protocol.FileInfo, bool, error)
	GetGlobalAvailability(folder, file string) ([]protocol.DeviceID, error)
}

// GlobalRequester is the subset of model.Model we need.
type GlobalRequester interface {
	RequestGlobal(ctx context.Context, deviceID protocol.DeviceID, folder, name string, blockNo int, offset int64, size int, hash []byte, fromTemporary bool) ([]byte, error)
}

// NewRemoteFilesystem constructs a filesystem whose file metadata are read
// from the global state stored in idx for the given folder, and whose file
// contents are fetched from connected remote devices via req. The supplied
// context bounds the lifetime of the filesystem; cancelling it aborts
// in-flight reads.
//
// openTimeout caps the wall-clock duration of a single open operation
// (covering the lookup & all block requests); a non-positive value disables
// the per-open timeout and only the lifetime context applies.
//
// The returned filesystem is read-only: any state-changing operation
// returns a permission error. File reads are buffered entirely in memory.
// Use accordingly.
func NewRemoteFilesystem(ctx context.Context, idx GlobalIndexer, req GlobalRequester, folderID string, openTimeout time.Duration) Filesystem {
	return &remoteFilesystem{
		ctx:         ctx,
		idx:         idx,
		req:         req,
		folderId:    folderID,
		openTimeout: openTimeout,
	}
}

type remoteFilesystem struct {
	ctx         context.Context //nolint:containedctx
	idx         GlobalIndexer
	req         GlobalRequester
	folderId    string
	openTimeout time.Duration
}

func (f *remoteFilesystem) Lstat(name string) (FileInfo, error) {
	pf, err := f.globalFile(name)
	if err != nil {
		return nil, err
	}
	return &remoteFileInfo{info: pf}, nil
}

func (f *remoteFilesystem) Stat(name string) (FileInfo, error) {
	return f.Lstat(name)
}

func (f *remoteFilesystem) Open(name string) (File, error) {
	return f.openForReading(name)
}

func (f *remoteFilesystem) OpenFile(name string, flags int, _ FileMode) (File, error) {
	// Only read-only opens are accepted; any flag that would imply a write
	// returns a permission error.
	const writeFlags = OptWriteOnly | OptReadWrite | OptCreate | OptTruncate | OptAppend | OptExclusive
	if flags&writeFlags != 0 {
		return nil, errRemoteReadOnly
	}
	return f.openForReading(name)
}

func (*remoteFilesystem) Type() FilesystemType { return FilesystemTypeRemote }

func (f *remoteFilesystem) URI() string { return "remote://" + f.folderId }

func (f *remoteFilesystem) Options() []Option { return nil }

func (f *remoteFilesystem) globalFile(name string) (protocol.FileInfo, error) {
	name, err := Canonicalize(name)
	if err != nil {
		return protocol.FileInfo{}, err
	}

	if name == "." {
		// The folder root has no entry of its own in the database; report
		// it as a synthetic directory.
		return protocol.FileInfo{
			Name:        ".",
			Type:        protocol.FileInfoTypeDirectory,
			Permissions: 0o444,
		}, nil
	}

	pf, ok, err := f.idx.GetGlobalFile(f.folderId, name)
	if err != nil {
		return protocol.FileInfo{}, err
	}
	if !ok || pf.IsDeleted() {
		return protocol.FileInfo{}, ErrNotExist
	}
	return pf, nil
}

// openForReading fetches the named regular file's blocks from a remote
// device and returns a buffered file. The work is bounded by the
// per-open timeout configured at construction (if any), layered on top
// of the filesystem's lifetime context.
func (f *remoteFilesystem) openForReading(name string) (File, error) {
	slog.DebugContext(f.ctx, "Opening remote file", slog.String("folder", f.folderId), slog.String("name", name))

	pf, err := f.globalFile(name)
	if err != nil {
		slog.DebugContext(f.ctx, "Failed to look up remote file", slog.String("folder", f.folderId), slog.String("name", name), slogutil.Error(err))
		return nil, err
	}
	if pf.Type != protocol.FileInfoTypeFile {
		// Matches the convention used by fakeFS: opening anything other
		// than a regular file looks like a missing file to callers.
		return nil, ErrNotExist
	}

	ctx, cancel := f.openContext()
	defer cancel()

	content, err := f.fetchBlocks(ctx, pf)
	if err != nil {
		slog.DebugContext(f.ctx, "Failed to fetch remote file blocks", slog.String("folder", f.folderId), pf.LogAttr(), slogutil.Error(err))
		return nil, err
	}

	file := &remoteFile{
		info:   pf,
		reader: bytes.NewReader(content),
	}

	return file, nil
}

func (f *remoteFilesystem) openContext() (context.Context, context.CancelFunc) {
	if f.openTimeout > 0 {
		return context.WithTimeout(f.ctx, f.openTimeout)
	}
	return context.WithCancel(f.ctx)
}

// fetchBlocks downloads every block of pf from a remote device that has the
// global version available, concatenating the result. For each block the
// available devices are tried in order; devices that fail to provide a
// block are removed from the list for next iteration.
//
// This is a deliberately naive implementation that optimises for small
// files available in their entirety from a single device, often consisting
// of a single block.
func (f *remoteFilesystem) fetchBlocks(ctx context.Context, pf protocol.FileInfo) ([]byte, error) {
	if len(pf.Blocks) == 0 {
		return nil, nil
	}

	devices, err := f.idx.GetGlobalAvailability(f.folderId, pf.Name)
	if err != nil {
		return nil, err
	}
	if len(devices) == 0 {
		return nil, errRemoteNoAvailability
	}

	buf := make([]byte, 0, pf.Size)
	skip := make(map[protocol.DeviceID]struct{})
	for idx, block := range pf.Blocks {
		var err error
		for _, dev := range devices {
			if _, ok := skip[dev]; ok {
				// skip devices that failed to provide a previous block, as
				// it is likely offline or experiencing some issue
				continue
			}
			var data []byte
			data, err = f.req.RequestGlobal(ctx, dev, f.folderId, pf.Name, idx, block.Offset, block.Size, block.Hash, false)
			if err != nil {
				skip[dev] = struct{}{}
				continue
			}
			buf = append(buf, data...)
		}
		if err != nil {
			// No device had the block
			return nil, err
		}
	}
	return buf, nil
}

func (*remoteFilesystem) Chmod(_ string, _ FileMode) error { return errRemoteReadOnly }

func (*remoteFilesystem) Lchown(_, _, _ string) error { return errRemoteReadOnly }

func (*remoteFilesystem) Chtimes(_ string, _ time.Time, _ time.Time) error { return errRemoteReadOnly }

func (*remoteFilesystem) Create(_ string) (File, error) { return nil, errRemoteReadOnly }

func (*remoteFilesystem) CreateSymlink(_, _ string) error { return errRemoteReadOnly }

func (*remoteFilesystem) Mkdir(_ string, _ FileMode) error { return errRemoteReadOnly }

func (*remoteFilesystem) MkdirAll(_ string, _ FileMode) error { return errRemoteReadOnly }

func (*remoteFilesystem) Remove(_ string) error { return errRemoteReadOnly }

func (*remoteFilesystem) RemoveAll(_ string) error { return errRemoteReadOnly }

func (*remoteFilesystem) Rename(_, _ string) error { return errRemoteReadOnly }

func (*remoteFilesystem) Hide(_ string) error { return errRemoteReadOnly }

func (*remoteFilesystem) Unhide(_ string) error { return errRemoteReadOnly }

func (*remoteFilesystem) SetXattr(_ string, _ []protocol.Xattr, _ XattrFilter) error {
	return errRemoteReadOnly
}

func (*remoteFilesystem) DirNames(_ string) ([]string, error) { return nil, errRemoteNotImpl }

func (*remoteFilesystem) ReadSymlink(_ string) (string, error) { return "", errRemoteNotImpl }

func (*remoteFilesystem) Walk(_ string, _ WalkFunc) error { return errRemoteNotImpl }

func (*remoteFilesystem) Glob(_ string) ([]string, error) { return nil, errRemoteNotImpl }

func (*remoteFilesystem) Roots() ([]string, error) { return nil, errRemoteNotImpl }

func (*remoteFilesystem) Usage(_ string) (Usage, error) { return Usage{}, errRemoteNotImpl }

func (*remoteFilesystem) PlatformData(_ string, _, _ bool, _ XattrFilter) (protocol.PlatformData, error) {
	return protocol.PlatformData{}, errRemoteNotImpl
}

func (*remoteFilesystem) GetXattr(_ string, _ XattrFilter) ([]protocol.Xattr, error) {
	return nil, errRemoteNotImpl
}

func (*remoteFilesystem) SymlinksSupported() bool { return false }

func (*remoteFilesystem) Watch(_ string, _ Matcher, _ context.Context, _ bool) (<-chan Event, <-chan error, error) {
	return nil, nil, ErrWatchNotSupported
}

func (*remoteFilesystem) SameFile(_, _ FileInfo) bool { return false }

type remoteFile struct {
	info   protocol.FileInfo
	reader *bytes.Reader
}

func (f *remoteFile) Name() string { return filepath.Base(f.info.Name) }

func (f *remoteFile) Read(p []byte) (int, error) { return f.reader.Read(p) }

func (f *remoteFile) ReadAt(p []byte, off int64) (int, error) { return f.reader.ReadAt(p, off) }

func (f *remoteFile) Seek(off int64, whence int) (int64, error) { return f.reader.Seek(off, whence) }

func (f *remoteFile) Stat() (FileInfo, error) { return &remoteFileInfo{info: f.info}, nil }

func (*remoteFile) Close() error { return nil }

func (*remoteFile) Write(_ []byte) (int, error) { return 0, errRemoteReadOnly }

func (*remoteFile) WriteAt(_ []byte, _ int64) (int, error) { return 0, errRemoteReadOnly }

func (*remoteFile) Truncate(_ int64) error { return errRemoteReadOnly }

func (*remoteFile) Sync() error { return errRemoteReadOnly }

type remoteFileInfo struct {
	info protocol.FileInfo
}

func (fi *remoteFileInfo) Name() string { return filepath.Base(fi.info.Name) }

func (fi *remoteFileInfo) Mode() FileMode {
	mode := FileMode(fi.info.Permissions & 0o777)
	switch {
	case fi.info.IsDirectory():
		mode |= FileMode(os.ModeDir)
	case fi.info.IsSymlink():
		mode |= FileMode(os.ModeSymlink)
	}
	return mode
}

func (fi *remoteFileInfo) Size() int64 { return fi.info.Size }

func (fi *remoteFileInfo) ModTime() time.Time { return fi.info.ModTime() }

func (fi *remoteFileInfo) IsDir() bool { return fi.info.IsDirectory() }

func (*remoteFileInfo) Sys() any { return nil }

func (fi *remoteFileInfo) IsRegular() bool { return fi.info.Type == protocol.FileInfoTypeFile }

func (fi *remoteFileInfo) IsSymlink() bool { return fi.info.IsSymlink() }

func (*remoteFileInfo) Owner() int { return -1 }

func (*remoteFileInfo) Group() int { return -1 }
