// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

// +build windows

package symlinks

import (
	"os"
	"path/filepath"

	"github.com/syncthing/syncthing/internal/osutil"
	"github.com/syncthing/syncthing/internal/protocol"

	"syscall"
	"unicode/utf16"
	"unsafe"
)

const (
	FSCTL_GET_REPARSE_POINT      = 0x900a8
	FILE_FLAG_OPEN_REPARSE_POINT = 0x00200000
	FILE_ATTRIBUTE_REPARSE_POINT = 0x400
	IO_REPARSE_TAG_SYMLINK       = 0xA000000C
	SYMBOLIC_LINK_FLAG_DIRECTORY = 0x1
)

var (
	modkernel32            = syscall.NewLazyDLL("kernel32.dll")
	procDeviceIoControl    = modkernel32.NewProc("DeviceIoControl")
	procCreateSymbolicLink = modkernel32.NewProc("CreateSymbolicLinkW")

	Supported = false
)

func init() {
	// Needs administrator priviledges.
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

	isLink, err := IsSymlink(path)
	if err != nil || !isLink {
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
	handle, err := syscall.CreateFile(ptr, 0, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE, nil, syscall.OPEN_EXISTING, syscall.FILE_FLAG_BACKUP_SEMANTICS|FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil || handle == syscall.InvalidHandle {
		return "", protocol.FlagSymlinkMissingTarget, err
	}
	defer syscall.Close(handle)
	var ret uint16
	var data reparseData

	r1, _, err := syscall.Syscall9(procDeviceIoControl.Addr(), 8, uintptr(handle), FSCTL_GET_REPARSE_POINT, 0, 0, uintptr(unsafe.Pointer(&data)), unsafe.Sizeof(data), uintptr(unsafe.Pointer(&ret)), 0, 0)
	if r1 == 0 {
		return "", protocol.FlagSymlinkMissingTarget, err
	}

	var flags uint32 = 0
	attr, err := syscall.GetFileAttributes(ptr)
	if err != nil {
		flags = protocol.FlagSymlinkMissingTarget
	} else if attr&syscall.FILE_ATTRIBUTE_DIRECTORY != 0 {
		flags = protocol.FlagDirectory
	}

	return osutil.NormalizedFilename(data.PrintName()), flags, nil
}

func IsSymlink(path string) (bool, error) {
	ptr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return false, err
	}

	attr, err := syscall.GetFileAttributes(ptr)
	if err != nil {
		return false, err
	}
	return attr&FILE_ATTRIBUTE_REPARSE_POINT != 0, nil
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
			mode = SYMBOLIC_LINK_FLAG_DIRECTORY
		}
	} else if flags&protocol.FlagDirectory != 0 {
		mode = SYMBOLIC_LINK_FLAG_DIRECTORY
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
