// Copyright (C) 2026 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package protocol

func (f FileInfo) WireSize(withInternalFields bool) int {
	if f.truncated && !(f.IsDeleted() || f.IsInvalid() || f.IsIgnored()) {
		panic("bug: must not serialize truncated file info")
	}

	size := 0
	size += sizeStringField(1, f.Name)
	size += sizeEnumField(2, int(f.Type))
	size += sizeInt64Field(3, f.Size)
	size += sizeUint32Field(4, f.Permissions)
	size += sizeInt64Field(5, f.ModifiedS)
	size += sizeBoolField(6, f.Deleted)
	size += sizeBoolField(7, f.IsInvalid())
	size += sizeBoolField(8, f.NoPermissions)
	size += sizeMessageField(9, f.Version.wireSize())
	size += sizeInt64Field(10, f.Sequence)
	size += sizeInt32Field(11, int(f.ModifiedNs))
	size += sizeUint64Field(12, uint64(f.ModifiedBy))
	size += sizeInt32Field(13, int(f.RawBlockSize))
	size += sizeMessageField(14, f.Platform.wireSize())
	for _, block := range f.Blocks {
		size += sizeMessageField(16, block.wireSize())
	}
	size += sizeBytesField(17, f.SymlinkTarget)
	size += sizeBytesField(18, f.BlocksHash)
	size += sizeBytesField(19, f.Encrypted)
	size += sizeBytesField(20, f.PreviousBlocksHash)

	if withInternalFields {
		size += sizeUint32Field(1000, uint32(f.LocalFlags))
		size += sizeInt32Field(1003, f.EncryptionTrailerSize)
	}

	return size
}

func (v Vector) wireSize() int {
	size := 0
	for _, counter := range v.Counters {
		size += sizeMessageField(1, counter.wireSize())
	}
	return size
}

func (c Counter) wireSize() int {
	return sizeUint64Field(1, uint64(c.ID)) + sizeUint64Field(2, c.Value)
}

func (b BlockInfo) wireSize() int {
	return sizeInt64Field(1, b.Offset) +
		sizeInt32Field(2, b.Size) +
		sizeBytesField(3, b.Hash)
}

func (p PlatformData) wireSize() int {
	size := 0
	if p.Unix != nil {
		size += sizeMessageField(1, p.Unix.wireSize())
	}
	if p.Windows != nil {
		size += sizeMessageField(2, windowsDataWireSize(p.Windows))
	}
	if p.Linux != nil {
		size += sizeMessageField(3, p.Linux.wireSize())
	}
	if p.Darwin != nil {
		size += sizeMessageField(4, p.Darwin.wireSize())
	}
	if p.FreeBSD != nil {
		size += sizeMessageField(5, p.FreeBSD.wireSize())
	}
	if p.NetBSD != nil {
		size += sizeMessageField(6, p.NetBSD.wireSize())
	}
	return size
}

func (u *UnixData) wireSize() int {
	if u == nil {
		return 0
	}
	return sizeStringField(1, u.OwnerName) +
		sizeStringField(2, u.GroupName) +
		sizeInt32Field(3, u.UID) +
		sizeInt32Field(4, u.GID)
}

func windowsDataWireSize(w *WindowsData) int {
	if w == nil {
		return 0
	}
	return sizeStringField(1, w.OwnerName) +
		sizeBoolField(2, w.OwnerIsGroup)
}

func (x *XattrData) wireSize() int {
	if x == nil {
		return 0
	}
	size := 0
	for _, attr := range x.Xattrs {
		size += sizeMessageField(1, attr.wireSize())
	}
	return size
}

func (a Xattr) wireSize() int {
	return sizeStringField(1, a.Name) +
		sizeBytesField(2, a.Value)
}

func sizeMessageField(field int, messageSize int) int {
	return sizeTag(field) + sizeUvarint(messageSize) + messageSize
}

func sizeStringField(field int, value string) int {
	if value == "" {
		return 0
	}
	return sizeTag(field) + sizeUvarint(len(value)) + len(value)
}

func sizeBytesField(field int, value []byte) int {
	if len(value) == 0 {
		return 0
	}
	return sizeTag(field) + sizeUvarint(len(value)) + len(value)
}

func sizeBoolField(field int, value bool) int {
	if !value {
		return 0
	}
	return sizeTag(field) + 1
}

func sizeEnumField(field int, value int) int {
	return sizeInt32Field(field, value)
}

func sizeUint32Field(field int, value uint32) int {
	if value == 0 {
		return 0
	}
	return sizeTag(field) + sizeUvarint(int(value))
}

func sizeUint64Field(field int, value uint64) int {
	if value == 0 {
		return 0
	}
	return sizeTag(field) + sizeVarint(value)
}

func sizeInt32Field(field int, value int) int {
	if value == 0 {
		return 0
	}
	if value < 0 {
		return sizeTag(field) + 10
	}
	return sizeTag(field) + sizeUvarint(value)
}

func sizeInt64Field(field int, value int64) int {
	if value == 0 {
		return 0
	}
	if value < 0 {
		return sizeTag(field) + 10
	}
	return sizeTag(field) + sizeVarint(uint64(value))
}

func sizeTag(field int) int {
	return sizeUvarint(field << 3)
}

func sizeUvarint(value int) int {
	return sizeVarint(uint64(value)) //nolint:gosec // G115: value is a non-negative length or field index
}

func sizeVarint(value uint64) int {
	size := 1
	for value >= 0x80 {
		value >>= 7
		size++
	}
	return size
}
