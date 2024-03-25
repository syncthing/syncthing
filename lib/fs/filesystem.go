// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/syncthing/syncthing/lib/ignore/ignoreresult"
	"github.com/syncthing/syncthing/lib/protocol"
)

type filesystemWrapperType int32

const (
	filesystemWrapperTypeNone filesystemWrapperType = iota
	filesystemWrapperTypeMtime
	filesystemWrapperTypeCase
	filesystemWrapperTypeError
	filesystemWrapperTypeWalk
	filesystemWrapperTypeLog
	filesystemWrapperTypeMetrics
)

type XattrFilter interface {
	Permit(string) bool
	GetMaxSingleEntrySize() int
	GetMaxTotalSize() int
}

// The Filesystem interface abstracts access to the file system.
type Filesystem interface {
	Chmod(name string, mode FileMode) error
	Lchown(name string, uid, gid string) error // uid/gid as strings; numeric on POSIX, SID on Windows, like in os/user package
	Chtimes(name string, atime time.Time, mtime time.Time) error
	Create(name string) (File, error)
	CreateSymlink(target, name string) error
	DirNames(name string) ([]string, error)
	Lstat(name string) (FileInfo, error)
	Mkdir(name string, perm FileMode) error
	MkdirAll(name string, perm FileMode) error
	Open(name string) (File, error)
	OpenFile(name string, flags int, mode FileMode) (File, error)
	ReadSymlink(name string) (string, error)
	Remove(name string) error
	RemoveAll(name string) error
	Rename(oldname, newname string) error
	Stat(name string) (FileInfo, error)
	SymlinksSupported() bool
	Walk(name string, walkFn WalkFunc) error
	// If setup fails, returns non-nil error, and if afterwards a fatal (!)
	// error occurs, sends that error on the channel. Afterwards this watch
	// can be considered stopped.
	Watch(path string, ignore Matcher, ctx context.Context, ignorePerms bool) (<-chan Event, <-chan error, error)
	Hide(name string) error
	Unhide(name string) error
	Glob(pattern string) ([]string, error)
	Roots() ([]string, error)
	Usage(name string) (Usage, error)
	Type() FilesystemType
	URI() string
	Options() []Option
	SameFile(fi1, fi2 FileInfo) bool
	PlatformData(name string, withOwnership, withXattrs bool, xattrFilter XattrFilter) (protocol.PlatformData, error)
	GetXattr(name string, xattrFilter XattrFilter) ([]protocol.Xattr, error)
	SetXattr(path string, xattrs []protocol.Xattr, xattrFilter XattrFilter) error
	ValidPath(name string) error

	// Used for unwrapping things
	underlying() (Filesystem, bool)
	wrapperType() filesystemWrapperType
}

// The File interface abstracts access to a regular file, being a somewhat
// smaller interface than os.File
type File interface {
	io.Closer
	io.Reader
	io.ReaderAt
	io.Seeker
	io.Writer
	io.WriterAt
	Name() string
	Truncate(size int64) error
	Stat() (FileInfo, error)
	Sync() error
}

// The FileInfo interface is almost the same as os.FileInfo, but with the
// Sys method removed (as we don't want to expose whatever is underlying)
// and with a couple of convenience methods added.
type FileInfo interface {
	// Standard things present in os.FileInfo
	Name() string
	Mode() FileMode
	Size() int64
	ModTime() time.Time
	IsDir() bool
	Sys() interface{}
	// Extensions
	IsRegular() bool
	IsSymlink() bool
	Owner() int
	Group() int
	InodeChangeTime() time.Time // may be zero if not supported
}

// FileMode is similar to os.FileMode
type FileMode uint32

func (fm FileMode) String() string {
	return os.FileMode(fm).String()
}

// Usage represents filesystem space usage
type Usage struct {
	Free  uint64
	Total uint64
}

type Matcher interface {
	Match(name string) ignoreresult.R
}

type Event struct {
	Name string
	Type EventType
}

type EventType int

const (
	NonRemove EventType = 1 + iota
	Remove
	Mixed // Should probably not be necessary to be used in filesystem interface implementation
)

// Merge returns Mixed, except if evType and other are the same and not Mixed.
func (evType EventType) Merge(other EventType) EventType {
	return evType | other
}

func (evType EventType) String() string {
	switch {
	case evType == NonRemove:
		return "non-remove"
	case evType == Remove:
		return "remove"
	case evType == Mixed:
		return "mixed"
	default:
		panic("bug: Unknown event type")
	}
}

var (
	ErrWatchNotSupported  = errors.New("watching is not supported")
	ErrXattrsNotSupported = errors.New("extended attributes are not supported on this platform")
)

// Equivalents from os package.

const (
	ModePerm      = FileMode(os.ModePerm)
	ModeSetgid    = FileMode(os.ModeSetgid)
	ModeSetuid    = FileMode(os.ModeSetuid)
	ModeSticky    = FileMode(os.ModeSticky)
	ModeSymlink   = FileMode(os.ModeSymlink)
	ModeType      = FileMode(os.ModeType)
	PathSeparator = os.PathSeparator
	OptAppend     = os.O_APPEND
	OptCreate     = os.O_CREATE
	OptExclusive  = os.O_EXCL
	OptReadOnly   = os.O_RDONLY
	OptReadWrite  = os.O_RDWR
	OptSync       = os.O_SYNC
	OptTruncate   = os.O_TRUNC
	OptWriteOnly  = os.O_WRONLY
)

// SkipDir is used as a return value from WalkFuncs to indicate that
// the directory named in the call is to be skipped. It is not returned
// as an error by any function.
var SkipDir = filepath.SkipDir

func IsExist(err error) bool {
	return errors.Is(err, ErrExist)
}

// ErrExist is the equivalent of os.ErrExist
var ErrExist = fs.ErrExist

// IsNotExist is the equivalent of os.IsNotExist
func IsNotExist(err error) bool {
	return errors.Is(err, ErrNotExist)
}

// ErrNotExist is the equivalent of os.ErrNotExist
var ErrNotExist = fs.ErrNotExist

// IsPermission is the equivalent of os.IsPermission
func IsPermission(err error) bool {
	return errors.Is(err, fs.ErrPermission)
}

// IsPathSeparator is the equivalent of os.IsPathSeparator
var IsPathSeparator = os.IsPathSeparator

// Option modifies a filesystem at creation. An option might be specific
// to a filesystem-type.
//
// String is used to detect options with the same effect, i.e. must be different
// for options with different effects. Meaning if an option has parameters, a
// representation of those must be part of the returned string.
type Option interface {
	String() string
	apply(Filesystem) Filesystem
}

func NewFilesystem(fsType FilesystemType, uri string, opts ...Option) Filesystem {
	var caseOpt Option
	var mtimeOpt Option
	i := 0
	for _, opt := range opts {
		if caseOpt != nil && mtimeOpt != nil {
			break
		}
		switch opt.(type) {
		case *OptionDetectCaseConflicts:
			caseOpt = opt
		case *optionMtime:
			mtimeOpt = opt
		default:
			opts[i] = opt
			i++
		}
	}
	opts = opts[:i]

	var fs Filesystem
	switch fsType {
	case FilesystemTypeBasic:
		fs = newBasicFilesystem(uri, opts...)
	case FilesystemTypeFake:
		fs = newFakeFilesystem(uri, opts...)
	default:
		l.Debugln("Unknown filesystem", fsType, uri)
		fs = &errorFilesystem{
			fsType: fsType,
			uri:    uri,
			err:    errors.New("filesystem with type " + fsType.String() + " does not exist."),
		}
	}

	// Case handling is the innermost, as any filesystem calls by wrappers should be case-resolved
	if caseOpt != nil {
		fs = caseOpt.apply(fs)
	}

	// mtime handling should happen inside walking, as filesystem calls while
	// walking should be mtime-resolved too
	if mtimeOpt != nil {
		fs = mtimeOpt.apply(fs)
	}

	fs = &metricsFS{next: fs}

	if l.ShouldDebug("walkfs") {
		return NewWalkFilesystem(&logFilesystem{fs})
	}

	if l.ShouldDebug("fs") {
		return &logFilesystem{NewWalkFilesystem(fs)}
	}

	return NewWalkFilesystem(fs)
}

// IsInternal returns true if the file, as a path relative to the folder
// root, represents an internal file that should always be ignored. The file
// path must be clean (i.e., in canonical shortest form).
func IsInternal(file string) bool {
	// fs cannot import config or versioner, so we hard code .stfolder
	// (config.DefaultMarkerName) and .stversions (versioner.DefaultPath)
	internals := []string{".stfolder", ".stignore", ".stversions"}
	for _, internal := range internals {
		if file == internal {
			return true
		}
		if IsParent(file, internal) {
			return true
		}
	}
	return false
}

var (
	errPathInvalid           = errors.New("path is invalid")
	errPathTraversingUpwards = errors.New("relative path traversing upwards (starting with ..)")
)

// Canonicalize checks that the file path is valid and returns it in the "canonical" form:
// - /foo/bar -> foo/bar
// - / -> "."
func Canonicalize(file string) (string, error) {
	const pathSep = string(PathSeparator)

	if strings.HasPrefix(file, pathSep+pathSep) {
		// The relative path may pretend to be an absolute path within
		// the root, but the double path separator on Windows implies
		// something else and is out of spec.
		return "", errPathInvalid
	}

	// The relative path should be clean from internal dotdots and similar
	// funkyness.
	file = filepath.Clean(file)

	// It is not acceptable to attempt to traverse upwards.
	if file == ".." {
		return "", errPathTraversingUpwards
	}
	if strings.HasPrefix(file, ".."+pathSep) {
		return "", errPathTraversingUpwards
	}

	if strings.HasPrefix(file, pathSep) {
		if file == pathSep {
			return ".", nil
		}
		return file[1:], nil
	}

	return file, nil
}

// unwrapFilesystem removes "wrapping" filesystems to expose the filesystem of the requested wrapperType, if it exists.
func unwrapFilesystem(fs Filesystem, wrapperType filesystemWrapperType) (Filesystem, bool) {
	var ok bool
	for {
		if fs.wrapperType() == wrapperType {
			return fs, true
		}
		fs, ok = fs.underlying()
		if !ok {
			return nil, false
		}
	}
}

// WriteFile writes data to the named file, creating it if necessary.
// If the file does not exist, WriteFile creates it with permissions perm (before umask);
// otherwise WriteFile truncates it before writing, without changing permissions.
// Since Writefile requires multiple system calls to complete, a failure mid-operation
// can leave the file in a partially written state.
func WriteFile(fs Filesystem, name string, data []byte, perm FileMode) error {
	f, err := fs.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err1 := f.Close(); err1 != nil && err == nil {
		err = err1
	}
	return err
}
