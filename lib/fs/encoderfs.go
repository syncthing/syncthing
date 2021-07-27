// Copyright (C) 2021 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"
)

// An EncoderFilesystem ensures that paths that contain characters
// that are reserved in certain filesystems (such as <>:"|?* on NTFS, etc),
// or characters disallowed at the beginning, or end of a filename, can be
// safety stored. It does this by replacing the reserved characters with
// UNICODE characters in the private use area \uf000-\uf07f.
// This conversion is compatible with Cygwin, Git-Bash, Msys2,
// Windows Subsystem for Linux (WSL), and other platforms.
type EncoderFilesystem struct {
	Filesystem
	reservedChars      string
	reservedStartChars string
	reservedEndChars   string
	// reservedNames      []string
	patternMap       map[rune]rune
	reservedMap      map[rune]rune
	reservedStartMap map[rune]rune
	reservedEndMap   map[rune]rune
}

// See https://en.wikipedia.org/wiki/Private_Use_Areas#Vendor_use
const privateUseBase = 0xf000
const firstPrivateUseRune = rune(privateUseBase)

// We map 0xf000 to 0xf07f
const privateUseCharsToMap = 128

var privateUseRunes []rune
var privateUseChars string

var filesystemEncoderFunctions map[FilesystemEncoderType]func(fs Filesystem) Filesystem

func init() {
	privateUseRunes = make([]rune, privateUseCharsToMap)
	for i := 0; i < privateUseCharsToMap; i++ {
		privateUseRunes[i] = rune(int(privateUseBase) + i)
	}

	privateUseChars = string(privateUseRunes)

	filesystemEncoderFunctions = make(map[FilesystemEncoderType]func(fs Filesystem) Filesystem)
	filesystemEncoderFunctions[FilesystemEncoderTypeDefault] = nil // will use the BasicFilesystem
	filesystemEncoderFunctions[FilesystemEncoderTypeWindows] = NewWindowsEncoderFilesystem
	filesystemEncoderFunctions[FilesystemEncoderTypeAndroid] = NewAndroidEncoderFilesystem
	filesystemEncoderFunctions[FilesystemEncoderTypeIos] = NewIosEncoderFilesystem
	filesystemEncoderFunctions[FilesystemEncoderTypePlan9] = NewPlan9EncoderFilesystem
	filesystemEncoderFunctions[FilesystemEncoderTypeSafe] = NewSafeEncoderFilesystem
	// filesystemEncoderFunctions[FilesystemEncoderTypeCustom] = NewCustomEncodeFilesystem
}

func NewEncoderFilesystemFunction(t FilesystemEncoderType) func(fs Filesystem) Filesystem {
	encoderFunc, ok := filesystemEncoderFunctions[t]
	if ok {
		return encoderFunc
	}
	return nil
}

func (f *EncoderFilesystem) init() {
	f.patternMap = make(map[rune]rune)
	f.reservedMap = make(map[rune]rune)
	f.reservedStartMap = make(map[rune]rune)
	f.reservedEndMap = make(map[rune]rune)

	for _, r := range f.reservedChars {
		f.reservedMap[r] = rune(r | privateUseBase)
		if r != '*' && r != '?' {
			f.patternMap[r] = rune(r | privateUseBase)
		}
	}

	for _, r := range f.reservedStartChars {
		f.reservedStartMap[r] = rune(r | privateUseBase)
	}

	for _, r := range f.reservedEndChars {
		f.reservedEndMap[r] = rune(r | privateUseBase)
	}
}

func (f *EncoderFilesystem) Chmod(name string, mode FileMode) error {
	return f.Filesystem.Chmod(f.encodedPath(name), mode)
}

func (f *EncoderFilesystem) Lchown(name string, uid, gid int) error {
	return f.Filesystem.Lchown(f.encodedPath(name), uid, gid)
}

func (f *EncoderFilesystem) Chtimes(name string, atime time.Time, mtime time.Time) error {
	return f.Filesystem.Chtimes(f.encodedPath(name), atime, mtime)
}

func (f *EncoderFilesystem) Mkdir(name string, perm FileMode) error {
	return f.Filesystem.Mkdir(f.encodedPath(name), perm)
}

func (f *EncoderFilesystem) MkdirAll(path string, perm FileMode) error {
	return f.Filesystem.MkdirAll(f.encodedPath(path), perm)
}

type decodedFileInfo struct {
	FileInfo
	decodedName string
}

func (fi decodedFileInfo) Name() string {
	// Return the "normal" (non-encoded) ASCII filename to the caller.
	return fi.decodedName
}

func (f *EncoderFilesystem) Lstat(name string) (FileInfo, error) {
	info, err := f.Filesystem.Lstat(f.encodedPath(name))
	if err != nil {
		return nil, err
	}
	decodedInfo := decodedFileInfo{
		FileInfo:    info,
		decodedName: decodedPath(info.Name()),
	}
	return decodedInfo, nil
}

func (f *EncoderFilesystem) Remove(name string) error {
	return f.Filesystem.Remove(f.encodedPath(name))
}

func (f *EncoderFilesystem) RemoveAll(name string) error {
	return f.Filesystem.RemoveAll(f.encodedPath(name))
}

func (f *EncoderFilesystem) Rename(oldpath, newpath string) error {
	return f.Filesystem.Rename(f.encodedPath(oldpath), f.encodedPath(newpath))
}

func (f *EncoderFilesystem) Stat(name string) (FileInfo, error) {
	info, err := f.Filesystem.Stat(f.encodedPath(name))
	if err != nil {
		return nil, err
	}
	decodedInfo := decodedFileInfo{
		FileInfo:    info,
		decodedName: decodedPath(info.Name()),
	}
	return decodedInfo, nil
}

func (f *EncoderFilesystem) DirNames(name string) ([]string, error) {
	dirs, err := f.Filesystem.DirNames(f.encodedPath(name))
	if err != nil {
		return nil, err
	}
	for i, dir := range dirs {
		dirs[i] = decodedPath(dir)
	}
	return dirs, nil
}

func (f *EncoderFilesystem) Open(name string) (File, error) {
	return f.Filesystem.Open(f.encodedPath(name))
}

func (f *EncoderFilesystem) OpenFile(name string, flags int, mode FileMode) (File, error) {
	return f.Filesystem.OpenFile(f.encodedPath(name), flags, mode)
}

func (f *EncoderFilesystem) ReadSymlink(name string) (string, error) {
	return f.Filesystem.ReadSymlink(f.encodedPath(name))
}

func (f *EncoderFilesystem) Create(name string) (File, error) {
	return f.Filesystem.Create(f.encodedPath(name))
}

func (f *EncoderFilesystem) CreateSymlink(target, name string) error {
	return f.Filesystem.CreateSymlink(f.encodedPath(target), f.encodedPath(name))
}

func (f *EncoderFilesystem) Walk(root string, walkFn WalkFunc) error {
	// Walking the filesystem is likely (in Syncthing's fix certainly) done
	// to pick up external changes, for which caching is undesirable.
	decodingWalkFunc := func(path string, info FileInfo, err error) error {
		decodedInfo := decodedFileInfo{
			FileInfo:    info,
			decodedName: decodedPath(info.Name()),
		}
		return walkFn(decodedPath(path), decodedInfo, err)
	}

	return f.Filesystem.Walk(f.encodedPath(root), decodingWalkFunc)
}

func (f *EncoderFilesystem) Watch(path string, ignore Matcher, ctx context.Context, ignorePerms bool) (<-chan Event, <-chan error, error) {
	return f.Filesystem.Watch(f.encodedPath(path), ignore, ctx, ignorePerms)
}

func (f *EncoderFilesystem) Hide(name string) error {
	return f.Filesystem.Hide(f.encodedPath(name))
}

func (f *EncoderFilesystem) Unhide(name string) error {
	return f.Filesystem.Unhide(f.encodedPath(name))
}

func (f *EncoderFilesystem) underlying() (Filesystem, bool) {
	return f.Filesystem, true
}

func (f *EncoderFilesystem) wrapperType() filesystemWrapperType {
	return filesystemWrapperTypeEncoder
}

func (f *EncoderFilesystem) encodedPath(path string) string {
	return f._encodedPath(path)
}

func (f *EncoderFilesystem) _encodedPath(path string) string {
	encodedPath := ""

	var encodedPart string

	for i, part := range PathComponents(path) {
		if part == "" {
			if i > 0 {
				encodedPath += pathSeparatorString
			}
			continue
		}
		encodedPart = f.encodedName(part)

		if i == 0 {
			encodedPath += encodedPart
		} else {
			encodedPath += pathSeparatorString + encodedPart
		}
	}
	return encodedPath
}

func (f *EncoderFilesystem) needsEncoding(name string) bool {
	if strings.ContainsAny(name, f.reservedChars) {
		return true
	}
	lastChar, _ := utf8.DecodeLastRuneInString(name)
	if strings.ContainsRune(f.reservedEndChars, lastChar) {
		return true
	}
	firstChar, _ := utf8.DecodeRuneInString(name)
	return strings.ContainsRune(f.reservedStartChars, firstChar)
}

func (f *EncoderFilesystem) encodedName(name string) string {
	// "." and ".." are valid names
	if name == "." || name == ".." {
		return name
	}
	if !f.needsEncoding(name) {
		return name
	}
	runes := []rune(name)

	start := true
	for i, r := range runes {
		if start {
			if c, ok := f.reservedStartMap[r]; ok {
				runes[i] = c
				continue
			}
			start = false
		}
		if c, ok := f.reservedMap[r]; ok {
			runes[i] = c
		}
	}

	if len(f.reservedEndMap) > 0 {
		for i := len(runes) - 1; i >= 0; i-- {
			if c, ok := f.reservedEndMap[runes[i]]; ok {
				runes[i] = c
				continue
			}
			break
		}
	}
	return string(runes)
}

func decodedPath(path string) string {
	if !strings.ContainsAny(path, privateUseChars) {
		return path
	}
	runes := []rune(path)
	for i, r := range runes {
		if strings.ContainsRune(privateUseChars, r) {
			runes[i] &^= firstPrivateUseRune
		}
	}
	return string(runes)
}

/* 
// Not currently used:
func (f *EncoderFilesystem) encodedPattern(pattern string) string {
	runes := []rune(pattern)
	for i, r := range runes {
		if c, ok := f.patternMap[r]; ok {
			runes[i] = c
		}
	}
	return string(runes)
}

func isEncoded(path string) bool {
	return strings.ContainsAny(path, privateUseChars)
}
*/
