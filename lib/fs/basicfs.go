// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/syncthing/syncthing/lib/build"
	"github.com/shirou/gopsutil/v3/disk"
)

var (
	errInvalidFilenameEmpty               = errors.New("name is invalid, must not be empty")
	errInvalidFilenameIosDotFile          = errors.New("name is invalid, must not start with period on iOS")
	errInvalidFilenameWindowsSpacePeriod  = errors.New("name is invalid, must not end in space or period on Windows")
	errInvalidFilenameWindowsReservedName = errors.New("name is invalid, contains Windows reserved name (NUL, COM1, etc.)")
	errInvalidFilenameWindowsReservedChar = errors.New("name is invalid, contains Windows reserved character (?, *, etc.)")
	errNotRelative                        = errors.New("not a relative path")
)

type OptionJunctionsAsDirs struct{}

func (o *OptionJunctionsAsDirs) apply(fs Filesystem) {
	if basic, ok := fs.(*BasicFilesystem); !ok {
		l.Warnln("WithJunctionsAsDirs must only be used with FilesystemTypeBasic")
	} else {
		basic.junctionsAsDirs = true
	}
}

func (o *OptionJunctionsAsDirs) String() string {
	return "junctionsAsDirs"
}

// The BasicFilesystem implements all aspects by delegating to package os.
// All paths are relative to the root and cannot (should not) escape the root directory.
type BasicFilesystem struct {
	root            string
	junctionsAsDirs bool
	options         []Option
}

func newBasicFilesystem(root string, opts ...Option) *BasicFilesystem {
	if root == "" {
		root = "." // Otherwise "" becomes "/" below
	}

	// The reason it's done like this:
	// C:          ->  C:\            ->  C:\        (issue that this is trying to fix)
	// C:\somedir  ->  C:\somedir\    ->  C:\somedir
	// C:\somedir\ ->  C:\somedir\\   ->  C:\somedir
	// This way in the tests, we get away without OS specific separators
	// in the test configs.
	sep := string(filepath.Separator)
	root = filepath.Dir(root + sep)

	if build.IsIOS() && !filepath.IsAbs(root) && root[0] != '~' {
	  newroot, err2 := rooted(root, "~/Documents")
		if err2 == nil {
		  root = newroot
		} else {
		  l.Warnln("Illegal folder", root, "-", err2)
			// Cannot error from here so use an unwritable path that will fail later
			root = "~/bad"
		}
	}

	// Attempt tilde expansion; leave unchanged in case of error
	if path, err := ExpandTilde(root); err == nil {
		root = path
	}

	// Attempt absolutification; leave unchanged in case of error
	if !filepath.IsAbs(root) {
		// Abs() looks like a fairly expensive syscall on Windows, while
		// IsAbs() is a whole bunch of string mangling. I think IsAbs() may be
		// somewhat faster in the general case, hence the outer if...
		if path, err := filepath.Abs(root); err == nil {
			root = path
		}
	}

	// Attempt to enable long filename support on Windows. We may still not
	// have an absolute path here if the previous steps failed.
	if runtime.GOOS == "windows" {
		root = longFilenameSupport(root)
	}

	fs := &BasicFilesystem{
		root:    root,
		options: opts,
	}
	for _, opt := range opts {
		opt.apply(fs)
	}
	return fs
}

// rooted expands the relative path to the full path that is then used with os
// package. If the relative path somehow causes the final path to escape the root
// directory, this returns an error, to prevent accessing files that are not in the
// shared directory.
func (f *BasicFilesystem) rooted(rel string) (string, error) {
	return rooted(rel, f.root)
}

func rooted(rel, root string) (string, error) {
	// The root must not be empty.
	if root == "" {
		return "", errInvalidFilenameEmpty
	}

	var err error
	// Takes care that rel does not try to escape
	rel, err = Canonicalize(rel)
	if err != nil {
		return "", err
	}

	return filepath.Join(root, rel), nil
}

func (f *BasicFilesystem) unrooted(path string) string {
	return rel(path, f.root)
}

func (f *BasicFilesystem) Chmod(name string, mode FileMode) error {
	name, err := f.rooted(name)
	if err != nil {
		return err
	}
	return os.Chmod(name, os.FileMode(mode))
}

func (f *BasicFilesystem) Lchown(name string, uid, gid int) error {
	name, err := f.rooted(name)
	if err != nil {
		return err
	}
	return os.Lchown(name, uid, gid)
}

func (f *BasicFilesystem) Chtimes(name string, atime time.Time, mtime time.Time) error {
	name, err := f.rooted(name)
	if err != nil {
		return err
	}
	return os.Chtimes(name, atime, mtime)
}

func (f *BasicFilesystem) Mkdir(name string, perm FileMode) error {
	name, err := f.rooted(name)
	if err != nil {
		return err
	}
	return os.Mkdir(name, os.FileMode(perm))
}

// MkdirAll creates a directory named path, along with any necessary parents,
// and returns nil, or else returns an error.
// The permission bits perm are used for all directories that MkdirAll creates.
// If path is already a directory, MkdirAll does nothing and returns nil.
func (f *BasicFilesystem) MkdirAll(path string, perm FileMode) error {
	path, err := f.rooted(path)
	if err != nil {
		return err
	}

	return f.mkdirAll(path, os.FileMode(perm))
}

func (f *BasicFilesystem) Lstat(name string) (FileInfo, error) {
	name, err := f.rooted(name)
	if err != nil {
		return nil, err
	}
	fi, err := f.underlyingLstat(name)
	if err != nil {
		return nil, err
	}
	return basicFileInfo{fi}, err
}

func (f *BasicFilesystem) Remove(name string) error {
	name, err := f.rooted(name)
	if err != nil {
		return err
	}
	return os.Remove(name)
}

func (f *BasicFilesystem) RemoveAll(name string) error {
	name, err := f.rooted(name)
	if err != nil {
		return err
	}
	return os.RemoveAll(name)
}

func (f *BasicFilesystem) Rename(oldpath, newpath string) error {
	oldpath, err := f.rooted(oldpath)
	if err != nil {
		return err
	}
	newpath, err = f.rooted(newpath)
	if err != nil {
		return err
	}
	return os.Rename(oldpath, newpath)
}

func (f *BasicFilesystem) Stat(name string) (FileInfo, error) {
	name, err := f.rooted(name)
	if err != nil {
		return nil, err
	}
	fi, err := os.Stat(name)
	if err != nil {
		return nil, err
	}
	return basicFileInfo{fi}, err
}

func (f *BasicFilesystem) DirNames(name string) ([]string, error) {
	name, err := f.rooted(name)
	if err != nil {
		return nil, err
	}
	fd, err := os.OpenFile(name, OptReadOnly, 0777)
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	names, err := fd.Readdirnames(-1)
	if err != nil {
		return nil, err
	}

	return names, nil
}

func (f *BasicFilesystem) Open(name string) (File, error) {
	rootedName, err := f.rooted(name)
	if err != nil {
		return nil, err
	}
	fd, err := os.Open(rootedName)
	if err != nil {
		return nil, err
	}
	return basicFile{fd, name}, err
}

func (f *BasicFilesystem) OpenFile(name string, flags int, mode FileMode) (File, error) {
	rootedName, err := f.rooted(name)
	if err != nil {
		return nil, err
	}
	fd, err := os.OpenFile(rootedName, flags, os.FileMode(mode))
	if err != nil {
		return nil, err
	}
	return basicFile{fd, name}, err
}

func (f *BasicFilesystem) Create(name string) (File, error) {
	rootedName, err := f.rooted(name)
	if err != nil {
		return nil, err
	}
	fd, err := os.Create(rootedName)
	if err != nil {
		return nil, err
	}
	return basicFile{fd, name}, err
}

func (f *BasicFilesystem) Walk(root string, walkFn WalkFunc) error {
	// implemented in WalkFilesystem
	return errors.New("not implemented")
}

func (f *BasicFilesystem) Glob(pattern string) ([]string, error) {
	pattern, err := f.rooted(pattern)
	if err != nil {
		return nil, err
	}
	files, err := filepath.Glob(pattern)
	unrooted := make([]string, len(files))
	for i := range files {
		unrooted[i] = f.unrooted(files[i])
	}
	return unrooted, err
}

func (f *BasicFilesystem) Usage(name string) (Usage, error) {
	name, err := f.rooted(name)
	if err != nil {
		return Usage{}, err
	}
	u, err := disk.Usage(name)
	if err != nil {
		return Usage{}, err
	}
	return Usage{
		Free:  u.Free,
		Total: u.Total,
	}, nil
}

func (f *BasicFilesystem) Type() FilesystemType {
	return FilesystemTypeBasic
}

func (f *BasicFilesystem) URI() string {
	return strings.TrimPrefix(f.root, `\\?\`)
}

func (f *BasicFilesystem) Options() []Option {
	return f.options
}

func (f *BasicFilesystem) SameFile(fi1, fi2 FileInfo) bool {
	// Like os.SameFile, we always return false unless fi1 and fi2 were created
	// by this package's Stat/Lstat method.
	f1, ok1 := fi1.(basicFileInfo)
	f2, ok2 := fi2.(basicFileInfo)
	if !ok1 || !ok2 {
		return false
	}

	return os.SameFile(f1.osFileInfo(), f2.osFileInfo())
}

func (f *BasicFilesystem) underlying() (Filesystem, bool) {
	return nil, false
}

func (f *BasicFilesystem) wrapperType() filesystemWrapperType {
	return filesystemWrapperTypeNone
}

// basicFile implements the fs.File interface on top of an os.File
type basicFile struct {
	*os.File
	name string
}

func (f basicFile) Name() string {
	return f.name
}

func (f basicFile) Stat() (FileInfo, error) {
	info, err := f.File.Stat()
	if err != nil {
		return nil, err
	}
	return basicFileInfo{info}, nil
}

// basicFileInfo implements the fs.FileInfo interface on top of an os.FileInfo.
type basicFileInfo struct {
	os.FileInfo
}

func (e basicFileInfo) IsSymlink() bool {
	// Must use basicFileInfo.Mode() because it may apply magic.
	return e.Mode()&ModeSymlink != 0
}

func (e basicFileInfo) IsRegular() bool {
	// Must use basicFileInfo.Mode() because it may apply magic.
	return e.Mode()&ModeType == 0
}

// longFilenameSupport adds the necessary prefix to the path to enable long
// filename support on windows if necessary.
// This does NOT check the current system, i.e. will also take effect on unix paths.
func longFilenameSupport(path string) string {
	if filepath.IsAbs(path) && !strings.HasPrefix(path, `\\`) {
		return `\\?\` + path
	}
	return path
}

type ErrWatchEventOutsideRoot struct{ msg string }

func (e *ErrWatchEventOutsideRoot) Error() string {
	return e.msg
}

func (f *BasicFilesystem) newErrWatchEventOutsideRoot(absPath string, roots []string) *ErrWatchEventOutsideRoot {
	return &ErrWatchEventOutsideRoot{fmt.Sprintf("Watching for changes encountered an event outside of the filesystem root: f.root==%v, roots==%v, path==%v. This should never happen, please report this message to forum.syncthing.net.", f.root, roots, absPath)}
}
