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
)

type metricsFS struct {
	next Filesystem
}

var _ Filesystem = (*metricsFS)(nil)

func (m *metricsFS) account(op string) func() {
	t0 := time.Now()
	root := m.next.URI()
	return func() {
		metricTotalOperationSeconds.WithLabelValues(root, op).Add(time.Since(t0).Seconds())
		metricTotalOperationsCount.WithLabelValues(root, op).Inc()
	}
}

func (m *metricsFS) Chmod(name string, mode FileMode) error {
	defer m.account("Chmod")()
	return m.next.Chmod(name, mode)
}

func (m *metricsFS) Lchown(name string, uid, gid string) error {
	defer m.account("Lchown")()
	return m.next.Lchown(name, uid, gid)
}

func (m *metricsFS) Chtimes(name string, atime time.Time, mtime time.Time) error {
	defer m.account("Chtimes")()
	return m.next.Chtimes(name, atime, mtime)
}

func (m *metricsFS) Create(name string) (File, error) {
	defer m.account("Create")()
	return m.next.Create(name)
}

func (m *metricsFS) CreateSymlink(target, name string) error {
	defer m.account("CreateSymlink")()
	return m.next.CreateSymlink(target, name)
}

func (m *metricsFS) DirNames(name string) ([]string, error) {
	defer m.account("DirNames")()
	return m.next.DirNames(name)
}

func (m *metricsFS) Lstat(name string) (FileInfo, error) {
	defer m.account("Lstat")()
	return m.next.Lstat(name)
}

func (m *metricsFS) Mkdir(name string, perm FileMode) error {
	defer m.account("Mkdir")()
	return m.next.Mkdir(name, perm)
}

func (m *metricsFS) MkdirAll(name string, perm FileMode) error {
	defer m.account("MkdirAll")()
	return m.next.MkdirAll(name, perm)
}

func (m *metricsFS) Open(name string) (File, error) {
	defer m.account("Open")()
	return m.next.Open(name)
}

func (m *metricsFS) OpenFile(name string, flags int, mode FileMode) (File, error) {
	defer m.account("OpenFile")()
	return m.next.OpenFile(name, flags, mode)
}

func (m *metricsFS) ReadSymlink(name string) (string, error) {
	defer m.account("ReadSymlink")()
	return m.next.ReadSymlink(name)
}

func (m *metricsFS) Remove(name string) error {
	defer m.account("Remove")()
	return m.next.Remove(name)
}

func (m *metricsFS) RemoveAll(name string) error {
	defer m.account("RemoveAll")()
	return m.next.RemoveAll(name)
}

func (m *metricsFS) Rename(oldname, newname string) error {
	defer m.account("Rename")()
	return m.next.Rename(oldname, newname)
}

func (m *metricsFS) Stat(name string) (FileInfo, error) {
	defer m.account("Stat")()
	return m.next.Stat(name)
}

func (m *metricsFS) SymlinksSupported() bool {
	defer m.account("SymlinksSupported")()
	return m.next.SymlinksSupported()
}

func (m *metricsFS) Walk(name string, walkFn WalkFunc) error {
	defer m.account("Walk")()
	return m.next.Walk(name, walkFn)
}

func (m *metricsFS) Watch(path string, ignore Matcher, ctx context.Context, ignorePerms bool) (<-chan Event, <-chan error, error) {
	defer m.account("Watch")()
	return m.next.Watch(path, ignore, ctx, ignorePerms)
}

func (m *metricsFS) Hide(name string) error {
	defer m.account("Hide")()
	return m.next.Hide(name)
}

func (m *metricsFS) Unhide(name string) error {
	defer m.account("Unhide")()
	return m.next.Unhide(name)
}

func (m *metricsFS) Glob(pattern string) ([]string, error) {
	defer m.account("Glob")()
	return m.next.Glob(pattern)
}

func (m *metricsFS) Roots() ([]string, error) {
	defer m.account("Roots")()
	return m.next.Roots()
}

func (m *metricsFS) Usage(name string) (Usage, error) {
	defer m.account("Usage")()
	return m.next.Usage(name)
}

func (m *metricsFS) Type() FilesystemType {
	defer m.account("Type")()
	return m.next.Type()
}

func (m *metricsFS) URI() string {
	defer m.account("URI")()
	return m.next.URI()
}

func (m *metricsFS) Options() []Option {
	defer m.account("Options")()
	return m.next.Options()
}

func (m *metricsFS) SameFile(fi1, fi2 FileInfo) bool {
	defer m.account("SameFile")()
	return m.next.SameFile(fi1, fi2)
}

func (m *metricsFS) PlatformData(name string, withOwnership, withXattrs bool, xattrFilter XattrFilter) (protocol.PlatformData, error) {
	defer m.account("PlatformData")()
	return m.next.PlatformData(name, withOwnership, withXattrs, xattrFilter)
}

func (m *metricsFS) GetXattr(name string, xattrFilter XattrFilter) ([]protocol.Xattr, error) {
	defer m.account("GetXattr")()
	return m.next.GetXattr(name, xattrFilter)
}

func (m *metricsFS) SetXattr(path string, xattrs []protocol.Xattr, xattrFilter XattrFilter) error {
	defer m.account("SetXattr")()
	return m.next.SetXattr(path, xattrs, xattrFilter)
}

func (m *metricsFS) underlying() (Filesystem, bool) {
	return m.next, true
}

func (m *metricsFS) wrapperType() filesystemWrapperType {
	return filesystemWrapperTypeMetrics
}
