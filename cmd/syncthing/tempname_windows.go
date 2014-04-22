// +build windows

package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"syscall"
)

type tempNamer struct {
	prefix string
}

var defTempNamer = tempNamer{"~syncthing~"}

func (t tempNamer) IsTemporary(name string) bool {
	return strings.HasPrefix(filepath.Base(name), t.prefix)
}

func (t tempNamer) TempName(name string) string {
	tdir := filepath.Dir(name)
	tname := fmt.Sprintf("%s.%s.tmp", t.prefix, filepath.Base(name))
	return filepath.Join(tdir, tname)
}

func (t tempNamer) Hide(path string) error {
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

func (t tempNamer) Show(path string) error {
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
