// Copyright (C) 2014 The Protocol Authors.

//go:generate go run ../../script/protofmt.go bep.proto
//go:generate protoc -I ../../vendor/ -I ../../vendor/github.com/gogo/protobuf/protobuf -I . --gogofast_out=. bep.proto

package protocol

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"runtime"
	"time"

	"github.com/syncthing/syncthing/lib/rand"
)

const (
	SyntheticDirectorySize = 128
	HelloMessageMagic      = uint32(0x2EA7D90B)
)

func (m Hello) Magic() uint32 {
	return HelloMessageMagic
}

func (f FileInfo) String() string {
	switch f.Type {
	case FileInfoTypeDirectory:
		return fmt.Sprintf("Directory{Name:%q, Sequence:%d, Permissions:0%o, ModTime:%v, Version:%v, Deleted:%v, Invalid:%v, LocalFlags:0x%x, NoPermissions:%v}",
			f.Name, f.Sequence, f.Permissions, f.ModTime(), f.Version, f.Deleted, f.RawInvalid, f.LocalFlags, f.NoPermissions)
	case FileInfoTypeFile:
		return fmt.Sprintf("File{Name:%q, Sequence:%d, Permissions:0%o, ModTime:%v, Version:%v, Length:%d, Deleted:%v, Invalid:%v, LocalFlags:0x%x, NoPermissions:%v, BlockSize:%d, Blocks:%v}",
			f.Name, f.Sequence, f.Permissions, f.ModTime(), f.Version, f.Size, f.Deleted, f.RawInvalid, f.LocalFlags, f.NoPermissions, f.RawBlockSize, f.Blocks)
	case FileInfoTypeSymlink, FileInfoTypeDeprecatedSymlinkDirectory, FileInfoTypeDeprecatedSymlinkFile:
		return fmt.Sprintf("Symlink{Name:%q, Type:%v, Sequence:%d, Version:%v, Deleted:%v, Invalid:%v, LocalFlags:0x%x, NoPermissions:%v, SymlinkTarget:%q}",
			f.Name, f.Type, f.Sequence, f.Version, f.Deleted, f.RawInvalid, f.LocalFlags, f.NoPermissions, f.SymlinkTarget)
	default:
		panic("mystery file type detected")
	}
}

func (f FileInfo) IsDeleted() bool {
	return f.Deleted
}

func (f FileInfo) IsInvalid() bool {
	return f.RawInvalid || f.LocalFlags&LocalInvalidFlags != 0
}

func (f FileInfo) IsIgnored() bool {
	return f.LocalFlags&FlagLocalIgnored != 0
}

func (f FileInfo) MustRescan() bool {
	return f.LocalFlags&FlagLocalMustRescan != 0
}

func (f FileInfo) IsDirectory() bool {
	return f.Type == FileInfoTypeDirectory
}

func (f FileInfo) ShouldConflict() bool {
	return f.LocalFlags&LocalConflictFlags != 0
}

func (f FileInfo) IsSymlink() bool {
	switch f.Type {
	case FileInfoTypeSymlink, FileInfoTypeDeprecatedSymlinkDirectory, FileInfoTypeDeprecatedSymlinkFile:
		return true
	default:
		return false
	}
}

func (f FileInfo) HasPermissionBits() bool {
	return !f.NoPermissions
}

func (f FileInfo) FileSize() int64 {
	if f.Deleted {
		return 0
	}
	if f.IsDirectory() || f.IsSymlink() {
		return SyntheticDirectorySize
	}
	return f.Size
}

func (f FileInfo) BlockSize() int {
	if f.RawBlockSize == 0 {
		return MinBlockSize
	}
	return int(f.RawBlockSize)
}

func (f FileInfo) FileName() string {
	return f.Name
}

func (f FileInfo) ModTime() time.Time {
	return time.Unix(f.ModifiedS, int64(f.ModifiedNs))
}

func (f FileInfo) SequenceNo() int64 {
	return f.Sequence
}

func (f FileInfo) FileVersion() Vector {
	return f.Version
}

// WinsConflict returns true if "f" is the one to choose when it is in
// conflict with "other".
func (f FileInfo) WinsConflict(other FileInfo) bool {
	// If only one of the files is invalid, that one loses
	if f.IsInvalid() != other.IsInvalid() {
		return !f.IsInvalid()
	}

	// If a modification is in conflict with a delete, we pick the
	// modification.
	if !f.IsDeleted() && other.IsDeleted() {
		return true
	}
	if f.IsDeleted() && !other.IsDeleted() {
		return false
	}

	// The one with the newer modification time wins.
	if f.ModTime().After(other.ModTime()) {
		return true
	}
	if f.ModTime().Before(other.ModTime()) {
		return false
	}

	// The modification times were equal. Use the device ID in the version
	// vector as tie breaker.
	return f.Version.Compare(other.Version) == ConcurrentGreater
}

func (f FileInfo) IsEmpty() bool {
	return f.Version.Counters == nil
}

// IsEquivalent checks that the two file infos represent the same actual file content,
// i.e. it does purposely not check only selected (see below) struct members.
// Permissions (config) and blocks (scanning) can be excluded from the comparison.
// Any file info is not "equivalent", if it has different
//  - type
//  - deleted flag
//  - invalid flag
//  - permissions, unless they are ignored
// A file is not "equivalent", if it has different
//  - modification time
//  - size
//  - blocks, unless there are no blocks to compare (scanning)
// A symlink is not "equivalent", if it has different
//  - target
// A directory does not have anything specific to check.
func (f FileInfo) IsEquivalent(other FileInfo, ignorePerms bool, ignoreBlocks bool) bool {
	if f.MustRescan() || other.MustRescan() {
		// These are per definition not equivalent because they don't
		// represent a valid state, even if both happen to have the
		// MustRescan bit set.
		return false
	}

	if f.Name != other.Name || f.Type != other.Type || f.Deleted != other.Deleted || f.IsInvalid() != other.IsInvalid() {
		return false
	}

	if !ignorePerms && !f.NoPermissions && !other.NoPermissions && !PermsEqual(f.Permissions, other.Permissions) {
		return false
	}

	switch f.Type {
	case FileInfoTypeFile:
		return f.Size == other.Size && f.ModTime().Equal(other.ModTime()) && (ignoreBlocks || BlocksEqual(f.Blocks, other.Blocks))
	case FileInfoTypeSymlink:
		return f.SymlinkTarget == other.SymlinkTarget
	case FileInfoTypeDirectory:
		return true
	}

	return false
}

func PermsEqual(a, b uint32) bool {
	switch runtime.GOOS {
	case "windows":
		// There is only writeable and read only, represented for user, group
		// and other equally. We only compare against user.
		return a&0600 == b&0600
	default:
		// All bits count
		return a&0777 == b&0777
	}
}

// BlocksEqual returns whether two slices of blocks are exactly the same hash
// and index pair wise.
func BlocksEqual(a, b []BlockInfo) bool {
	if len(b) != len(a) {
		return false
	}

	for i, sblk := range a {
		if !bytes.Equal(sblk.Hash, b[i].Hash) {
			return false
		}
	}

	return true
}

func (f *FileInfo) SetMustRescan(by ShortID) {
	f.LocalFlags = FlagLocalMustRescan
	f.ModifiedBy = by
	f.Blocks = nil
	f.Sequence = 0
}

func (f *FileInfo) SetIgnored(by ShortID) {
	f.LocalFlags = FlagLocalIgnored
	f.ModifiedBy = by
	f.Blocks = nil
	f.Sequence = 0
}

func (f *FileInfo) SetUnsupported(by ShortID) {
	f.LocalFlags = FlagLocalUnsupported
	f.ModifiedBy = by
	f.Blocks = nil
	f.Sequence = 0
}

func (b BlockInfo) String() string {
	return fmt.Sprintf("Block{%d/%d/%d/%x}", b.Offset, b.Size, b.WeakHash, b.Hash)
}

// IsEmpty returns true if the block is a full block of zeroes.
func (b BlockInfo) IsEmpty() bool {
	if v, ok := sha256OfEmptyBlock[int(b.Size)]; ok {
		return bytes.Equal(b.Hash, v[:])
	}
	return false
}

type IndexID uint64

func (i IndexID) String() string {
	return fmt.Sprintf("0x%16X", uint64(i))
}

func (i IndexID) Marshal() ([]byte, error) {
	bs := make([]byte, 8)
	binary.BigEndian.PutUint64(bs, uint64(i))
	return bs, nil
}

func (i *IndexID) Unmarshal(bs []byte) error {
	if len(bs) != 8 {
		return errors.New("incorrect IndexID length")
	}
	*i = IndexID(binary.BigEndian.Uint64(bs))
	return nil
}

func NewIndexID() IndexID {
	return IndexID(rand.Int64())
}

func (f Folder) Description() string {
	// used by logging stuff
	if f.Label == "" {
		return f.ID
	}
	return fmt.Sprintf("%q (%s)", f.Label, f.ID)
}
