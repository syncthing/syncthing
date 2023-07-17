// Copyright (C) 2023 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/syncthing/syncthing/lib/protocol"
)

var (
	metricTotalOperationSeconds = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "syncthing",
		Subsystem: "fs",
		Name:      "operation_seconds_total",
		Help:      "Total time spent in FS operations",
	}, []string{"root", "operation"})
	metricTotalOperationsCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "syncthing",
		Subsystem: "fs",
		Name:      "operations_total",
		Help:      "Total number of FS operations",
	}, []string{"root", "operation"})
	metricTotalBytesCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "syncthing",
		Subsystem: "fs",
		Name:      "operation_bytes_total",
		Help:      "Total number of FS bytes",
	}, []string{"root", "operation"})
)

type metricsFS struct {
	next Filesystem
}

var _ Filesystem = (*metricsFS)(nil)

func (m *metricsFS) account(op string) func(bytes int) {
	t0 := time.Now()
	root := m.next.URI()
	return func(bytes int) {
		metricTotalOperationSeconds.WithLabelValues(root, op).Add(time.Since(t0).Seconds())
		metricTotalOperationsCount.WithLabelValues(root, op).Inc()
		if bytes >= 0 {
			metricTotalBytesCount.WithLabelValues(root, op).Add(float64(bytes))
		}
	}
}

func (m *metricsFS) Chmod(name string, mode FileMode) error {
	defer m.account("Chmod")(-1)
	return m.next.Chmod(name, mode)
}

func (m *metricsFS) Lchown(name string, uid, gid string) error {
	defer m.account("Lchown")(-1)
	return m.next.Lchown(name, uid, gid)
}

func (m *metricsFS) Chtimes(name string, atime time.Time, mtime time.Time) error {
	defer m.account("Chtimes")(-1)
	return m.next.Chtimes(name, atime, mtime)
}

func (m *metricsFS) Create(name string) (File, error) {
	defer m.account("Create")(-1)
	f, err := m.next.Create(name)
	if err != nil {
		return nil, err
	}
	return &metricsFile{next: f, fs: m}, nil
}

func (m *metricsFS) CreateSymlink(target, name string) error {
	defer m.account("CreateSymlink")(-1)
	return m.next.CreateSymlink(target, name)
}

func (m *metricsFS) DirNames(name string) ([]string, error) {
	defer m.account("DirNames")(-1)
	return m.next.DirNames(name)
}

func (m *metricsFS) Lstat(name string) (FileInfo, error) {
	defer m.account("Lstat")(-1)
	return m.next.Lstat(name)
}

func (m *metricsFS) Mkdir(name string, perm FileMode) error {
	defer m.account("Mkdir")(-1)
	return m.next.Mkdir(name, perm)
}

func (m *metricsFS) MkdirAll(name string, perm FileMode) error {
	defer m.account("MkdirAll")(-1)
	return m.next.MkdirAll(name, perm)
}

func (m *metricsFS) Open(name string) (File, error) {
	defer m.account("Open")(-1)
	f, err := m.next.Open(name)
	if err != nil {
		return nil, err
	}
	return &metricsFile{next: f, fs: m}, nil
}

func (m *metricsFS) OpenFile(name string, flags int, mode FileMode) (File, error) {
	defer m.account("OpenFile")(-1)
	f, err := m.next.OpenFile(name, flags, mode)
	if err != nil {
		return nil, err
	}
	return &metricsFile{next: f, fs: m}, nil
}

func (m *metricsFS) ReadSymlink(name string) (string, error) {
	defer m.account("ReadSymlink")(-1)
	return m.next.ReadSymlink(name)
}

func (m *metricsFS) Remove(name string) error {
	defer m.account("Remove")(-1)
	return m.next.Remove(name)
}

func (m *metricsFS) RemoveAll(name string) error {
	defer m.account("RemoveAll")(-1)
	return m.next.RemoveAll(name)
}

func (m *metricsFS) Rename(oldname, newname string) error {
	defer m.account("Rename")(-1)
	return m.next.Rename(oldname, newname)
}

func (m *metricsFS) Stat(name string) (FileInfo, error) {
	defer m.account("Stat")(-1)
	return m.next.Stat(name)
}

func (m *metricsFS) SymlinksSupported() bool {
	defer m.account("SymlinksSupported")(-1)
	return m.next.SymlinksSupported()
}

func (m *metricsFS) Walk(name string, walkFn WalkFunc) error {
	defer m.account("Walk")(-1)
	return m.next.Walk(name, walkFn)
}

func (m *metricsFS) Watch(path string, ignore Matcher, ctx context.Context, ignorePerms bool) (<-chan Event, <-chan error, error) {
	defer m.account("Watch")(-1)
	return m.next.Watch(path, ignore, ctx, ignorePerms)
}

func (m *metricsFS) Hide(name string) error {
	defer m.account("Hide")(-1)
	return m.next.Hide(name)
}

func (m *metricsFS) Unhide(name string) error {
	defer m.account("Unhide")(-1)
	return m.next.Unhide(name)
}

func (m *metricsFS) Glob(pattern string) ([]string, error) {
	defer m.account("Glob")(-1)
	return m.next.Glob(pattern)
}

func (m *metricsFS) Roots() ([]string, error) {
	defer m.account("Roots")(-1)
	return m.next.Roots()
}

func (m *metricsFS) Usage(name string) (Usage, error) {
	defer m.account("Usage")(-1)
	return m.next.Usage(name)
}

func (m *metricsFS) Type() FilesystemType {
	defer m.account("Type")(-1)
	return m.next.Type()
}

func (m *metricsFS) URI() string {
	defer m.account("URI")(-1)
	return m.next.URI()
}

func (m *metricsFS) Options() []Option {
	defer m.account("Options")(-1)
	return m.next.Options()
}

func (m *metricsFS) SameFile(fi1, fi2 FileInfo) bool {
	defer m.account("SameFile")(-1)
	return m.next.SameFile(fi1, fi2)
}

func (m *metricsFS) PlatformData(name string, withOwnership, withXattrs bool, xattrFilter XattrFilter) (protocol.PlatformData, error) {
	defer m.account("PlatformData")(-1)
	return m.next.PlatformData(name, withOwnership, withXattrs, xattrFilter)
}

func (m *metricsFS) GetXattr(name string, xattrFilter XattrFilter) ([]protocol.Xattr, error) {
	defer m.account("GetXattr")(-1)
	return m.next.GetXattr(name, xattrFilter)
}

func (m *metricsFS) SetXattr(path string, xattrs []protocol.Xattr, xattrFilter XattrFilter) error {
	defer m.account("SetXattr")(-1)
	return m.next.SetXattr(path, xattrs, xattrFilter)
}

func (m *metricsFS) underlying() (Filesystem, bool) {
	return m.next, true
}

func (m *metricsFS) wrapperType() filesystemWrapperType {
	return filesystemWrapperTypeMetrics
}

type metricsFile struct {
	fs   *metricsFS
	next File
}

func (m *metricsFile) Read(p []byte) (n int, err error) {
	acc := m.fs.account("Read")
	defer func() { acc(n) }()
	return m.next.Read(p)
}

func (m *metricsFile) ReadAt(p []byte, off int64) (n int, err error) {
	acc := m.fs.account("ReadAt")
	defer func() { acc(n) }()
	return m.next.ReadAt(p, off)
}

func (m *metricsFile) Seek(offset int64, whence int) (int64, error) {
	defer m.fs.account("Seek")(-1)
	return m.next.Seek(offset, whence)
}

func (m *metricsFile) Stat() (FileInfo, error) {
	defer m.fs.account("Stat")(-1)
	return m.next.Stat()
}

func (m *metricsFile) Sync() error {
	defer m.fs.account("Sync")(-1)
	return m.next.Sync()
}

func (m *metricsFile) Truncate(size int64) error {
	defer m.fs.account("Truncate")(-1)
	return m.next.Truncate(size)
}

func (m *metricsFile) Write(p []byte) (n int, err error) {
	acc := m.fs.account("Write")
	defer func() { acc(n) }()
	return m.next.Write(p)
}

func (m *metricsFile) WriteAt(p []byte, off int64) (n int, err error) {
	acc := m.fs.account("WriteAt")
	defer func() { acc(n) }()
	return m.next.WriteAt(p, off)
}

func (m *metricsFile) Close() error {
	defer m.fs.account("Close")(-1)
	return m.next.Close()
}

func (m *metricsFile) Name() string {
	defer m.fs.account("Name")(-1)
	return m.next.Name()
}

func (m *metricsFile) underlying() (File, bool) {
	return m.next, true
}
