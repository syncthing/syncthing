// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"os"
	"path/filepath"
	"syscall"
)

// DebugSymlinkForTestsOnly is os.Symlink taken from the 1.9.2 stdlib,
// hacked with the SYMBOLIC_LINK_FLAG_ALLOW_UNPRIVILEGED_CREATE flag to
// create symlinks when not elevated.
//
// This is not and should not be used in Syncthing code, hence the
// cumbersome name to make it obvious if this ever leaks. Nonetheless it's
// useful in tests.
func DebugSymlinkForTestsOnly(oldFs, newFS Filesystem, oldname, newname string) error {
	oldname = filepath.Join(oldFs.URI(), oldname)
	newname = filepath.Join(newFS.URI(), newname)

	// CreateSymbolicLink is not supported before Windows Vista
	if syscall.LoadCreateSymbolicLink() != nil {
		return &os.LinkError{"symlink", oldname, newname, syscall.EWINDOWS}
	}

	// '/' does not work in link's content
	oldname = filepath.FromSlash(oldname)

	// need the exact location of the oldname when it's relative to determine if it's a directory
	destpath := oldname
	if !filepath.IsAbs(oldname) {
		destpath = filepath.Dir(newname) + `\` + oldname
	}

	fi, err := os.Lstat(destpath)
	isdir := err == nil && fi.IsDir()

	n, err := syscall.UTF16PtrFromString(fixLongPath(newname))
	if err != nil {
		return &os.LinkError{"symlink", oldname, newname, err}
	}
	o, err := syscall.UTF16PtrFromString(fixLongPath(oldname))
	if err != nil {
		return &os.LinkError{"symlink", oldname, newname, err}
	}

	var flags uint32
	if isdir {
		flags |= syscall.SYMBOLIC_LINK_FLAG_DIRECTORY
	}
	flags |= 0x02 // SYMBOLIC_LINK_FLAG_ALLOW_UNPRIVILEGED_CREATE
	err = syscall.CreateSymbolicLink(n, o, flags)
	if err != nil {
		return &os.LinkError{"symlink", oldname, newname, err}
	}
	return nil
}

// fixLongPath returns the extended-length (\\?\-prefixed) form of
// path when needed, in order to avoid the default 260 character file
// path limit imposed by Windows. If path is not easily converted to
// the extended-length form (for example, if path is a relative path
// or contains .. elements), or is short enough, fixLongPath returns
// path unmodified.
//
// See https://docs.microsoft.com/windows/win32/fileio/naming-a-file#maximum-path-length-limitation
func fixLongPath(path string) string {
	// Do nothing (and don't allocate) if the path is "short".
	// Empirically (at least on the Windows Server 2013 builder),
	// the kernel is arbitrarily okay with < 248 bytes. That
	// matches what the docs above say:
	// "When using an API to create a directory, the specified
	// path cannot be so long that you cannot append an 8.3 file
	// name (that is, the directory name cannot exceed MAX_PATH
	// minus 12)." Since MAX_PATH is 260, 260 - 12 = 248.
	//
	// The MS docs appear to say that a normal path that is 248 bytes long
	// will work; empirically the path must be less than 248 bytes long.
	if len(path) < 248 {
		// Don't fix. (This is how Go 1.7 and earlier worked,
		// not automatically generating the \\?\ form)
		return path
	}

	// The extended form begins with \\?\, as in
	// \\?\c:\windows\foo.txt or \\?\UNC\server\share\foo.txt.
	// The extended form disables evaluation of . and .. path
	// elements and disables the interpretation of / as equivalent
	// to \. The conversion here rewrites / to \ and elides
	// . elements as well as trailing or duplicate separators. For
	// simplicity it avoids the conversion entirely for relative
	// paths or paths containing .. elements. For now,
	// \\server\share paths are not converted to
	// \\?\UNC\server\share paths because the rules for doing so
	// are less well-specified.
	if len(path) >= 2 && path[:2] == `\\` {
		// Don't canonicalize UNC paths.
		return path
	}
	if !filepath.IsAbs(path) {
		// Relative path
		return path
	}

	const prefix = `\\?`

	pathbuf := make([]byte, len(prefix)+len(path)+len(`\`))
	copy(pathbuf, prefix)
	n := len(path)
	r, w := 0, len(prefix)
	for r < n {
		switch {
		case os.IsPathSeparator(path[r]):
			// empty block
			r++
		case path[r] == '.' && (r+1 == n || os.IsPathSeparator(path[r+1])):
			// /./
			r++
		case r+1 < n && path[r] == '.' && path[r+1] == '.' && (r+2 == n || os.IsPathSeparator(path[r+2])):
			// /../ is currently unhandled
			return path
		default:
			pathbuf[w] = '\\'
			w++
			for ; r < n && !os.IsPathSeparator(path[r]); r++ {
				pathbuf[w] = path[r]
				w++
			}
		}
	}
	// A drive's root directory needs a trailing \
	if w == len(`\\?\c:`) {
		pathbuf[w] = '\\'
		w++
	}
	return string(pathbuf[:w])
}
