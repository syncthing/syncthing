// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:generate go run ../../script/protofmt.go structs.proto
//go:generate protoc -I ../../../../../ -I ../../vendor/ -I ../../vendor/github.com/gogo/protobuf/protobuf -I . --gogofast_out=. structs.proto

package db

import (
	"fmt"
	"time"

	"github.com/syncthing/syncthing/lib/protocol"
)

func (f FileInfoTruncated) String() string {
	return fmt.Sprintf("File{Name:%q, Permissions:0%o, Modified:%v, Version:%v, Length:%d, Deleted:%v, Invalid:%v, NoPermissions:%v}",
		f.Name, f.Permissions, f.ModTime(), f.Version, f.Size, f.Deleted, f.Invalid, f.NoPermissions)
}

func (f FileInfoTruncated) IsDeleted() bool {
	return f.Deleted
}

func (f FileInfoTruncated) IsInvalid() bool {
	return f.Invalid
}

func (f FileInfoTruncated) IsDirectory() bool {
	return f.Type == protocol.FileInfoTypeDirectory
}

func (f FileInfoTruncated) IsSymlink() bool {
	switch f.Type {
	case protocol.FileInfoTypeSymlink, protocol.FileInfoTypeDeprecatedSymlinkDirectory, protocol.FileInfoTypeDeprecatedSymlinkFile:
		return true
	default:
		return false
	}
}

func (f FileInfoTruncated) HasPermissionBits() bool {
	return !f.NoPermissions
}

func (f FileInfoTruncated) FileSize() int64 {
	if f.Deleted {
		return 0
	}
	if f.IsDirectory() || f.IsSymlink() {
		return protocol.SyntheticDirectorySize
	}
	return f.Size
}

func (f FileInfoTruncated) FileName() string {
	return f.Name
}

func (f FileInfoTruncated) ModTime() time.Time {
	return time.Unix(f.ModifiedS, int64(f.ModifiedNs))
}

func (f FileInfoTruncated) SequenceNo() int64 {
	return f.Sequence
}

func (f FileInfoTruncated) ConvertToInvalidFileInfo(invalidatedBy protocol.ShortID) protocol.FileInfo {
	return protocol.FileInfo{
		Name:       f.Name,
		Type:       f.Type,
		ModifiedS:  f.ModifiedS,
		ModifiedNs: f.ModifiedNs,
		ModifiedBy: invalidatedBy,
		Invalid:    true,
		Version:    f.Version,
	}
}
