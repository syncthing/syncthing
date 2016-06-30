// Copyright (C) 2014 The Protocol Authors.

//go:generate go run ../../script/protofmt.go bep.proto
//go:generate protoc --proto_path=../../../../../:../../../../gogo/protobuf/protobuf:. --gogofast_out=. bep.proto

package protocol

import (
	"bytes"
	"crypto/sha256"
	"fmt"
)

var (
	sha256OfEmptyBlock = sha256.Sum256(make([]byte, BlockSize))
	HelloMessageMagic  = uint32(0x2EA7D90B)
)

func (m Hello) Magic() uint32 {
	return HelloMessageMagic
}

func (f FileInfo) String() string {
	return fmt.Sprintf("File{Name:%q, Permissions:0%o, Modified:%d, Version:%v, Length:%d, Deleted:%v, Invalid:%v, NoPermissions:%v, Blocks:%v}",
		f.Name, f.Permissions, f.Modified, f.Version, f.Size, f.Deleted, f.Invalid, f.NoPermissions, f.Blocks)
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
	case FileInfoTypeSymlinkDirectory, FileInfoTypeSymlinkFile, FileInfoTypeSymlinkUnknown:
		return true
	default:
		return false
	}
}

func (f FileInfo) HasPermissionBits() bool {
	return !f.NoPermissions
}

func (f FileInfo) FileSize() int64 {
	if f.IsDirectory() || f.IsDeleted() {
		return 128
	}
	return f.Size
}

func (f FileInfo) FileName() string {
	return f.Name
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
	if f.Modified > other.Modified {
		return true
	}
	if f.Modified < other.Modified {
		return false
	}

	// The modification times were equal. Use the device ID in the version
	// vector as tie breaker.
	return f.Version.Compare(other.Version) == ConcurrentGreater
}

func (b BlockInfo) String() string {
	return fmt.Sprintf("Block{%d/%d/%x}", b.Offset, b.Size, b.Hash)
}

// IsEmpty returns true if the block is a full block of zeroes.
func (b BlockInfo) IsEmpty() bool {
	return b.Size == BlockSize && bytes.Equal(b.Hash, sha256OfEmptyBlock[:])
}
