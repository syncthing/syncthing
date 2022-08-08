// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build windows
// +build windows

package fs

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"github.com/syncthing/syncthing/lib/protocol"
	"golang.org/x/sys/windows"
)

var errNotSupported = errors.New("symlinks not supported")

func (BasicFilesystem) SymlinksSupported() bool {
	return false
}

func (BasicFilesystem) ReadSymlink(path string) (string, error) {
	return "", errNotSupported
}

func (BasicFilesystem) CreateSymlink(target, name string) error {
	return errNotSupported
}

// Required due to https://github.com/golang/go/issues/10900
func (f *BasicFilesystem) mkdirAll(path string, perm os.FileMode) error {
	// Fast path: if we can tell whether path is a directory or file, stop with success or error.
	dir, err := os.Stat(path)
	if err == nil {
		if dir.IsDir() {
			return nil
		}
		return &os.PathError{
			Op:   "mkdir",
			Path: path,
			Err:  syscall.ENOTDIR,
		}
	}

	// Slow path: make sure parent exists and then call Mkdir for path.
	i := len(path)
	for i > 0 && IsPathSeparator(path[i-1]) { // Skip trailing path separator.
		i--
	}

	j := i
	for j > 0 && !IsPathSeparator(path[j-1]) { // Scan backward over element.
		j--
	}

	if j > 1 {
		// Create parent
		parent := path[0 : j-1]
		if parent != filepath.VolumeName(parent) {
			err = f.mkdirAll(parent, perm)
			if err != nil {
				return err
			}
		}
	}

	// Parent now exists; invoke Mkdir and use its result.
	err = os.Mkdir(path, perm)
	if err != nil {
		// Handle arguments like "foo/." by
		// double-checking that directory doesn't exist.
		dir, err1 := os.Lstat(path)
		if err1 == nil && dir.IsDir() {
			return nil
		}
		return err
	}
	return nil
}

func (f *BasicFilesystem) Unhide(name string) error {
	name, err := f.rooted(name)
	if err != nil {
		return err
	}
	p, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return err
	}

	attrs, err := syscall.GetFileAttributes(p)
	if err != nil {
		return err
	}

	attrs &^= syscall.FILE_ATTRIBUTE_HIDDEN
	return syscall.SetFileAttributes(p, attrs)
}

func (f *BasicFilesystem) Hide(name string) error {
	name, err := f.rooted(name)
	if err != nil {
		return err
	}
	p, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return err
	}

	attrs, err := syscall.GetFileAttributes(p)
	if err != nil {
		return err
	}

	attrs |= syscall.FILE_ATTRIBUTE_HIDDEN
	return syscall.SetFileAttributes(p, attrs)
}

func (f *BasicFilesystem) Roots() ([]string, error) {
	kernel32, err := syscall.LoadDLL("kernel32.dll")
	if err != nil {
		return nil, err
	}
	getLogicalDriveStringsHandle, err := kernel32.FindProc("GetLogicalDriveStringsA")
	if err != nil {
		return nil, err
	}

	buffer := [1024]byte{}
	bufferSize := uint32(len(buffer))

	hr, _, _ := getLogicalDriveStringsHandle.Call(uintptr(unsafe.Pointer(&bufferSize)), uintptr(unsafe.Pointer(&buffer)))
	if hr == 0 {
		return nil, errors.New("syscall failed")
	}

	var drives []string
	parts := bytes.Split(buffer[:], []byte{0})
	for _, part := range parts {
		if len(part) == 0 {
			break
		}
		drives = append(drives, string(part))
	}

	return drives, nil
}

func (f *BasicFilesystem) GetXattr(name string, xattrFilter StringFilter) ([]protocol.Xattr, error) {
	// XXX: implement
	return nil, nil
}

func (f *BasicFilesystem) SetXattr(path string, xattrs []protocol.Xattr, xattrFilter StringFilter) error {
	// XXX: implement
	return nil
}

func (f *BasicFilesystem) Lchown(name, uid, gid string) error {
	name, err := f.rooted(name)
	if err != nil {
		return err
	}

	hdl, err := windows.Open(name, windows.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer windows.Close(hdl)

	// Depending on whether we got an uid or a gid, we need to set the
	// appropriate flag and parse the corresponding SID. The one we're not
	// setting remains nil, which is what we want in the call to
	// SetSecurityInfo.

	var si windows.SECURITY_INFORMATION
	var ownerSID, groupSID *syscall.SID
	if uid != "" {
		ownerSID, err = syscall.StringToSid(uid)
		if err == nil {
			si |= windows.OWNER_SECURITY_INFORMATION
		}
	} else if gid != "" {
		groupSID, err = syscall.StringToSid(uid)
		if err == nil {
			si |= windows.GROUP_SECURITY_INFORMATION
		}
	} else {
		return errors.New("neither uid nor gid specified")
	}

	return windows.SetSecurityInfo(hdl, windows.SE_FILE_OBJECT, si, (*windows.SID)(ownerSID), (*windows.SID)(groupSID), nil, nil)
}

// unrootedChecked returns the path relative to the folder root (same as
// unrooted) or an error if the given path is not a subpath and handles the
// special case when the given path is the folder root without a trailing
// pathseparator.
func (f *BasicFilesystem) unrootedChecked(absPath string, roots []string) (string, error) {
	absPath = f.resolveWin83(absPath)
	lowerAbsPath := UnicodeLowercaseNormalized(absPath)
	for _, root := range roots {
		lowerRoot := UnicodeLowercaseNormalized(root)
		if lowerAbsPath+string(PathSeparator) == lowerRoot {
			return ".", nil
		}
		if strings.HasPrefix(lowerAbsPath, lowerRoot) {
			return rel(absPath, root), nil
		}
	}
	return "", f.newErrWatchEventOutsideRoot(lowerAbsPath, roots)
}

func rel(path, prefix string) string {
	lowerRel := strings.TrimPrefix(strings.TrimPrefix(UnicodeLowercaseNormalized(path), UnicodeLowercaseNormalized(prefix)), string(PathSeparator))
	return path[len(path)-len(lowerRel):]
}

func (f *BasicFilesystem) resolveWin83(absPath string) string {
	if !isMaybeWin83(absPath) {
		return absPath
	}
	if in, err := syscall.UTF16FromString(absPath); err == nil {
		out := make([]uint16, 4*len(absPath)) // *2 for UTF16 and *2 to double path length
		if n, err := syscall.GetLongPathName(&in[0], &out[0], uint32(len(out))); err == nil {
			if n <= uint32(len(out)) {
				return syscall.UTF16ToString(out[:n])
			}
			out = make([]uint16, n)
			if _, err = syscall.GetLongPathName(&in[0], &out[0], n); err == nil {
				return syscall.UTF16ToString(out)
			}
		}
	}
	// Failed getting the long path. Return the part of the path which is
	// already a long path.
	lowerRoot := UnicodeLowercaseNormalized(f.root)
	for absPath = filepath.Dir(absPath); strings.HasPrefix(UnicodeLowercaseNormalized(absPath), lowerRoot); absPath = filepath.Dir(absPath) {
		if !isMaybeWin83(absPath) {
			return absPath
		}
	}
	return f.root
}

func isMaybeWin83(absPath string) bool {
	if !strings.Contains(absPath, "~") {
		return false
	}
	if strings.Contains(filepath.Dir(absPath), "~") {
		return true
	}
	return strings.Contains(strings.TrimPrefix(filepath.Base(absPath), WindowsTempPrefix), "~")
}

func getFinalPathName(in string) (string, error) {
	// Return the normalized path
	// Wrap the call to GetFinalPathNameByHandleW
	// The string returned by this function uses the \?\ syntax
	// Implies GetFullPathName + GetLongPathName
	kernel32, err := syscall.LoadDLL("kernel32.dll")
	if err != nil {
		return "", err
	}
	GetFinalPathNameByHandleW, err := kernel32.FindProc("GetFinalPathNameByHandleW")
	// https://github.com/golang/go/blob/ff048033e4304898245d843e79ed1a0897006c6d/src/internal/syscall/windows/syscall_windows.go#L303
	if err != nil {
		return "", err
	}
	inPath, err := syscall.UTF16PtrFromString(in)
	if err != nil {
		return "", err
	}
	// Get a file handler
	h, err := syscall.CreateFile(inPath,
		syscall.GENERIC_READ,
		syscall.FILE_SHARE_READ,
		nil,
		syscall.OPEN_EXISTING,
		uint32(syscall.FILE_FLAG_BACKUP_SEMANTICS),
		0)
	if err != nil {
		return "", err
	}
	defer syscall.CloseHandle(h)
	// Call GetFinalPathNameByHandleW
	var VOLUME_NAME_DOS uint32 = 0x0      // not yet defined in syscall
	var bufSize uint32 = syscall.MAX_PATH // 260
	for i := 0; i < 2; i++ {
		buf := make([]uint16, bufSize)
		var ret uintptr
		ret, _, err = GetFinalPathNameByHandleW.Call(
			uintptr(h),                       // HANDLE hFile
			uintptr(unsafe.Pointer(&buf[0])), // LPWSTR lpszFilePath
			uintptr(bufSize),                 // DWORD  cchFilePath
			uintptr(VOLUME_NAME_DOS),         // DWORD  dwFlags
		)
		// The returned value is the actual length of the norm path
		// After Win 10 build 1607, MAX_PATH limitations have been removed
		// so it is necessary to check newBufSize
		newBufSize := uint32(ret) + 1
		if ret == 0 || newBufSize > bufSize*100 {
			break
		}
		if newBufSize <= bufSize {
			return syscall.UTF16ToString(buf), nil
		}
		bufSize = newBufSize
	}
	return "", err
}

func evalSymlinks(in string) (string, error) {
	out, err := filepath.EvalSymlinks(in)
	if err != nil && strings.HasPrefix(in, `\\?\`) {
		// Try again without the `\\?\` prefix
		out, err = filepath.EvalSymlinks(in[4:])
	}
	if err != nil {
		// Try to get a normalized path from Win-API
		var err1 error
		out, err1 = getFinalPathName(in)
		if err1 != nil {
			return "", err // return the prior error
		}
		// Trim UNC prefix, equivalent to
		// https://github.com/golang/go/blob/2396101e0590cb7d77556924249c26af0ccd9eff/src/os/file_windows.go#L470
		if strings.HasPrefix(out, `\\?\UNC\`) {
			out = `\` + out[7:] // path like \\server\share\...
		} else {
			out = strings.TrimPrefix(out, `\\?\`)
		}
	}
	return longFilenameSupport(out), nil
}

// watchPaths adjust the folder root for use with the notify backend and the
// corresponding absolute path to be passed to notify to watch name.
func (f *BasicFilesystem) watchPaths(name string) (string, []string, error) {
	root, err := evalSymlinks(f.root)
	if err != nil {
		return "", nil, err
	}

	// Remove `\\?\` prefix if the path is just a drive letter as a dirty
	// fix for https://github.com/syncthing/syncthing/issues/5578
	if filepath.Clean(name) == "." && len(root) <= 7 && len(root) > 4 && root[:4] == `\\?\` {
		root = root[4:]
	}

	absName, err := rooted(name, root)
	if err != nil {
		return "", nil, err
	}

	roots := []string{f.resolveWin83(root)}
	absName = f.resolveWin83(absName)

	// Events returned from fs watching are all over the place, so allow
	// both the user's input and the result of "canonicalizing" the path.
	if roots[0] != f.root {
		roots = append(roots, f.root)
	}

	return filepath.Join(absName, "..."), roots, nil
}
