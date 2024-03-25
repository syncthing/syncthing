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
	"os/user"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/shirou/gopsutil/v3/disk"
	"github.com/syncthing/syncthing/lib/build"
)

var (
	errInvalidFilenameEmpty                 = errors.New("name is invalid, must not be empty")
	errInvalidFilenameWindowsSpacePeriod    = errors.New("name is invalid, must not end in space or period on Windows")
	errInvalidFilenameWindowsReservedName   = errors.New("name is invalid, contains Windows reserved name")
	errInvalidFilenameWindowsReservedChar   = errors.New("name is invalid, contains Windows reserved character")
	errNotRelative                          = errors.New("not a relative path")
	errInvalidFilenameSyncthingReservedChar = errors.New("name is invalid, contains Syncthing reserved character (\\uf000-\\uf0ff)")
	filenameContainsSyncthingReservedChars  = "Skipping %q as the name contains Syncthing reserved characters (\\uf0xx)"
)

type OptionJunctionsAsDirs struct{}

func (*OptionJunctionsAsDirs) apply(fs Filesystem) Filesystem {
	if basic, ok := fs.(*BasicFilesystem); !ok {
		l.Warnln("WithJunctionsAsDirs must only be used with FilesystemTypeBasic")
	} else {
		basic.junctionsAsDirs = true
	}
	return fs
}

func (*OptionJunctionsAsDirs) String() string {
	return "junctionsAsDirs"
}

// The BasicFilesystem implements all aspects by delegating to package os.
// All paths are relative to the root and cannot (should not) escape the root directory.
type BasicFilesystem struct {
	root            string
	junctionsAsDirs bool
	encoderType     FilesystemEncoderType
	encoder         encoder
	options         []Option
	userCache       *userCache
	groupCache      *groupCache
}

type (
	userCache  = valueCache[string, *user.User]
	groupCache = valueCache[string, *user.Group]
)

func newBasicFilesystem(root string, opts ...Option) *BasicFilesystem {
	if root == "" {
		root = "." // Otherwise "" becomes "/" below
	}

	// The reason it's done like this:
	// C:          ->  C:\            ->  C:\        (issue that this is trying to fix)
	// C:\somedir  ->  C:\somedir\    ->  C:\somedir
	// C:\somedir\ ->  C:\somedir\    ->  C:\somedir
	// This way in the tests, we get away without OS specific separators
	// in the test configs.
	sep := string(filepath.Separator)
	root = filepath.Clean(filepath.Dir(root + sep))

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
	if build.IsWindows {
		root = longFilenameSupport(root)
	}

	fs := &BasicFilesystem{
		root:        root,
		options:     opts,
		userCache:   newValueCache(time.Hour, user.LookupId),
		groupCache:  newValueCache(time.Hour, user.LookupGroupId),
		encoderType: DefaultFilesystemEncoderType,
	}
	for _, opt := range opts {
		opt.apply(fs)
	}

	fs.encoder = GetEncoder(fs.encoderType)
	return fs
}

// rooted expands the relative path to the full path that is then used with os
// package. If the relative path somehow causes the final path to escape the root
// directory, this returns an error, to prevent accessing files that are not in the
// shared directory.
func (f *BasicFilesystem) rooted(rel string, op string) (string, error) {
	if f.ignore(rel) {
		switch op {
		case "mkdir", "readdir", "watch":
			return "", &os.PathError{Op: op, Path: rel, Err: syscall.ENOTDIR}
		default:
			return "", &os.PathError{Op: op, Path: rel, Err: syscall.ENOENT}
		}
	}
	if f.isEncoding() { // was f.encoder.CharsToEncode() != ""
		if l.ShouldDebug("encoder") {
			encoded := f.encoder.encode(rel)
			if encoded != rel {
				l.Debugf("Encoded %q as %q via %v encoder", rel, encoded, f.encoderType)
			}
		}
		rel = f.encoder.encode(rel)
	}
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
	rel := rel(path, f.root)
	if f.encoder != nil {
		rel = decode(rel)
	}
	return rel
}

func (f *BasicFilesystem) Chmod(name string, mode FileMode) error {
	name, err := f.rooted(name, "chmod")
	if err != nil {
		return err
	}
	return os.Chmod(name, os.FileMode(mode))
}

func (f *BasicFilesystem) Chtimes(name string, atime time.Time, mtime time.Time) error {
	name, err := f.rooted(name, "chtimes")
	if err != nil {
		return err
	}
	return os.Chtimes(name, atime, mtime)
}

func (f *BasicFilesystem) Mkdir(name string, perm FileMode) error {
	name, err := f.rooted(name, "mkdir")
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
	path, err := f.rooted(path, "mkdir")
	if err != nil {
		return err
	}

	return f.mkdirAll(path, os.FileMode(perm))
}

func (f *BasicFilesystem) Lstat(name string) (FileInfo, error) {
	name, err := f.rooted(name, "lstat")
	if err != nil {
		return nil, err
	}
	fi, err := f.underlyingLstat(name)
	if err != nil {
		return nil, err
	}
	if f.encoder == nil {
		return basicFileInfo{fi, fi.Name()}, err
	}
	return basicFileInfo{fi, decode(fi.Name())}, err
}

func (f *BasicFilesystem) RemoveAll(name string) error {
	name, err := f.rooted(name, "removeall")
	if err != nil {
		return err
	}
	return os.RemoveAll(name)
}

func (f *BasicFilesystem) Rename(oldpath, newpath string) error {
	oldpath2, err := f.rooted(oldpath, "rename")
	if err != nil {
		var pathError *os.PathError
		if errors.As(err, &pathError) {
			return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: syscall.ENOENT}
		}
		return err
	}
	newpath2, err := f.rooted(newpath, "rename")
	if err != nil {
		var pathError *os.PathError
		if errors.As(err, &pathError) {
			return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: syscall.ENOENT}
		}
		return err
	}
	return os.Rename(oldpath2, newpath2)
}

func (f *BasicFilesystem) Stat(name string) (FileInfo, error) {
	name, err := f.rooted(name, "stat")
	if err != nil {
		return nil, err
	}
	fi, err := os.Stat(name)
	if err != nil {
		return nil, err
	}
	if f.encoder == nil {
		return basicFileInfo{fi, fi.Name()}, err
	}
	return basicFileInfo{fi, decode(fi.Name())}, err
}

func (f *BasicFilesystem) DirNames(name string) ([]string, error) {
	name, err := f.rooted(name, "readdir")
	if err != nil {
		return nil, err
	}
	fd, err := os.OpenFile(name, OptReadOnly, 0o777)
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	names, err := fd.Readdirnames(-1)
	if err != nil {
		return nil, err
	}
	if f.encoder != nil {
		// If we find an encoded filename on disk, but it was encoded by a
		// encoder other than the current encoder, then we ignore it.
		// These files must be ignored or the same file would be created
		// with different encoded names.
		//
		// The standard encoder doesn't encode any filenames, so any encoded
		// filenames found on disk are ignored, as they are probably due to
		// the user selecting a different encoder, such as fat, and then
		// having that encoder save files with encoded names. Then, the user
		// changes the encoder back to standard. If the user wants Syncthing
		// to "see" these encoded filenames, they need to select an encoder,
		// such as the fat encoder, that will decode these filenames, or
		// configure all connected devices to use the passthrough encoder.
		names = f.encoder.filter(names)
	}
	return names, nil
}

func (f *BasicFilesystem) Open(name string) (File, error) {
	rootedName, err := f.rooted(name, "open")
	if err != nil {
		return nil, err
	}
	fd, err := os.Open(rootedName)
	if err != nil {
		return nil, err
	}
	if f.encoder == nil {
		return basicFile{fd, name}, err
	}
	return basicFile{fd, decode(name)}, err
}

func (f *BasicFilesystem) OpenFile(name string, flags int, mode FileMode) (File, error) {
	rootedName, err := f.rooted(name, "open")
	if err != nil {
		return nil, err
	}
	fd, err := os.OpenFile(rootedName, flags, os.FileMode(mode))
	if err != nil {
		return nil, err
	}
	if f.encoder == nil {
		return basicFile{fd, name}, err
	}
	return basicFile{fd, decode(name)}, err
}

func (f *BasicFilesystem) Create(name string) (File, error) {
	rootedName, err := f.rooted(name, "open")
	if err != nil {
		return nil, err
	}
	fd, err := os.Create(rootedName)
	if err != nil {
		return nil, err
	}
	if f.encoder == nil {
		return basicFile{fd, name}, err
	}
	return basicFile{fd, decode(name)}, err
}

func (*BasicFilesystem) Walk(_ string, _ WalkFunc) error {
	// implemented in WalkFilesystem
	return errors.New("not implemented")
}

func (f *BasicFilesystem) Glob(pattern string) ([]string, error) {
	pattern, err := f.rooted(pattern, "readdir")
	if err != nil {
		return nil, err
	}
	// If * and ? where encoded, unencode them
	pattern = decodePattern(pattern)
	files, err := filepath.Glob(pattern)
	// See DirNames' comments above
	unrooted := make([]string, len(files))
	for i, file := range files {
		unrooted[i] = rel(file, f.root)
	}
	if f.encoder != nil {
		unrooted = f.encoder.filter(unrooted)
	}
	return unrooted, err
}

func (f *BasicFilesystem) Usage(name string) (Usage, error) {
	name, err := f.rooted(name, "usage")
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

func (*BasicFilesystem) Type() FilesystemType {
	return FilesystemTypeBasic
}

func (f *BasicFilesystem) URI() string {
	return strings.TrimPrefix(f.root, `\\?\`)
}

func (f *BasicFilesystem) Options() []Option {
	return f.options
}

func (*BasicFilesystem) SameFile(fi1, fi2 FileInfo) bool {
	// Like os.SameFile, we always return false unless fi1 and fi2 were created
	// by this package's Stat/Lstat method.
	f1, ok1 := fi1.(basicFileInfo)
	f2, ok2 := fi2.(basicFileInfo)
	if !ok1 || !ok2 {
		return false
	}

	return os.SameFile(f1.osFileInfo(), f2.osFileInfo())
}

// ValidPath returns an error if the path is not valid in the underlying
// filesystem/operating system. The path is a non-rooted, relative path.
//
// ValidPath() is only called in lib/model/folder_sendrecv.go.
func (f *BasicFilesystem) ValidPath(path string) error {
	// Unless the passthrough encoder is selected, encoded filenames should
	// always be decoded to their original name before being transmitted.
	// So sending or receiving an encoded filename is an error, and they
	// are rejected.
	if f.encoder != nil && isEncoded(path) {
		return errInvalidFilenameSyncthingReservedChar
	}
	// The WindowsInvalidFilename() function only applies in Windows, as
	// NTFS/exFAT/FAT32 partitions mounted on other OSs allow reserved
	// filenames, such as CON, and files that end in periods or spaces.
	if !build.IsWindows {
		return nil
	}

	if f.encoder != nil && f.encoder.wouldEncode(path) {
		// it's a valid file, as the current encoder will encode it
		return nil
	}

	// The WindowsInvalidFilename() function is only called by this
	// ValidPath() function, so we could simplify this logic by merging the
	// two functions together.
	err := WindowsInvalidFilename(path)
	if f.encoder == nil {
		return err
	}
	if err == errInvalidFilenameWindowsSpacePeriod || err == errInvalidFilenameWindowsReservedName {
		if f.encoder.AllowReservedFilenames() {
			return nil
		}
	}
	return err
}

func (f *BasicFilesystem) underlying() (Filesystem, bool) {
	return nil, false
}

func (*BasicFilesystem) wrapperType() filesystemWrapperType {
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
	// save the decoded name
	name := filepath.Base(f.name)
	info, err := f.File.Stat()
	if err != nil {
		return nil, err
	}
	if name == "" {
		name = info.Name()
	}
	return basicFileInfo{info, name}, nil
}

// basicFileInfo implements the fs.FileInfo interface on top of an os.FileInfo.
type basicFileInfo struct {
	os.FileInfo
	name string // the original non-encoded filename
}

func (e basicFileInfo) Name() string {
	return e.name
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

// isEncoding() returns true if the current encoder encodes filenames.
func (f *BasicFilesystem) isEncoding() bool {
	return f.encoder != nil && f.encoderType != FilesystemEncoderTypeStandard
}

// ignore() returns true if the file is encoded and should be ignored by
// the current encoder.
func (f *BasicFilesystem) ignore(name string) bool {
	if f.encoder != nil && f.encoderType == FilesystemEncoderTypeStandard && isEncoded(name) {
		if l.ShouldDebug("encoder") {
			l.Debugf("Ignoring %q by %v encoder", name, f.encoderType)
		}
		return true
	}
	return false
}
