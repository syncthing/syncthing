// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package osutil

import (
	"errors"
	"path/filepath"
	"runtime"

	"github.com/syncthing/syncthing/lib/fs"
)

var (
	ErrClosed  = errors.New("write to closed writer")
	TempPrefix = ".syncthing.tmp."
)

// An AtomicWriter is an *os.File that writes to a temporary file in the same
// directory as the final path. On successful Close the file is renamed to
// its final path. Any error on Write or during Close is accumulated and
// returned on Close, so a lazy user can ignore errors until Close.
type AtomicWriter struct {
	path string
	next fs.File
	fs   fs.Filesystem
	err  error
}

// CreateAtomic is like os.Create, except a temporary file name is used
// instead of the given name. The file is created with secure (0600)
// permissions.
func CreateAtomic(path string) (*AtomicWriter, error) {
	fs := fs.NewFilesystem(fs.FilesystemTypeBasic, filepath.Dir(path))
	return CreateAtomicFilesystem(fs, filepath.Base(path))
}

// CreateAtomicFilesystem is like os.Create, except a temporary file name is used
// instead of the given name. The file is created with secure (0600)
// permissions.
func CreateAtomicFilesystem(filesystem fs.Filesystem, path string) (*AtomicWriter, error) {
	// The security of this depends on the tempfile having secure
	// permissions, 0600, from the beginning. This is what ioutil.TempFile
	// does. We have a test that verifies that that is the case, should this
	// ever change in the standard library in the future.
	fd, err := TempFile(filesystem, filepath.Dir(path), TempPrefix)
	if err != nil {
		return nil, err
	}

	w := &AtomicWriter{
		path: path,
		next: fd,
		fs:   filesystem,
	}

	return w, nil
}

// Write is like io.Writer, but is a no-op on an already failed AtomicWriter.
func (w *AtomicWriter) Write(bs []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	n, err := w.next.Write(bs)
	if err != nil {
		w.err = err
		w.next.Close()
	}
	return n, err
}

// Close closes the temporary file and renames it to the final path. It is
// invalid to call Write() or Close() after Close().
func (w *AtomicWriter) Close() error {
	if w.err != nil {
		return w.err
	}

	// Try to not leave temp file around, but ignore error.
	defer w.fs.Remove(w.next.Name())

	if err := w.next.Sync(); err != nil {
		w.err = err
		return err
	}

	if err := w.next.Close(); err != nil {
		w.err = err
		return err
	}

	// Remove the destination file, on Windows only. If it fails, and not due
	// to the file not existing, we won't be able to complete the rename
	// either. Return this error because it may be more informative. On non-
	// Windows we want the atomic rename behavior so we don't attempt remove.
	if runtime.GOOS == "windows" {
		if err := w.fs.Remove(w.path); err != nil && !fs.IsNotExist(err) {
			return err
		}
	}

	if err := w.fs.Rename(w.next.Name(), w.path); err != nil {
		w.err = err
		return err
	}

	// fsync the directory too
	if fd, err := w.fs.Open(filepath.Dir(w.next.Name())); err == nil {
		fd.Sync()
		fd.Close()
	}

	// Set w.err to return appropriately for any future operations.
	w.err = ErrClosed

	return nil
}
