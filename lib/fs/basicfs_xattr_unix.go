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
	"sync"
	"syscall"

	"github.com/syncthing/syncthing/lib/protocol"
	"golang.org/x/sys/unix"
)

func (f *BasicFilesystem) GetXattr(path string, xattrFilter XattrFilter) ([]protocol.Xattr, error) {
	path, err := f.rooted(path)
	if err != nil {
		return nil, fmt.Errorf("get xattr %s: %w", path, err)
	}

	attrs, err := listXattr(path)
	if err != nil {
		return nil, fmt.Errorf("get xattr %s: %w", path, err)
	}

	res := make([]protocol.Xattr, 0, len(attrs))
	var totSize int
	for _, attr := range attrs {
		if !xattrFilter.Permit(attr) {
			l.Debugf("get xattr %s: skipping attribute %q denied by filter", path, attr)
			continue
		}
		val, err := getXattr(path, attr)
		var errNo syscall.Errno
		if errors.As(err, &errNo) && errNo == 0x5d {
			// ENOATTR, returned on BSD when asking for an attribute that
			// doesn't exist (any more?)
			continue
		} else if err != nil {
			return nil, fmt.Errorf("get xattr %s: %w", path, err)
		}
		if max := xattrFilter.GetMaxSingleEntrySize(); max > 0 && len(attr)+len(val) > max {
			l.Debugf("get xattr %s: attribute %q exceeds max size", path, attr)
			continue
		}
		totSize += len(attr) + len(val)
		if max := xattrFilter.GetMaxTotalSize(); max > 0 && totSize > max {
			l.Debugf("get xattr %s: attribute %q would cause max size to be exceeded", path, attr)
			continue
		}
		res = append(res, protocol.Xattr{
			Name:  attr,
			Value: val,
		})
	}
	return res, nil
}

var xattrBufPool = sync.Pool{
	New: func() any { return make([]byte, 1024) },
}

func getXattr(path, name string) ([]byte, error) {
	buf := xattrBufPool.Get().([]byte) //nolint:forcetypeassert
	defer func() {
		// Put the buffer back in the pool, or not if we're not supposed to
		// (we returned it to the caller).
		if buf != nil {
			xattrBufPool.Put(buf)
		}
	}()

	size, err := unix.Lgetxattr(path, name, buf)
	if errors.Is(err, unix.ERANGE) {
		// Buffer was too small. Figure out how large it needs to be, and
		// allocate.
		size, err = unix.Lgetxattr(path, name, nil)
		if err != nil {
			return nil, fmt.Errorf("Lgetxattr %s %q: %w", path, name, err)
		}
		if size > len(buf) {
			xattrBufPool.Put(buf)
			buf = make([]byte, size)
		}
		size, err = unix.Lgetxattr(path, name, buf)
	}
	if err != nil {
		return nil, fmt.Errorf("Lgetxattr %s %q: %w", path, name, err)
	}

	if size >= len(buf)/4*3 {
		// The buffer is adequately sized (at least three quarters of it is
		// used), return it as-is.
		val := buf[:size]
		buf = nil // Don't put it back in the pool.
		return val, nil
	}

	// The buffer is larger than required, copy the data to a new buffer of
	// the correct size. This avoids having lots of 1024-sized allocations
	// sticking around when 24 bytes or whatever would be enough.
	val := make([]byte, size)
	copy(val, buf)
	return val, nil
}

func (f *BasicFilesystem) SetXattr(path string, xattrs []protocol.Xattr, xattrFilter XattrFilter) error {
	// Index the new attribute set.
	xattrsIdx := make(map[string]int)
	for i, xa := range xattrs {
		xattrsIdx[xa.Name] = i
	}

	// Get and index the existing attribute set
	current, err := f.GetXattr(path, xattrFilter)
	if err != nil {
		return fmt.Errorf("set xattrs %s: GetXattr: %w", path, err)
	}
	currentIdx := make(map[string]int)
	for i, xa := range current {
		currentIdx[xa.Name] = i
	}

	path, err = f.rooted(path)
	if err != nil {
		return fmt.Errorf("set xattrs %s: %w", path, err)
	}

	// Remove all existing xattrs that are not in the new set
	for _, xa := range current {
		if _, ok := xattrsIdx[xa.Name]; !ok {
			if err := unix.Lremovexattr(path, xa.Name); err != nil {
				return fmt.Errorf("set xattrs %s: Removexattr %q: %w", path, xa.Name, err)
			}
		}
	}

	// Set all xattrs that are different in the new set
	for _, xa := range xattrs {
		if old, ok := currentIdx[xa.Name]; ok && bytes.Equal(xa.Value, current[old].Value) {
			continue
		}
		if err := unix.Lsetxattr(path, xa.Name, xa.Value, 0); err != nil {
			return fmt.Errorf("set xattrs %s: Setxattr %q: %w", path, xa.Name, err)
		}
	}

	return nil
}

func compact(ss []string) []string {
	i := 0
	for _, s := range ss {
		if s != "" {
			ss[i] = s
			i++
		}
	}
	return ss[:i]
}
