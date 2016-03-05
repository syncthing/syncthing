// Copyright (c) 2012, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package storage provides storage abstraction for LevelDB.
package storage

import (
	"errors"
	"fmt"
	"io"

	"github.com/syndtr/goleveldb/leveldb/util"
)

type FileType int

const (
	TypeManifest FileType = 1 << iota
	TypeJournal
	TypeTable
	TypeTemp

	TypeAll = TypeManifest | TypeJournal | TypeTable | TypeTemp
)

func (t FileType) String() string {
	switch t {
	case TypeManifest:
		return "manifest"
	case TypeJournal:
		return "journal"
	case TypeTable:
		return "table"
	case TypeTemp:
		return "temp"
	}
	return fmt.Sprintf("<unknown:%d>", t)
}

var (
	ErrInvalidFile = errors.New("leveldb/storage: invalid file for argument")
	ErrLocked      = errors.New("leveldb/storage: already locked")
	ErrClosed      = errors.New("leveldb/storage: closed")
)

// ErrCorrupted is the type that wraps errors that indicate corruption of
// a file. Package storage has its own type instead of using
// errors.ErrCorrupted to prevent circular import.
type ErrCorrupted struct {
	Fd  FileDesc
	Err error
}

func (e *ErrCorrupted) Error() string {
	if !e.Fd.Nil() {
		return fmt.Sprintf("%v [file=%v]", e.Err, e.Fd)
	} else {
		return e.Err.Error()
	}
}

// Syncer is the interface that wraps basic Sync method.
type Syncer interface {
	// Sync commits the current contents of the file to stable storage.
	Sync() error
}

// Reader is the interface that groups the basic Read, Seek, ReadAt and Close
// methods.
type Reader interface {
	io.ReadSeeker
	io.ReaderAt
	io.Closer
}

// Writer is the interface that groups the basic Write, Sync and Close
// methods.
type Writer interface {
	io.WriteCloser
	Syncer
}

type Lock interface {
	util.Releaser
}

// FileDesc is a file descriptor.
type FileDesc struct {
	Type FileType
	Num  int64
}

func (fd FileDesc) String() string {
	switch fd.Type {
	case TypeManifest:
		return fmt.Sprintf("MANIFEST-%06d", fd.Num)
	case TypeJournal:
		return fmt.Sprintf("%06d.log", fd.Num)
	case TypeTable:
		return fmt.Sprintf("%06d.ldb", fd.Num)
	case TypeTemp:
		return fmt.Sprintf("%06d.tmp", fd.Num)
	default:
		return fmt.Sprintf("%#x-%d", fd.Type, fd.Num)
	}
}

// Nil returns true if fd == (FileDesc{}).
func (fd FileDesc) Nil() bool {
	return fd == (FileDesc{})
}

// FileDescOk returns true if fd is a valid file descriptor.
func FileDescOk(fd FileDesc) bool {
	switch fd.Type {
	case TypeManifest:
	case TypeJournal:
	case TypeTable:
	case TypeTemp:
	default:
		return false
	}
	return fd.Num >= 0
}

// Storage is the storage. A storage instance must be goroutine-safe.
type Storage interface {
	// Lock locks the storage. Any subsequent attempt to call Lock will fail
	// until the last lock released.
	// After use the caller should call the Release method.
	Lock() (Lock, error)

	// Log logs a string. This is used for logging.
	// An implementation may write to a file, stdout or simply do nothing.
	Log(str string)

	// SetMeta sets to point to the given fd, which then can be acquired using
	// GetMeta method.
	// SetMeta should be implemented in such way that changes should happened
	// atomically.
	SetMeta(fd FileDesc) error

	// GetManifest returns a manifest file.
	// Returns os.ErrNotExist if meta doesn't point to any fd, or point to fd
	// that doesn't exist.
	GetMeta() (FileDesc, error)

	// List returns fds that match the given file types.
	// The file types may be OR'ed together.
	List(ft FileType) ([]FileDesc, error)

	// Open opens file with the given fd read-only.
	// Returns os.ErrNotExist error if the file does not exist.
	// Returns ErrClosed if the underlying storage is closed.
	Open(fd FileDesc) (Reader, error)

	// Create creates file with the given fd, truncate if already exist and
	// opens write-only.
	// Returns ErrClosed if the underlying storage is closed.
	Create(fd FileDesc) (Writer, error)

	// Remove removes file with the given fd.
	// Returns ErrClosed if the underlying storage is closed.
	Remove(fd FileDesc) error

	// Rename renames file from oldfd to newfd.
	// Returns ErrClosed if the underlying storage is closed.
	Rename(oldfd, newfd FileDesc) error

	// Close closes the storage.
	// It is valid to call Close multiple times. Other methods should not be
	// called after the storage has been closed.
	Close() error
}
