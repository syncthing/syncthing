// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// +build windows

package symlinks

import (
	"os"
	"path/filepath"

	"github.com/syncthing/protocol"
	"github.com/syncthing/syncthing/lib/osutil"

	"syscall"
	"unicode/utf16"
	"unsafe"
)

const (
	Win32FsctlGetReparsePoint      = 0x900a8
	Win32FileFlagOpenReparsePoint  = 0x00200000
	Win32FileAttributeReparsePoint = 0x400
	Win32IOReparseTagSymlink       = 0xA000000C
	Win32SymbolicLinkFlagDirectory = 0x1
)

var (
	modkernel32            = syscall.NewLazyDLL("kernel32.dll")
	procDeviceIoControl    = modkernel32.NewProc("DeviceIoControl")
	procCreateSymbolicLink = modkernel32.NewProc("CreateSymbolicLinkW")

	Supported = false
)

func init() {
	defer func() {
		if err := recover(); err != nil {
			// Ensure that the supported flag is disabled when we hit an
			// error, even though it should already be. Also, silently swallow
			// the error since it's fine for a system not to support symlinks.
			Supported = false
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

	err := Create(path, base, protocol.FlagDirectory)
	if err != nil {
		return
	}

	stat, err := osutil.Lstat(path)
	if err != nil || stat.Mode()&os.ModeSymlink == 0 {
		return
	}

	target, flags, err := Read(path)
	if err != nil || osutil.NativeFilename(target) != base || flags&protocol.FlagDirectory == 0 {
		return
	}
	Supported = true
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
	buffer [1048]uint16
}

func (r *reparseData) PrintName() string {
	// No clue why the offset and length is doubled...
	offset := r.printNameOffset / 2
	length := r.printNameLength / 2
	return string(utf16.Decode(r.buffer[offset : offset+length]))
}

func (r *reparseData) SubstituteName() string {
	// No clue why the offset and length is doubled...
	offset := r.substitueNameOffset / 2
	length := r.substitueNameLength / 2
	return string(utf16.Decode(r.buffer[offset : offset+length]))
}

func Read(path string) (string, uint32, error) {
	ptr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return "", protocol.FlagSymlinkMissingTarget, err
	}
	handle, err := syscall.CreateFile(ptr, 0, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE, nil, syscall.OPEN_EXISTING, syscall.FILE_FLAG_BACKUP_SEMANTICS|Win32FileFlagOpenReparsePoint, 0)
	if err != nil || handle == syscall.InvalidHandle {
		return "", protocol.FlagSymlinkMissingTarget, err
	}
	defer syscall.Close(handle)
	var ret uint16
	var data reparseData

	r1, _, err := syscall.Syscall9(procDeviceIoControl.Addr(), 8, uintptr(handle), Win32FsctlGetReparsePoint, 0, 0, uintptr(unsafe.Pointer(&data)), unsafe.Sizeof(data), uintptr(unsafe.Pointer(&ret)), 0, 0)
	if r1 == 0 {
		return "", protocol.FlagSymlinkMissingTarget, err
	}

	var flags uint32
	attr, err := syscall.GetFileAttributes(ptr)
	if err != nil {
		flags = protocol.FlagSymlinkMissingTarget
	} else if attr&syscall.FILE_ATTRIBUTE_DIRECTORY != 0 {
		flags = protocol.FlagDirectory
	}

	return osutil.NormalizedFilename(data.PrintName()), flags, nil
}

func Create(source, target string, flags uint32) error {
	srcp, err := syscall.UTF16PtrFromString(source)
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
	if flags&protocol.FlagSymlinkMissingTarget != 0 {
		path := target
		if !filepath.IsAbs(target) {
			path = filepath.Join(filepath.Dir(source), target)
		}

		stat, err := os.Stat(path)
		if err == nil && stat.IsDir() {
			mode = Win32SymbolicLinkFlagDirectory
		}
	} else if flags&protocol.FlagDirectory != 0 {
		mode = Win32SymbolicLinkFlagDirectory
	}

	r0, _, err := syscall.Syscall(procCreateSymbolicLink.Addr(), 3, uintptr(unsafe.Pointer(srcp)), uintptr(unsafe.Pointer(trgp)), uintptr(mode))
	if r0 == 1 {
		return nil
	}
	return err
}

func ChangeType(path string, flags uint32) error {
	target, cflags, err := Read(path)
	if err != nil {
		return err
	}
	// If it's the same type, nothing to do.
	if cflags&protocol.SymlinkTypeMask == flags&protocol.SymlinkTypeMask {
		return nil
	}

	// If the actual type is unknown, but the new type is file, nothing to do
	if cflags&protocol.FlagSymlinkMissingTarget != 0 && flags&protocol.FlagDirectory == 0 {
		return nil
	}
	return osutil.InWritableDir(func(path string) error {
		// It should be a symlink as well hence no need to change permissions on
		// the file.
		os.Remove(path)
		return Create(path, target, flags)
	}, path)
}
