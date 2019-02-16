// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

var (
	errNoHome = errors.New("no home directory found - set $HOME (or the platform equivalent)")
	// Apparently BTRFS wants ioctl, XFS/NFS wants copy_file_range, so make this adjustable
	copyOptimisations = getCopyOptimisations()
)

func ExpandTilde(path string) (string, error) {
	if path == "~" {
		return getHomeDir()
	}

	path = filepath.FromSlash(path)
	if !strings.HasPrefix(path, fmt.Sprintf("~%c", PathSeparator)) {
		return path, nil
	}

	home, err := getHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, path[2:]), nil
}

func getHomeDir() (string, error) {
	var home string

	switch runtime.GOOS {
	case "windows":
		home = filepath.Join(os.Getenv("HomeDrive"), os.Getenv("HomePath"))
		if home == "" {
			home = os.Getenv("UserProfile")
		}
	default:
		home = os.Getenv("HOME")
	}

	if home == "" {
		return "", errNoHome
	}

	return home, nil
}

var windowsDisallowedCharacters = string([]rune{
	'<', '>', ':', '"', '|', '?', '*',
	0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10,
	11, 12, 13, 14, 15, 16, 17, 18, 19, 20,
	21, 22, 23, 24, 25, 26, 27, 28, 29, 30,
	31,
})

func WindowsInvalidFilename(name string) bool {
	// None of the path components should end in space
	for _, part := range strings.Split(name, `\`) {
		if len(part) == 0 {
			continue
		}
		if part[len(part)-1] == ' ' {
			// Names ending in space are not valid.
			return true
		}
	}

	// The path must not contain any disallowed characters
	return strings.ContainsAny(name, windowsDisallowedCharacters)
}

func IsParent(path, parent string) bool {
	if len(parent) == 0 {
		// The empty string is the parent of everything except the empty
		// string. (Avoids panic in the next step.)
		return len(path) > 0
	}
	if parent[len(parent)-1] != PathSeparator {
		parent += string(PathSeparator)
	}
	return strings.HasPrefix(path, parent)
}

func CopyRange(src, dst File, srcOffset, dstOffset, size int64) error {
	srcFile, srcOk := src.(fsFile)
	dstFile, dstOk := dst.(fsFile)
	if srcOk && dstOk {
		if err := copyRangeOptimised(srcFile, dstFile, srcOffset, dstOffset, size); err == nil {
			return nil
		}
	}

	return copyRangeGeneric(src, dst, srcOffset, dstOffset, size)
}

func copyRangeGeneric(src, dst File, srcOffset, dstOffset, size int64) error {
	oldOffset, err := src.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil
	}

	// Check that the source file has the data in question
	if fi, err := src.Stat(); err != nil {
		return err
	} else if fi.Size() < srcOffset+size {
		return io.ErrUnexpectedEOF
	}

	// Check that the destination file has sufficient space
	if fi, err := dst.Stat(); err != nil {
		return err
	} else if fi.Size() < dstOffset+size {
		if err := dst.Truncate(dstOffset + size); err != nil {
			return err
		}
	}

	if n, err := src.Seek(srcOffset, io.SeekStart); err != nil {
		return err
	} else if n != srcOffset {
		return io.ErrUnexpectedEOF
	}

	if n, err := dst.Seek(dstOffset, io.SeekStart); err != nil {
		return err
	} else if n != dstOffset {
		return io.ErrUnexpectedEOF
	}

	for size > 0 {
		n, err := io.CopyN(dst, src, size)
		if err != nil {
			_, _ = src.Seek(oldOffset, io.SeekStart)
			return err
		}
		size -= n
	}

	if n, err := src.Seek(oldOffset, io.SeekStart); err != nil {
		return err
	} else if n != oldOffset {
		return io.ErrUnexpectedEOF
	}
	return nil
}

func getCopyOptimisations() []string {
	opt := os.Getenv("STCOPYOPTIMISATIONS")
	if opt == "" {
		opt = "ioctl,copy_file_range,sendfile"
	}
	return strings.Split(opt, ",")
}
