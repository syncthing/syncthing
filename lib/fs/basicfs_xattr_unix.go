package fs

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

func (f *BasicFilesystem) GetXattr(path string) (map[string][]byte, error) {
	path, err := f.rooted(path)
	if err != nil {
		return nil, err
	}

	buf := make([]byte, 1)
	buf, err = listXattr(path, buf)
	if err != nil {
		return nil, err
	}

	attrs := strings.Split(string(buf), "\x00")
	res := make(map[string][]byte, len(attrs))
	var val []byte
	for _, attr := range attrs {
		if attr == "" {
			continue
		}
		val, buf, err = getXattr(path, attr, buf)
		if err != nil {
			fmt.Println("Error getting xattr", attr, err)
			continue
		}
		res[attr] = val
	}
	return res, nil
}

func listXattr(path string, buf []byte) ([]byte, error) {
	size, err := unix.Listxattr(path, buf)
	if errors.Is(err, syscall.ERANGE) {
		// Buffer is too small. Try again with a zero sized buffer to get
		// the size, then allocate a buffer of the correct size.
		size, err = unix.Listxattr(path, nil)
		if err != nil {
			return nil, err
		}
		if size > len(buf) {
			buf = make([]byte, size)
		}
		size, err = unix.Listxattr(path, buf)
	}
	return buf[:size], err
}

func getXattr(path, name string, buf []byte) (val []byte, rest []byte, err error) {
	if len(buf) == 0 {
		buf = make([]byte, 1024)
	}
	size, err := unix.Getxattr(path, name, buf)
	if errors.Is(err, syscall.ERANGE) {
		// Buffer was too small. Figure out how large it needs to be, and
		// allocate.
		size, err = unix.Getxattr(path, name, nil)
		if err != nil {
			return nil, nil, err
		}
		if size > len(buf) {
			buf = make([]byte, size)
		}
		size, err = unix.Getxattr(path, name, buf)
	}
	if err != nil {
		return nil, buf, err
	}
	return buf[:size], buf[size:], nil
}

func (f *BasicFilesystem) SetXattrs(path string, xattrs map[string][]byte) error {
	current, err := f.GetXattr(path)
	if err != nil {
		return err
	}

	path, err = f.rooted(path)
	if err != nil {
		return err
	}

	// Remove all xattrs that are not in the new map
	for key := range current {
		if _, ok := xattrs[key]; !ok {
			if err := unix.Removexattr(path, key); err != nil {
				return err
			}
		}
	}
	// Set all xattrs that are in the new map
	for key, val := range xattrs {
		if old, ok := current[key]; ok && bytes.Equal(old, val) {
			continue
		}
		if err := unix.Setxattr(path, key, val, 0); err != nil {
			return err
		}
	}

	return nil
}
