// Copyright (C) 2014 The Protocol Authors.

//go:generate go run ../../script/protofmt.go bep.proto
//go:generate protoc -I ../../vendor/ -I ../../vendor/github.com/gogo/protobuf/protobuf -I . --gogofast_out=. bep.proto

package protocol

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/syncthing/syncthing/lib/rand"
	"github.com/syncthing/syncthing/lib/sha256"
)

const (
	SyntheticDirectorySize = 128
)

var (
	sha256OfEmptyBlock = sha256.Sum256(make([]byte, BlockSize))
	HelloMessageMagic  = uint32(0x2EA7D90B)
)

func (m Hello) Magic() uint32 {
	return HelloMessageMagic
}

func (f FileInfo) String() string {
	switch f.Type {
	case FileInfoTypeDirectory:
		return fmt.Sprintf("Directory{Name:%q, Sequence:%d, Permissions:0%o, ModTime:%v, Version:%v, Deleted:%v, Invalid:%v, NoPermissions:%v}",
			f.Name, f.Sequence, f.Permissions, f.ModTime(), f.Version, f.Deleted, f.Invalid, f.NoPermissions)
	case FileInfoTypeFile:
		return fmt.Sprintf("File{Name:%q, Sequence:%d, Permissions:0%o, ModTime:%v, Version:%v, Length:%d, Deleted:%v, Invalid:%v, NoPermissions:%v, Blocks:%v}",
			f.Name, f.Sequence, f.Permissions, f.ModTime(), f.Version, f.Size, f.Deleted, f.Invalid, f.NoPermissions, f.Blocks)
	case FileInfoTypeSymlink, FileInfoTypeDeprecatedSymlinkDirectory, FileInfoTypeDeprecatedSymlinkFile:
		return fmt.Sprintf("Symlink{Name:%q, Type:%v, Sequence:%d, Version:%v, Deleted:%v, Invalid:%v, NoPermissions:%v, SymlinkTarget:%q}",
			f.Name, f.Type, f.Sequence, f.Version, f.Deleted, f.Invalid, f.NoPermissions, f.SymlinkTarget)
	default:
		panic("mystery file type detected")
	}
}

func (f FileInfo) IsDeleted() bool {
	return f.Deleted
}

func (f FileInfo) IsInvalid() bool {
	return f.Invalid
}

func (f FileInfo) IsDirectory() bool {
	return f.Type == FileInfoTypeDirectory
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

func (f FileInfo) FileName() string {
	return f.Name
}

func (f FileInfo) ModTime() time.Time {
	return time.Unix(f.ModifiedS, int64(f.ModifiedNs))
}

func (f FileInfo) SequenceNo() int64 {
	return f.Sequence
}

// WinsConflict returns true if "f" is the one to choose when it is in
// conflict with "other".
func (f FileInfo) WinsConflict(other FileInfo) bool {
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

func (f *FileInfo) Invalidate(invalidatedBy ShortID) {
	f.Invalid = true
	f.ModifiedBy = invalidatedBy
	f.Blocks = nil
	f.Sequence = 0
}

func (b BlockInfo) String() string {
	return fmt.Sprintf("Block{%d/%d/%d/%x}", b.Offset, b.Size, b.WeakHash, b.Hash)
}

// IsEmpty returns true if the block is a full block of zeroes.
func (b BlockInfo) IsEmpty() bool {
	return b.Size == BlockSize && bytes.Equal(b.Hash, sha256OfEmptyBlock[:])
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
