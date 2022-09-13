// Copyright (C) 2022 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build freebsd || netbsd
// +build freebsd netbsd

package fs

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	namespaces        = [...]int{unix.EXTATTR_NAMESPACE_USER, unix.EXTATTR_NAMESPACE_SYSTEM}
	namespacePrefixes = [...]string{unix.EXTATTR_NAMESPACE_USER: "user.", unix.EXTATTR_NAMESPACE_SYSTEM: "system."}
)

func listXattr(path string) ([]string, error) {
	var attrs []string

	// List the two namespaces explicitly and prefix any results with the
	// namespace name.
	for _, nsid := range namespaces {
		buf := make([]byte, 1024)
		size, err := unixLlistxattr(path, buf, nsid)
		if errors.Is(err, unix.ERANGE) || size == len(buf) {
			// Buffer is too small. Try again with a zero sized buffer to
			// get the size, then allocate a buffer of the correct size. We
			// inlude the size == len(buf) because apparently macOS doesn't
			// return ERANGE as it should -- no harm done, just an extra
			// read if we happened to need precisely 1024 bytes on the first
			// pass.
			size, err = unixLlistxattr(path, nil, nsid)
			if err != nil {
				return nil, fmt.Errorf("Listxattr %s: %w", path, err)
			}
			buf = make([]byte, size)
			size, err = unixLlistxattr(path, buf, nsid)
		}
		if err != nil {
			return nil, fmt.Errorf("Listxattr %s: %w", path, err)
		}

		buf = buf[:size]

		// "Each list entry consists of a single byte containing the length
		// of the attribute name, followed by the attribute name.  The
		// attribute name is not terminated by ASCII 0 (nul)."
		i := 0
		for i < len(buf) {
			l := int(buf[i])
			i++
			if i+l > len(buf) {
				// uh-oh
				return nil, fmt.Errorf("get xattr %s: attribute length %d at offset %d exceeds buffer length %d", path, l, i, len(buf))
			}
			if l > 0 {
				attrs = append(attrs, namespacePrefixes[nsid]+string(buf[i:i+l]))
				i += l
			}
		}
	}

	sort.Strings(attrs)
	return attrs, nil
}

// This is unix.Llistxattr except taking a namespace parameter to dodge
// https://github.com/golang/go/issues/54357 ("Listxattr on FreeBSD loses
// namespace info")
func unixLlistxattr(link string, dest []byte, nsid int) (sz int, err error) {
	d := initxattrdest(dest, 0)
	destsiz := len(dest)

	s, e := unix.ExtattrListLink(link, nsid, uintptr(d), destsiz)
	if e != nil && e == unix.EPERM && nsid != unix.EXTATTR_NAMESPACE_USER {
		return 0, nil
	} else if e != nil {
		return s, e
	}

	return s, nil
}

var _zero uintptr

func initxattrdest(dest []byte, idx int) (d unsafe.Pointer) {
	if len(dest) > idx {
		return unsafe.Pointer(&dest[idx])
	} else {
		return unsafe.Pointer(_zero)
	}
}
