// Copyright (C) 2022 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build !windows && !dragonfly && !illumos && !solaris && !openbsd
// +build !windows,!dragonfly,!illumos,!solaris,!openbsd

package fs

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/syncthing/syncthing/lib/build"
	"github.com/syncthing/syncthing/lib/protocol"
	"golang.org/x/sys/unix"
)

func (f *BasicFilesystem) GetXattr(path string, xattrFilter StringFilter) ([]protocol.Xattr, error) {
	path, err := f.rooted(path)
	if err != nil {
		return nil, fmt.Errorf("get xattr %q: %w", path, err)
	}

	attrs, err := listXattr(path)
	if err != nil {
		return nil, fmt.Errorf("get xattr %q: %w", path, err)
	}

	var res []protocol.Xattr
	var val, buf []byte
	for _, attr := range attrs {
		if attr == "" {
			continue
		}
		if !xattrFilter.Permit(attr) {
			continue
		}
		val, buf, err = getXattr(path, attr, buf)
		if err != nil {
			return nil, fmt.Errorf("get xattr %q: %w", path, err)
		}
		res = append(res, protocol.Xattr{
			Name:  attr,
			Value: val,
		})
	}
	sort.Slice(res, func(a, b int) bool {
		return res[a].Name < res[b].Name
	})
	return res, nil
}

func listXattr(path string) ([]string, error) {
	buf := make([]byte, 1024)
	size, err := unix.Llistxattr(path, buf)
	if errors.Is(err, unix.ERANGE) {
		// Buffer is too small. Try again with a zero sized buffer to get
		// the size, then allocate a buffer of the correct size.
		size, err = unix.Listxattr(path, nil)
		if err != nil {
			return nil, fmt.Errorf("Listxattr %q: %w", path, err)
		}
		if size > len(buf) {
			buf = make([]byte, size)
		}
		size, err = unix.Llistxattr(path, buf)
	}
	if err != nil {
		return nil, fmt.Errorf("Listxattr %q: %w", path, err)
	}

	buf = buf[:size]
	var attrs []string
	switch {
	case build.IsFreeBSD, build.IsNetBSD:
		// "Each list entry consists of a single byte containing the length
		// of the attribute name, followed by the attribute name.  The
		// attrbute name is not terminated by ASCII 0 (nul)."
		i := 0
		for i < len(buf) {
			l := int(buf[i])
			i++
			if i+l >= len(buf) {
				// uh-oh
				return nil, fmt.Errorf("get xattr %q: attribute length %d exceeds buffer length", path, l)
			}
			attrs = append(attrs, string(buf[i:i+l]))
			i += l
		}
	default:
		// "The list is the set of (null-terminated) names, one after the
		// other."
		attrs = strings.Split(string(buf), "\x00")
	}

	return attrs, nil
}

func getXattr(path, name string, buf []byte) (val []byte, rest []byte, err error) {
	if len(buf) == 0 {
		buf = make([]byte, 1024)
	}
	size, err := unix.Lgetxattr(path, name, buf)
	if errors.Is(err, unix.ERANGE) {
		// Buffer was too small. Figure out how large it needs to be, and
		// allocate.
		size, err = unix.Lgetxattr(path, name, nil)
		if err != nil {
			return nil, nil, fmt.Errorf("Lgetxattr %q %q: %w", path, name, err)
		}
		if size > len(buf) {
			buf = make([]byte, size)
		}
		size, err = unix.Lgetxattr(path, name, buf)
	}
	if err != nil {
		return nil, buf, fmt.Errorf("Lgetxattr %q %q: %w", path, name, err)
	}
	return buf[:size], buf[size:], nil
}

func (f *BasicFilesystem) SetXattr(path string, xattrs []protocol.Xattr, xattrFilter StringFilter) error {
	// Index the new attribute set
	xattrsIdx := make(map[string]int)
	for i, xa := range xattrs {
		xattrsIdx[xa.Name] = i
	}

	// Get and index the existing attribute set
	current, err := f.GetXattr(path, xattrFilter)
	if err != nil {
		return fmt.Errorf("set xattrs %q: GetXattr: %w", path, err)
	}
	currentIdx := make(map[string]int)
	for i, xa := range current {
		currentIdx[xa.Name] = i
	}

	path, err = f.rooted(path)
	if err != nil {
		return fmt.Errorf("set xattrs %q: %w", path, err)
	}

	// Remove all existing xattrs that are not in the new set
	for _, xa := range current {
		if _, ok := xattrsIdx[xa.Name]; !ok {
			if err := unix.Removexattr(path, xa.Name); err != nil {
				return fmt.Errorf("set xattrs %q: Removexattr %q: %w", path, xa.Name, err)
			}
		}
	}

	// Set all xattrs that are different in the new set
	for _, xa := range xattrs {
		if old, ok := currentIdx[xa.Name]; ok && bytes.Equal(xa.Value, current[old].Value) {
			continue
		}
		if err := unix.Setxattr(path, xa.Name, xa.Value, 0); err != nil {
			return fmt.Errorf("set xattrs %q: Setxattr %q: %w", path, xa.Name, err)

		}
	}

	return nil
}
