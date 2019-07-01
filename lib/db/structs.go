// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:generate go run ../../script/protofmt.go structs.proto
//go:generate protoc -I ../../ -I . --gogofast_out=Mlib/protocol/bep.proto=github.com/syncthing/syncthing/lib/protocol:. structs.proto

package db

import (
	"bytes"
	"fmt"
	"sort"
	"time"

	"github.com/syncthing/syncthing/lib/protocol"
)

func (f FileInfoTruncated) String() string {
	switch f.Type {
	case protocol.FileInfoTypeDirectory:
		return fmt.Sprintf("Directory{Name:%q, Sequence:%d, Permissions:0%o, ModTime:%v, Version:%v, Deleted:%v, Invalid:%v, LocalFlags:0x%x, NoPermissions:%v}",
			f.Name, f.Sequence, f.Permissions, f.ModTime(), f.Version, f.Deleted, f.RawInvalid, f.LocalFlags, f.NoPermissions)
	case protocol.FileInfoTypeFile:
		return fmt.Sprintf("File{Name:%q, Sequence:%d, Permissions:0%o, ModTime:%v, Version:%v, Length:%d, Deleted:%v, Invalid:%v, LocalFlags:0x%x, NoPermissions:%v, BlockSize:%d}",
			f.Name, f.Sequence, f.Permissions, f.ModTime(), f.Version, f.Size, f.Deleted, f.RawInvalid, f.LocalFlags, f.NoPermissions, f.RawBlockSize)
	case protocol.FileInfoTypeSymlink, protocol.FileInfoTypeDeprecatedSymlinkDirectory, protocol.FileInfoTypeDeprecatedSymlinkFile:
		return fmt.Sprintf("Symlink{Name:%q, Type:%v, Sequence:%d, Version:%v, Deleted:%v, Invalid:%v, LocalFlags:0x%x, NoPermissions:%v, SymlinkTarget:%q}",
			f.Name, f.Type, f.Sequence, f.Version, f.Deleted, f.RawInvalid, f.LocalFlags, f.NoPermissions, f.SymlinkTarget)
	default:
		panic("mystery file type detected")
	}
}

func (f FileInfoTruncated) IsDeleted() bool {
	return f.Deleted
}

func (f FileInfoTruncated) IsInvalid() bool {
	return f.RawInvalid || f.LocalFlags&protocol.LocalInvalidFlags != 0
}

func (f FileInfoTruncated) IsUnsupported() bool {
	return f.LocalFlags&protocol.FlagLocalUnsupported != 0
}

func (f FileInfoTruncated) IsIgnored() bool {
	return f.LocalFlags&protocol.FlagLocalIgnored != 0
}

func (f FileInfoTruncated) MustRescan() bool {
	return f.LocalFlags&protocol.FlagLocalMustRescan != 0
}

func (f FileInfoTruncated) IsReceiveOnlyChanged() bool {
	return f.LocalFlags&protocol.FlagLocalReceiveOnly != 0
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

func (f FileInfoTruncated) ShouldConflict() bool {
	return f.LocalFlags&protocol.LocalConflictFlags != 0
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

func (f FileInfoTruncated) BlockSize() int {
	if f.RawBlockSize == 0 {
		return protocol.MinBlockSize
	}
	return int(f.RawBlockSize)
}

func (f FileInfoTruncated) FileName() string {
	return f.Name
}

func (f FileInfoTruncated) FileLocalFlags() uint32 {
	return f.LocalFlags
}

func (f FileInfoTruncated) ModTime() time.Time {
	return time.Unix(f.ModifiedS, int64(f.ModifiedNs))
}

func (f FileInfoTruncated) SequenceNo() int64 {
	return f.Sequence
}

func (f FileInfoTruncated) FileVersion() protocol.Vector {
	return f.Version
}

func (f FileInfoTruncated) ConvertToIgnoredFileInfo(by protocol.ShortID) protocol.FileInfo {
	return protocol.FileInfo{
		Name:         f.Name,
		Type:         f.Type,
		ModifiedS:    f.ModifiedS,
		ModifiedNs:   f.ModifiedNs,
		ModifiedBy:   by,
		Version:      f.Version,
		RawBlockSize: f.RawBlockSize,
		LocalFlags:   protocol.FlagLocalIgnored,
	}
}

func (c Counts) Add(other Counts) Counts {
	return Counts{
		Files:       c.Files + other.Files,
		Directories: c.Directories + other.Directories,
		Symlinks:    c.Symlinks + other.Symlinks,
		Deleted:     c.Deleted + other.Deleted,
		Bytes:       c.Bytes + other.Bytes,
		Sequence:    c.Sequence + other.Sequence,
		DeviceID:    protocol.EmptyDeviceID[:],
		LocalFlags:  c.LocalFlags | other.LocalFlags,
	}
}

func (c Counts) TotalItems() int32 {
	return c.Files + c.Directories + c.Symlinks + c.Deleted
}

func (vl VersionList) String() string {
	var b bytes.Buffer
	var id protocol.DeviceID
	b.WriteString("{")
	for i, v := range vl.Versions {
		if i > 0 {
			b.WriteString(", ")
		}
		copy(id[:], v.Device)
		fmt.Fprintf(&b, "{%v, %v}", v.Version, id)
	}
	b.WriteString("}")
	return b.String()
}

// update brings the VersionList up to date with file. It returns the updated
// VersionList, a potentially removed old FileVersion and its index, as well as
// the index where the new FileVersion was inserted.
func (vl VersionList) update(folder, device []byte, file protocol.FileInfo, t readOnlyTransaction) (_ VersionList, removedFV FileVersion, removedAt int, insertedAt int) {
	vl, removedFV, removedAt = vl.pop(device)

	nv := FileVersion{
		Device:  device,
		Version: file.Version,
		Invalid: file.IsInvalid(),
	}
	i := 0
	if nv.Invalid {
		i = sort.Search(len(vl.Versions), func(j int) bool {
			return vl.Versions[j].Invalid
		})
	}
	for ; i < len(vl.Versions); i++ {
		switch vl.Versions[i].Version.Compare(file.Version) {
		case protocol.Equal:
			fallthrough

		case protocol.Lesser:
			// The version at this point in the list is equal to or lesser
			// ("older") than us. We insert ourselves in front of it.
			vl = vl.insertAt(i, nv)
			return vl, removedFV, removedAt, i

		case protocol.ConcurrentLesser, protocol.ConcurrentGreater:
			// The version at this point is in conflict with us. We must pull
			// the actual file metadata to determine who wins. If we win, we
			// insert ourselves in front of the loser here. (The "Lesser" and
			// "Greater" in the condition above is just based on the device
			// IDs in the version vector, which is not the only thing we use
			// to determine the winner.)
			//
			// A surprise missing file entry here is counted as a win for us.
			if of, ok := t.getFile(folder, vl.Versions[i].Device, []byte(file.Name)); !ok || file.WinsConflict(of) {
				vl = vl.insertAt(i, nv)
				return vl, removedFV, removedAt, i
			}
		}
	}

	// We didn't find a position for an insert above, so append to the end.
	vl.Versions = append(vl.Versions, nv)

	return vl, removedFV, removedAt, len(vl.Versions) - 1
}

func (vl VersionList) insertAt(i int, v FileVersion) VersionList {
	vl.Versions = append(vl.Versions, FileVersion{})
	copy(vl.Versions[i+1:], vl.Versions[i:])
	vl.Versions[i] = v
	return vl
}

// pop returns the VersionList without the entry for the given device, as well
// as the removed FileVersion and the position, where that FileVersion was.
// If there is no FileVersion for the given device, the position is -1.
func (vl VersionList) pop(device []byte) (VersionList, FileVersion, int) {
	removedAt := -1
	for i, v := range vl.Versions {
		if bytes.Equal(v.Device, device) {
			vl.Versions = append(vl.Versions[:i], vl.Versions[i+1:]...)
			return vl, v, i
		}
	}
	return vl, FileVersion{}, removedAt
}

func (vl VersionList) Get(device []byte) (FileVersion, bool) {
	for _, v := range vl.Versions {
		if bytes.Equal(v.Device, device) {
			return v, true
		}
	}

	return FileVersion{}, false
}

type fileList []protocol.FileInfo

func (fl fileList) Len() int {
	return len(fl)
}

func (fl fileList) Swap(a, b int) {
	fl[a], fl[b] = fl[b], fl[a]
}

func (fl fileList) Less(a, b int) bool {
	return fl[a].Name < fl[b].Name
}
