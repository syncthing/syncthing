// +build windows

package osutil

import "syscall"

func HideFile(path string) error {
	p, err := syscall.UTF16PtrFromString(path)
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

func ShowFile(path string) error {
	p, err := syscall.UTF16PtrFromString(path)
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
