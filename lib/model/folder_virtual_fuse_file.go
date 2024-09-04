// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package model

import (
	"context"
	"sync"

	//	"time"

	"syscall"

	ffs "github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/syncthing/syncthing/lib/logger"
)

func NewVirtualFile(rel_name string, sVF SyncthingVirtualFolderI) ffs.FileHandle {
	return &virtualFuseFile{sVF: sVF, rel_name: rel_name}
}

type virtualFuseFile struct {
	sVF      SyncthingVirtualFolderI
	mu       sync.Mutex
	rel_name string // relative filepath
}

var _ = (ffs.FileHandle)((*virtualFuseFile)(nil))
var _ = (ffs.FileReleaser)((*virtualFuseFile)(nil))
var _ = (ffs.FileGetattrer)((*virtualFuseFile)(nil))
var _ = (ffs.FileReader)((*virtualFuseFile)(nil))
var _ = (ffs.FileWriter)((*virtualFuseFile)(nil))
var _ = (ffs.FileGetlker)((*virtualFuseFile)(nil))
var _ = (ffs.FileSetlker)((*virtualFuseFile)(nil))
var _ = (ffs.FileSetlkwer)((*virtualFuseFile)(nil))
var _ = (ffs.FileLseeker)((*virtualFuseFile)(nil))
var _ = (ffs.FileFlusher)((*virtualFuseFile)(nil))
var _ = (ffs.FileFsyncer)((*virtualFuseFile)(nil))
var _ = (ffs.FileSetattrer)((*virtualFuseFile)(nil))
var _ = (ffs.FileAllocater)((*virtualFuseFile)(nil))

func (f *virtualFuseFile) Read(ctx context.Context, buf []byte, off int64) (res fuse.ReadResult, errno syscall.Errno) {
	f.mu.Lock()
	defer f.mu.Unlock()
	//r := fuse.ReadResultFd(uintptr(f.fd), off, len(buf))
	//return r, ffs.OK

	logger.DefaultLogger.Infof("virtualFile Read(len, off): %v, %v", len(buf), off)

	return f.sVF.readFile(f.rel_name, buf, off)
}

func (f *virtualFuseFile) Write(ctx context.Context, data []byte, off int64) (uint32, syscall.Errno) {
	f.mu.Lock()
	defer f.mu.Unlock()
	//willBeChangedFd(f.fd)
	//n, err := syscall.Pwrite(f.fd, data, off)
	//f.changeChan <- Event{f.rel_name, NonRemove}
	//return uint32(n), ffs.ToErrno(err)
	return 0, syscall.EACCES
}

func (f *virtualFuseFile) Release(ctx context.Context) syscall.Errno {
	f.mu.Lock()
	defer f.mu.Unlock()
	//if f.fd != -1 {
	//	err := syscall.Close(f.fd)
	//	f.fd = -1
	//	return ffs.ToErrno(err)
	//}
	//return syscall.EBADF
	return ffs.OK
}

func (f *virtualFuseFile) Flush(ctx context.Context) syscall.Errno {
	f.mu.Lock()
	defer f.mu.Unlock()
	//// Since Flush() may be called for each dup'd fd, we don't
	//// want to really close the file, we just want to flush. This
	//// is achieved by closing a dup'd fd.
	//newFd, err := syscall.Dup(f.fd)
	//
	//if err != nil {
	//	return ffs.ToErrno(err)
	//}
	//err = syscall.Close(newFd)
	//return ffs.ToErrno(err)

	logger.DefaultLogger.Infof("virtualFile Flush(file): %s", f.rel_name)

	return ffs.OK
}

func (f *virtualFuseFile) Fsync(ctx context.Context, flags uint32) (errno syscall.Errno) {
	f.mu.Lock()
	defer f.mu.Unlock()
	//r := ffs.ToErrno(syscall.Fsync(f.fd))
	//
	//return r
	logger.DefaultLogger.Infof("virtualFile Fsync(file, flags): %s, %v", f.rel_name, flags)
	return ffs.OK
}

const (
	_OFD_GETLK  = 36
	_OFD_SETLK  = 37
	_OFD_SETLKW = 38
)

func (f *virtualFuseFile) Getlk(ctx context.Context, owner uint64, lk *fuse.FileLock, flags uint32, out *fuse.FileLock) (errno syscall.Errno) {
	//f.mu.Lock()
	//defer f.mu.Unlock()
	//flk := syscall.Flock_t{}
	//lk.ToFlockT(&flk)
	//errno = ffs.ToErrno(syscall.FcntlFlock(uintptr(f.fd), _OFD_GETLK, &flk))
	//out.FromFlockT(&flk)
	//return
	return syscall.ENOSYS
}

func (f *virtualFuseFile) Setlk(ctx context.Context, owner uint64, lk *fuse.FileLock, flags uint32) (errno syscall.Errno) {
	//return f.setLock(ctx, owner, lk, flags, false)
	return syscall.ENOSYS
}

func (f *virtualFuseFile) Setlkw(ctx context.Context, owner uint64, lk *fuse.FileLock, flags uint32) (errno syscall.Errno) {
	//return f.setLock(ctx, owner, lk, flags, true)
	return syscall.ENOSYS
}

func (f *virtualFuseFile) setLock(ctx context.Context, owner uint64, lk *fuse.FileLock, flags uint32, blocking bool) (errno syscall.Errno) {
	f.mu.Lock()
	defer f.mu.Unlock()
	//if (flags & fuse.FUSE_LK_FLOCK) != 0 {
	//	var op int
	//	switch lk.Typ {
	//	case syscall.F_RDLCK:
	//		op = syscall.LOCK_SH
	//	case syscall.F_WRLCK:
	//		op = syscall.LOCK_EX
	//	case syscall.F_UNLCK:
	//		op = syscall.LOCK_UN
	//	default:
	//		return syscall.EINVAL
	//	}
	//	if !blocking {
	//		op |= syscall.LOCK_NB
	//	}
	//	return ffs.ToErrno(syscall.Flock(f.fd, op))
	//} else {
	//	flk := syscall.Flock_t{}
	//	lk.ToFlockT(&flk)
	//	var op int
	//	if blocking {
	//		op = _OFD_SETLKW
	//	} else {
	//		op = _OFD_SETLK
	//	}
	//	return ffs.ToErrno(syscall.FcntlFlock(uintptr(f.fd), op, &flk))
	//}
	return syscall.ENOSYS
}

func (f *virtualFuseFile) Setattr(ctx context.Context, in *fuse.SetAttrIn, out *fuse.AttrOut) syscall.Errno {
	//willBeChangedFd(f.fd)
	//if errno := f.setAttr(ctx, in); errno != 0 {
	//	return errno
	//}
	//
	//return f.Getattr(ctx, out)
	return syscall.ENOSYS
}

func (f *virtualFuseFile) fchmod(mode uint32) syscall.Errno {
	f.mu.Lock()
	defer f.mu.Unlock()
	//willBeChangedFd(f.fd)
	//err := syscall.Fchmod(f.fd, mode)
	//return ffs.ToErrno(err)
	return syscall.ENOSYS
}

func (f *virtualFuseFile) fchown(uid, gid int) syscall.Errno {
	f.mu.Lock()
	defer f.mu.Unlock()
	//willBeChangedFd(f.fd)
	//err := syscall.Fchown(f.fd, uid, gid)
	//return ffs.ToErrno(err)
	return syscall.ENOSYS
}

func (f *virtualFuseFile) ftruncate(sz uint64) syscall.Errno {
	//willBeChangedFd(f.fd)
	//err := syscall.Ftruncate(f.fd, int64(sz))
	//return ffs.ToErrno(err)
	return syscall.ENOSYS
}

func (f *virtualFuseFile) setAttr(ctx context.Context, in *fuse.SetAttrIn) syscall.Errno {

	//willBeChangedFd(f.fd)
	//var errno syscall.Errno
	//if mode, ok := in.GetMode(); ok {
	//	if errno := f.fchmod(mode); errno != 0 {
	//		return errno
	//	}
	//}
	//
	//uid32, uOk := in.GetUID()
	//gid32, gOk := in.GetGID()
	//if uOk || gOk {
	//	uid := -1
	//	gid := -1
	//
	//	if uOk {
	//		uid = int(uid32)
	//	}
	//	if gOk {
	//		gid = int(gid32)
	//	}
	//	if errno := f.fchown(uid, gid); errno != 0 {
	//		return errno
	//	}
	//}
	//
	//mtime, mok := in.GetMTime()
	//atime, aok := in.GetATime()
	//
	//if mok || aok {
	//	ap := &atime
	//	mp := &mtime
	//	if !aok {
	//		ap = nil
	//	}
	//	if !mok {
	//		mp = nil
	//	}
	//	errno = f.utimens(ap, mp)
	//	if errno != 0 {
	//		return errno
	//	}
	//}
	//
	//if sz, ok := in.GetSize(); ok {
	//	if errno := f.ftruncate(sz); errno != 0 {
	//		return errno
	//	}
	//}
	//
	//return ffs.OK
	return syscall.ENOSYS
}

func (f *virtualFuseFile) Getattr(ctx context.Context, a *fuse.AttrOut) syscall.Errno {
	f.mu.Lock()
	defer f.mu.Unlock()
	//st := syscall.Stat_t{}
	//err := syscall.Fstat(f.fd, &st)
	//if err != nil {
	//	return ffs.ToErrno(err)
	//}
	//a.FromStat(&st)
	//
	//return ffs.OK
	return syscall.ENOSYS
}

func (f *virtualFuseFile) Lseek(ctx context.Context, off uint64, whence uint32) (uint64, syscall.Errno) {
	//f.mu.Lock()
	//defer f.mu.Unlock()
	//n, err := unix.Seek(f.fd, int64(off), int(whence))
	//return uint64(n), ffs.ToErrno(err)
	return 0, syscall.ENOSYS
}

func (f *virtualFuseFile) Allocate(ctx context.Context, off uint64, sz uint64, mode uint32) syscall.Errno {
	f.mu.Lock()
	defer f.mu.Unlock()
	//willBeChangedFd(f.fd)
	//err := unix.Fallocate(f.fd, mode, int64(off), int64(sz))
	//if err != nil {
	//	return ffs.ToErrno(err)
	//}
	//return ffs.OK
	return syscall.ENOSYS
}
