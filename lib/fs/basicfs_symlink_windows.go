// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// +build windows

package fs

import (
	"os"
	"path/filepath"

	"github.com/syncthing/syncthing/lib/osutil"

	"syscall"
	"unicode/utf16"
	"unsafe"
)

const (
	win32FsctlGetReparsePoint      = 0x900a8
	win32FileFlagOpenReparsePoint  = 0x00200000
	win32SymbolicLinkFlagDirectory = 0x1
)

var (
	modkernel32            = syscall.NewLazyDLL("kernel32.dll")
	procDeviceIoControl    = modkernel32.NewProc("DeviceIoControl")
	procCreateSymbolicLink = modkernel32.NewProc("CreateSymbolicLinkW")
	symlinksSupported      = false
)

func init() {
	defer func() {
		if err := recover(); err != nil {
			// Ensure that the supported flag is disabled when we hit an
			// error, even though it should already be. Also, silently swallow
			// the error since it's fine for a system not to support symlinks.
			symlinksSupported = false
		}
	}()

	// Needs administrator privileges.
	// Let's check that everything works.
	// This could be done more officially:
	// http://stackoverflow.com/questions/2094663/determine-if-windows-process-has-privilege-to-create-symbolic-link
	// But I don't want to define 10 more structs just to look this up.
	base := os.TempDir()
	path := filepath.Join(base, "symlinktest")
	defer os.Remove(path)

	err := DefaultFilesystem.CreateSymlink(path, base, LinkTargetDirectory)
	if err != nil {
		return
	}

	stat, err := osutil.Lstat(path)
	if err != nil || stat.Mode()&os.ModeSymlink == 0 {
		return
	}

	target, tt, err := DefaultFilesystem.ReadSymlink(path)
	if err != nil || osutil.NativeFilename(target) != base || tt != LinkTargetDirectory {
		return
	}
	symlinksSupported = true
}

func DisableSymlinks() {
	symlinksSupported = false
}

func (BasicFilesystem) SymlinksSupported() bool {
	return symlinksSupported
}

func (BasicFilesystem) ReadSymlink(path string) (string, LinkTargetType, error) {
	ptr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return "", LinkTargetUnknown, err
	}
	handle, err := syscall.CreateFile(ptr, 0, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE, nil, syscall.OPEN_EXISTING, syscall.FILE_FLAG_BACKUP_SEMANTICS|win32FileFlagOpenReparsePoint, 0)
	if err != nil || handle == syscall.InvalidHandle {
		return "", LinkTargetUnknown, err
	}
	defer syscall.Close(handle)
	var ret uint16
	var data reparseData

	r1, _, err := syscall.Syscall9(procDeviceIoControl.Addr(), 8, uintptr(handle), win32FsctlGetReparsePoint, 0, 0, uintptr(unsafe.Pointer(&data)), unsafe.Sizeof(data), uintptr(unsafe.Pointer(&ret)), 0, 0)
	if r1 == 0 {
		return "", LinkTargetUnknown, err
	}

	tt := LinkTargetUnknown
	if attr, err := syscall.GetFileAttributes(ptr); err == nil {
		if attr&syscall.FILE_ATTRIBUTE_DIRECTORY != 0 {
			tt = LinkTargetDirectory
		} else {
			tt = LinkTargetFile
		}
	}

	return osutil.NormalizedFilename(data.printName()), tt, nil
}

func (BasicFilesystem) CreateSymlink(path, target string, tt LinkTargetType) error {
	srcp, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}

	trgp, err := syscall.UTF16PtrFromString(osutil.NativeFilename(target))
	if err != nil {
		return err
	}

	// Sadly for Windows we need to specify the type of the symlink,
	// whether it's a directory symlink or a file symlink.
	// If the flags doesn't reveal the target type, try to evaluate it
	// ourselves, and worst case default to the symlink pointing to a file.
	mode := 0
	if tt == LinkTargetUnknown {
		path := target
		if !filepath.IsAbs(target) {
			path = filepath.Join(filepath.Dir(path), target)
		}

		stat, err := os.Stat(path)
		if err == nil && stat.IsDir() {
			mode = win32SymbolicLinkFlagDirectory
		}
	} else if tt == LinkTargetDirectory {
		mode = win32SymbolicLinkFlagDirectory
	}

	r0, _, err := syscall.Syscall(procCreateSymbolicLink.Addr(), 3, uintptr(unsafe.Pointer(srcp)), uintptr(unsafe.Pointer(trgp)), uintptr(mode))
	if r0 == 1 {
		return nil
	}
	return err
}

func (fs BasicFilesystem) ChangeSymlinkType(path string, tt LinkTargetType) error {
	target, existingTargetType, err := fs.ReadSymlink(path)
	if err != nil {
		return err
	}
	// If it's the same type, nothing to do.
	if tt == existingTargetType {
		return nil
	}

	// If the actual type is unknown, but the new type is file, nothing to do
	if existingTargetType == LinkTargetUnknown && tt != LinkTargetDirectory {
		return nil
	}
	return osutil.InWritableDir(func(path string) error {
		// It should be a symlink as well hence no need to change permissions on
		// the file.
		os.Remove(path)
		return fs.CreateSymlink(path, target, tt)
	}, path)
}

type reparseData struct {
	reparseTag          uint32
	reparseDataLength   uint16
	reserved            uint16
	substitueNameOffset uint16
	substitueNameLength uint16
	printNameOffset     uint16
	printNameLength     uint16
	flags               uint32
	// substituteName - 264 widechars max = 528 bytes
	// printName      - 260 widechars max = 520 bytes
	//                                    = 1048 bytes total
	buffer [1048 / 2]uint16
}

func (r *reparseData) printName() string {
	// offset and length are in bytes but we're indexing a []uint16
	offset := r.printNameOffset / 2
	length := r.printNameLength / 2
	return string(utf16.Decode(r.buffer[offset : offset+length]))
}

func (r *reparseData) substituteName() string {
	// offset and length are in bytes but we're indexing a []uint16
	offset := r.substitueNameOffset / 2
	length := r.substitueNameLength / 2
	return string(utf16.Decode(r.buffer[offset : offset+length]))
}
