// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
)

// NewIOLimiterOption returns an Option that wraps the filesystem with an IOPS
// rate limiter. Both global and local may point to a nil *rate.Limiter (meaning
// unlimited). Either pointer may be updated concurrently via atomic.Pointer.Store
// without restarting the folder.
func NewIOLimiterOption(global, local *atomic.Pointer[rate.Limiter]) Option {
	return &optionIOLimiter{global: global, local: local}
}

type optionIOLimiter struct {
	global *atomic.Pointer[rate.Limiter]
	local  *atomic.Pointer[rate.Limiter]
}

func (o *optionIOLimiter) String() string {
	return "iolimiter"
}

func (o *optionIOLimiter) apply(underlying Filesystem) Filesystem {
	return &ioLimitedFilesystem{
		Filesystem: underlying,
		global:     o.global,
		local:      o.local,
	}
}

// ioLimitedFilesystem wraps a Filesystem and limits I/O operations per second.
// It throttles every filesystem and file operation that causes a disk access.
// The global limiter is shared across all folders; the local limiter is
// per-folder. An operation must be permitted by both limiters before it
// proceeds.
//
// Note: filesystem and file methods do not carry a context.Context, so
// context.Background() is used for rate limiter waits. For reasonable IOPS
// limits (>= 1) the maximum delay per operation is 1 second, which is
// acceptable for shutdown latency.
type ioLimitedFilesystem struct {
	Filesystem
	global *atomic.Pointer[rate.Limiter]
	local  *atomic.Pointer[rate.Limiter]
}

// underlying implements wrappingFilesystem.
func (fs *ioLimitedFilesystem) underlying() (Filesystem, bool) {
	return fs.Filesystem, true
}

// waitIOPS blocks until both the global and local limiters permit one I/O
// operation. It is a no-op if both limiters are nil.
func (fs *ioLimitedFilesystem) waitIOPS() {
	waitOneLimiter(fs.global)
	waitOneLimiter(fs.local)
}

func waitOneLimiter(p *atomic.Pointer[rate.Limiter]) {
	if p == nil {
		return
	}
	l := p.Load()
	if l == nil {
		return
	}
	// Use Reserve + sleep so that we can respect the delay without blocking
	// forever when the limiter is at capacity. context.Background() is
	// intentional here – see type doc above.
	r := l.Reserve()
	if !r.OK() {
		// Burst exceeded; skip this token rather than blocking indefinitely.
		return
	}
	if d := r.Delay(); d > 0 {
		time.Sleep(d)
	}
}

// Filesystem-level operations — each counts as one IOPS.

func (fs *ioLimitedFilesystem) Lstat(name string) (FileInfo, error) {
	fs.waitIOPS()
	return fs.Filesystem.Lstat(name)
}

func (fs *ioLimitedFilesystem) Stat(name string) (FileInfo, error) {
	fs.waitIOPS()
	return fs.Filesystem.Stat(name)
}

func (fs *ioLimitedFilesystem) DirNames(name string) ([]string, error) {
	fs.waitIOPS()
	return fs.Filesystem.DirNames(name)
}

func (fs *ioLimitedFilesystem) Remove(name string) error {
	fs.waitIOPS()
	return fs.Filesystem.Remove(name)
}

func (fs *ioLimitedFilesystem) RemoveAll(name string) error {
	fs.waitIOPS()
	return fs.Filesystem.RemoveAll(name)
}

func (fs *ioLimitedFilesystem) Rename(oldname, newname string) error {
	fs.waitIOPS()
	return fs.Filesystem.Rename(oldname, newname)
}

func (fs *ioLimitedFilesystem) Mkdir(name string, perm FileMode) error {
	fs.waitIOPS()
	return fs.Filesystem.Mkdir(name, perm)
}

func (fs *ioLimitedFilesystem) MkdirAll(name string, perm FileMode) error {
	fs.waitIOPS()
	return fs.Filesystem.MkdirAll(name, perm)
}

// File-opening operations return ioLimitedFile to throttle reads and writes.

func (fs *ioLimitedFilesystem) Open(name string) (File, error) {
	fs.waitIOPS()
	f, err := fs.Filesystem.Open(name)
	if err != nil {
		return nil, err
	}
	return &ioLimitedFile{File: f, global: fs.global, local: fs.local}, nil
}

func (fs *ioLimitedFilesystem) OpenFile(name string, flags int, mode FileMode) (File, error) {
	fs.waitIOPS()
	f, err := fs.Filesystem.OpenFile(name, flags, mode)
	if err != nil {
		return nil, err
	}
	return &ioLimitedFile{File: f, global: fs.global, local: fs.local}, nil
}

func (fs *ioLimitedFilesystem) Create(name string) (File, error) {
	fs.waitIOPS()
	f, err := fs.Filesystem.Create(name)
	if err != nil {
		return nil, err
	}
	return &ioLimitedFile{File: f, global: fs.global, local: fs.local}, nil
}

// ioLimitedFile wraps a File to rate-limit read and write operations.
type ioLimitedFile struct {
	File
	global *atomic.Pointer[rate.Limiter]
	local  *atomic.Pointer[rate.Limiter]
}

func (f *ioLimitedFile) waitIOPS() {
	waitOneLimiter(f.global)
	waitOneLimiter(f.local)
}

func (f *ioLimitedFile) Read(p []byte) (int, error) {
	f.waitIOPS()
	return f.File.Read(p)
}

func (f *ioLimitedFile) ReadAt(p []byte, off int64) (int, error) {
	f.waitIOPS()
	return f.File.ReadAt(p, off)
}

func (f *ioLimitedFile) Write(p []byte) (int, error) {
	f.waitIOPS()
	return f.File.Write(p)
}

func (f *ioLimitedFile) WriteAt(p []byte, off int64) (int, error) {
	f.waitIOPS()
	return f.File.WriteAt(p, off)
}

func (f *ioLimitedFile) Sync() error {
	f.waitIOPS()
	return f.File.Sync()
}

// newIOPSLimiter returns a token-bucket rate limiter that permits iops
// operations per second, or nil if iops <= 0 (unlimited).
func newIOPSLimiter(iops int) *rate.Limiter {
	if iops <= 0 {
		return nil
	}
	const minBurst = 10
	burst := iops
	if burst < minBurst {
		burst = minBurst
	}
	return rate.NewLimiter(rate.Limit(iops), burst)
}

// NewIOPSLimiter creates a token-bucket rate limiter that permits iops
// operations per second, or returns nil if iops <= 0 (unlimited).
func NewIOPSLimiter(iops int) *rate.Limiter {
	return newIOPSLimiter(iops)
}

// NewAtomicIOPSLimiter returns an *atomic.Pointer[rate.Limiter] pre-loaded
// with a limiter for iops operations per second, or nil (unlimited) when
// iops <= 0. This allows callers to create per-folder limiters without
// importing the rate or sync/atomic packages directly.
func NewAtomicIOPSLimiter(iops int) *atomic.Pointer[rate.Limiter] {
	p := new(atomic.Pointer[rate.Limiter])
	p.Store(newIOPSLimiter(iops))
	return p
}
