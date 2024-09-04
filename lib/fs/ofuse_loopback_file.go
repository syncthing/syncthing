// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"context"
	"sync"

	//	"time"

	"syscall"

	ffs "github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/syncthing/syncthing/lib/logger"
	"golang.org/x/sys/unix"
)

// NewLoopbackFile creates a FileHandle out of a file descriptor. All
// operations are implemented. When using the Fd from a *os.File, call
// syscall.Dup() on the fd, to avoid os.File's finalizer from closing
// the file descriptor.
func NewLoopbackFile(rel_name string, fd int, changeChan chan<- Event) ffs.FileHandle {
	return &loopbackFile{fd: fd, rel_name: rel_name, changeChan: changeChan}
}

type loopbackFile struct {
	mu         sync.Mutex
	fd         int
	rel_name   string // relative filepath
	changeChan chan<- Event
}

var _ = (ffs.FileHandle)((*loopbackFile)(nil))
var _ = (ffs.FileReleaser)((*loopbackFile)(nil))
var _ = (ffs.FileGetattrer)((*loopbackFile)(nil))
var _ = (ffs.FileReader)((*loopbackFile)(nil))
var _ = (ffs.FileWriter)((*loopbackFile)(nil))
var _ = (ffs.FileGetlker)((*loopbackFile)(nil))
var _ = (ffs.FileSetlker)((*loopbackFile)(nil))
var _ = (ffs.FileSetlkwer)((*loopbackFile)(nil))
var _ = (ffs.FileLseeker)((*loopbackFile)(nil))
var _ = (ffs.FileFlusher)((*loopbackFile)(nil))
var _ = (ffs.FileFsyncer)((*loopbackFile)(nil))
var _ = (ffs.FileSetattrer)((*loopbackFile)(nil))
var _ = (ffs.FileAllocater)((*loopbackFile)(nil))

func (f *loopbackFile) Read(ctx context.Context, buf []byte, off int64) (res fuse.ReadResult, errno syscall.Errno) {
	f.mu.Lock()
	defer f.mu.Unlock()
	logger.DefaultLogger.Infof("loopbackFile Read(len, off): %v, %v", len(buf), off)
	r := fuse.ReadResultFd(uintptr(f.fd), off, len(buf))
	return r, ffs.OK
}

func (f *loopbackFile) Write(ctx context.Context, data []byte, off int64) (uint32, syscall.Errno) {
	f.mu.Lock()
	defer f.mu.Unlock()
	willBeChangedFd(f.fd)
	n, err := syscall.Pwrite(f.fd, data, off)
	f.changeChan <- Event{f.rel_name, NonRemove}
	return uint32(n), ffs.ToErrno(err)
}

func (f *loopbackFile) Release(ctx context.Context) syscall.Errno {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fd != -1 {
		err := syscall.Close(f.fd)
		f.fd = -1
		return ffs.ToErrno(err)
	}
	return syscall.EBADF
}

func (f *loopbackFile) Flush(ctx context.Context) syscall.Errno {
	f.mu.Lock()
	defer f.mu.Unlock()
	// Since Flush() may be called for each dup'd fd, we don't
	// want to really close the file, we just want to flush. This
	// is achieved by closing a dup'd fd.
	newFd, err := syscall.Dup(f.fd)

	if err != nil {
		return ffs.ToErrno(err)
	}
	err = syscall.Close(newFd)
	return ffs.ToErrno(err)
}

func (f *loopbackFile) Fsync(ctx context.Context, flags uint32) (errno syscall.Errno) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r := ffs.ToErrno(syscall.Fsync(f.fd))

	return r
}

const (
	_OFD_GETLK  = 36
	_OFD_SETLK  = 37
	_OFD_SETLKW = 38
)

func (f *loopbackFile) Getlk(ctx context.Context, owner uint64, lk *fuse.FileLock, flags uint32, out *fuse.FileLock) (errno syscall.Errno) {
	f.mu.Lock()
	defer f.mu.Unlock()
	flk := syscall.Flock_t{}
	lk.ToFlockT(&flk)
	errno = ffs.ToErrno(syscall.FcntlFlock(uintptr(f.fd), _OFD_GETLK, &flk))
	out.FromFlockT(&flk)
	return
}

func (f *loopbackFile) Setlk(ctx context.Context, owner uint64, lk *fuse.FileLock, flags uint32) (errno syscall.Errno) {
	return f.setLock(ctx, owner, lk, flags, false)
}

func (f *loopbackFile) Setlkw(ctx context.Context, owner uint64, lk *fuse.FileLock, flags uint32) (errno syscall.Errno) {
	return f.setLock(ctx, owner, lk, flags, true)
}

func (f *loopbackFile) setLock(ctx context.Context, owner uint64, lk *fuse.FileLock, flags uint32, blocking bool) (errno syscall.Errno) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if (flags & fuse.FUSE_LK_FLOCK) != 0 {
		var op int
		switch lk.Typ {
		case syscall.F_RDLCK:
			op = syscall.LOCK_SH
		case syscall.F_WRLCK:
			op = syscall.LOCK_EX
		case syscall.F_UNLCK:
			op = syscall.LOCK_UN
		default:
			return syscall.EINVAL
		}
		if !blocking {
			op |= syscall.LOCK_NB
		}
		return ffs.ToErrno(syscall.Flock(f.fd, op))
	} else {
		flk := syscall.Flock_t{}
		lk.ToFlockT(&flk)
		var op int
		if blocking {
			op = _OFD_SETLKW
		} else {
			op = _OFD_SETLK
		}
		return ffs.ToErrno(syscall.FcntlFlock(uintptr(f.fd), op, &flk))
	}
}

func (f *loopbackFile) Setattr(ctx context.Context, in *fuse.SetAttrIn, out *fuse.AttrOut) syscall.Errno {
	willBeChangedFd(f.fd)
	if errno := f.setAttr(ctx, in); errno != 0 {
		return errno
	}

	return f.Getattr(ctx, out)
}

func (f *loopbackFile) fchmod(mode uint32) syscall.Errno {
	f.mu.Lock()
	defer f.mu.Unlock()
	willBeChangedFd(f.fd)
	err := syscall.Fchmod(f.fd, mode)
	return ffs.ToErrno(err)
}

func (f *loopbackFile) fchown(uid, gid int) syscall.Errno {
	f.mu.Lock()
	defer f.mu.Unlock()
	willBeChangedFd(f.fd)
	err := syscall.Fchown(f.fd, uid, gid)
	return ffs.ToErrno(err)
}

func (f *loopbackFile) ftruncate(sz uint64) syscall.Errno {
	willBeChangedFd(f.fd)
	err := syscall.Ftruncate(f.fd, int64(sz))
	return ffs.ToErrno(err)
}

func (f *loopbackFile) setAttr(ctx context.Context, in *fuse.SetAttrIn) syscall.Errno {

	willBeChangedFd(f.fd)
	var errno syscall.Errno
	if mode, ok := in.GetMode(); ok {
		if errno := f.fchmod(mode); errno != 0 {
			return errno
		}
	}

	uid32, uOk := in.GetUID()
	gid32, gOk := in.GetGID()
	if uOk || gOk {
		uid := -1
		gid := -1

		if uOk {
			uid = int(uid32)
		}
		if gOk {
			gid = int(gid32)
		}
		if errno := f.fchown(uid, gid); errno != 0 {
			return errno
		}
	}

	mtime, mok := in.GetMTime()
	atime, aok := in.GetATime()

	if mok || aok {
		ap := &atime
		mp := &mtime
		if !aok {
			ap = nil
		}
		if !mok {
			mp = nil
		}
		errno = f.utimens(ap, mp)
		if errno != 0 {
			return errno
		}
	}

	if sz, ok := in.GetSize(); ok {
		if errno := f.ftruncate(sz); errno != 0 {
			return errno
		}
	}

	return ffs.OK
}

func (f *loopbackFile) Getattr(ctx context.Context, a *fuse.AttrOut) syscall.Errno {
	f.mu.Lock()
	defer f.mu.Unlock()
	st := syscall.Stat_t{}
	err := syscall.Fstat(f.fd, &st)
	if err != nil {
		return ffs.ToErrno(err)
	}
	a.FromStat(&st)

	return ffs.OK
}

func (f *loopbackFile) Lseek(ctx context.Context, off uint64, whence uint32) (uint64, syscall.Errno) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n, err := unix.Seek(f.fd, int64(off), int(whence))
	return uint64(n), ffs.ToErrno(err)
}

func (f *loopbackFile) Allocate(ctx context.Context, off uint64, sz uint64, mode uint32) syscall.Errno {
	f.mu.Lock()
	defer f.mu.Unlock()
	willBeChangedFd(f.fd)
	err := unix.Fallocate(f.fd, mode, int64(off), int64(sz))
	if err != nil {
		return ffs.ToErrno(err)
	}
	return ffs.OK
}
