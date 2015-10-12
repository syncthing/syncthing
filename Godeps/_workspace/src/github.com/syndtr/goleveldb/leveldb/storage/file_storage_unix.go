// Copyright (c) 2012, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// +build darwin dragonfly freebsd linux netbsd openbsd

package storage

import (
	"os"
	"syscall"
)

type unixFileLock struct {
	f *os.File
}

func (fl *unixFileLock) release() error {
	if err := setFileLock(fl.f, false); err != nil {
		return err
	}
	return fl.f.Close()
}

func newFileLock(path string) (fl fileLock, err error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return
	}
	err = setFileLock(f, true)
	if err != nil {
		f.Close()
		return
	}
	fl = &unixFileLock{f: f}
	return
}

func setFileLock(f *os.File, lock bool) error {
	how := syscall.LOCK_UN
	if lock {
		how = syscall.LOCK_EX
	}
	return syscall.Flock(int(f.Fd()), how|syscall.LOCK_NB)
}

func rename(oldpath, newpath string) error {
	return os.Rename(oldpath, newpath)
}

func isErrInvalid(err error) bool {
	if err == os.ErrInvalid {
		return true
	}
	if syserr, ok := err.(*os.SyscallError); ok && syserr.Err == syscall.EINVAL {
		return true
	}
	return false
}

func syncDir(name string) error {
	f, err := os.Open(name)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := f.Sync(); err != nil && !isErrInvalid(err) {
		return err
	}
	return nil
}
