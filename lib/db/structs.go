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

func (f FileInfoTruncated) FileType() protocol.FileInfoType {
	return f.Type
}

func (f FileInfoTruncated) FilePermissions() uint32 {
	return f.Permissions
}

func (f FileInfoTruncated) FileModifiedBy() protocol.ShortID {
	return f.ModifiedBy
}

func (f FileInfoTruncated) ConvertToIgnoredFileInfo(by protocol.ShortID) protocol.FileInfo {
	file := f.copyToFileInfo()
	file.SetIgnored(by)
	return file
}

func (f FileInfoTruncated) ConvertToDeletedFileInfo(by protocol.ShortID) protocol.FileInfo {
	file := f.copyToFileInfo()
	file.SetDeleted(by)
	return file
}

// ConvertDeletedToFileInfo converts a deleted truncated file info to a regular file info
func (f FileInfoTruncated) ConvertDeletedToFileInfo() protocol.FileInfo {
	if !f.Deleted {
		panic("ConvertDeletedToFileInfo must only be called on deleted items")
	}
	return f.copyToFileInfo()
}

// copyToFileInfo just copies all members of FileInfoTruncated to protocol.FileInfo
func (f FileInfoTruncated) copyToFileInfo() protocol.FileInfo {
	return protocol.FileInfo{
		Name:          f.Name,
		Size:          f.Size,
		ModifiedS:     f.ModifiedS,
		ModifiedBy:    f.ModifiedBy,
		Version:       f.Version,
		Sequence:      f.Sequence,
		SymlinkTarget: f.SymlinkTarget,
		BlocksHash:    f.BlocksHash,
		Type:          f.Type,
		Permissions:   f.Permissions,
		ModifiedNs:    f.ModifiedNs,
		RawBlockSize:  f.RawBlockSize,
		LocalFlags:    f.LocalFlags,
		Deleted:       f.Deleted,
		RawInvalid:    f.RawInvalid,
		NoPermissions: f.NoPermissions,
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

// Equal compares the numbers only, not sequence/dev/flags.
func (c Counts) Equal(o Counts) bool {
	return c.Files == o.Files && c.Directories == o.Directories && c.Symlinks == o.Symlinks && c.Deleted == o.Deleted && c.Bytes == o.Bytes
}

func (vl VersionList) String() string {
	var b bytes.Buffer
	var id protocol.DeviceID
	b.WriteString("{")
	for i, v := range vl.RawVersions {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "{%v, {", v.Version)
		for j, dev := range v.Devices {
			if j > 0 {
				b.WriteString(", ")
			}
			copy(id[:], dev)
			fmt.Fprint(&b, id.Short())
		}
		b.WriteString("}, {")
		for j, dev := range v.InvalidDevices {
			if j > 0 {
				b.WriteString(", ")
			}
			copy(id[:], dev)
			fmt.Fprint(&b, id.Short())
		}
		fmt.Fprint(&b, "}}")
	}
	b.WriteString("}")
	return b.String()
}

// update brings the VersionList up to date with file. It returns the updated
// VersionList, a device that has the global/newest version, a device that previously
// had the global/newest version, a boolean indicating if the global version has
// changed and if any error occurred (only possible in db interaction).
func (vl VersionList) update(folder, device []byte, file protocol.FileIntf, t readOnlyTransaction) (VersionList, FileVersion, FileVersion, FileVersion, bool, bool, bool, error) {
	if len(vl.RawVersions) == 0 {
		nv := newFileVersion(device, file.FileVersion(), file.IsInvalid(), file.IsDeleted())
		vl.RawVersions = append(vl.RawVersions, nv)
		return vl, nv, FileVersion{}, FileVersion{}, false, false, true, nil
	}

	// Get the current global (before updating)
	oldFV, haveOldGlobal := vl.GetGlobal()

	// Remove ourselves first
	vl, removedFV, haveRemoved, _, err := vl.pop(folder, device, []byte(file.FileName()), t)
	if err == nil {
		// Find position and insert the file
		vl, err = vl.insert(folder, device, file, t)
	}
	if err != nil {
		return vl, FileVersion{}, FileVersion{}, FileVersion{}, false, false, false, err
	}

	newFV, _ := vl.GetGlobal() // We just inserted something above, can't be empty

	if !haveOldGlobal {
		return vl, newFV, FileVersion{}, removedFV, false, haveRemoved, true, nil
	}

	globalChanged := true
	if oldFV.IsInvalid() == newFV.IsInvalid() && oldFV.Version.Equal(newFV.Version) {
		globalChanged = false
	}

	return vl, newFV, oldFV, removedFV, true, haveRemoved, globalChanged, nil
}

func (vl VersionList) insert(folder, device []byte, file protocol.FileIntf, t readOnlyTransaction) (VersionList, error) {
	var added bool
	var err error
	i := 0
	for ; i < len(vl.RawVersions); i++ {
		// Insert our new version
		vl, added, err = vl.checkInsertAt(i, folder, device, []byte(file.FileName()), file.FileVersion(), file.IsInvalid(), file.IsDeleted(), file, true, t)
		if err != nil {
			return vl, err
		}
		if added {
			break
		}
	}
	if i == len(vl.RawVersions) {
		// Append to the end
		vl.RawVersions = append(vl.RawVersions, newFileVersion(device, file.FileVersion(), file.IsInvalid(), file.IsDeleted()))
	}
	return vl, nil
}

func (vl VersionList) insertAt(i int, v FileVersion) VersionList {
	vl.RawVersions = append(vl.RawVersions, FileVersion{})
	copy(vl.RawVersions[i+1:], vl.RawVersions[i:])
	vl.RawVersions[i] = v
	return vl
}

// pop returns the VersionList without the entry for the given device, as well
// as the removed FileVersion, whether it was found/removed at all and whether
// the global changed in the process.
func (vl VersionList) pop(folder, device, name []byte, t readOnlyTransaction) (VersionList, FileVersion, bool, bool, error) {
	invDevice, i, j, ok := vl.findDevice(device)
	if !ok {
		return vl, FileVersion{}, false, false, nil
	}
	globalPos := vl.findGlobal()

	if vl.RawVersions[i].deviceCount() == 1 {
		fv := vl.RawVersions[i]
		vl.RawVersions = popVersionAt(vl.RawVersions, i)
		return vl, fv, true, globalPos == i, nil
	}

	if invDevice {
		vl.RawVersions[i].InvalidDevices = popDeviceAt(vl.RawVersions[i].InvalidDevices, j)
	} else {
		vl.RawVersions[i].Devices = popDeviceAt(vl.RawVersions[i].Devices, j)
	}
	// If the last valid device of the previous global was removed above,
	// the next entry is now the global entry (unless all entries are invalid).
	if len(vl.RawVersions[i].Devices) == 0 && globalPos == i {
		return vl, vl.RawVersions[i], true, globalPos == vl.findGlobal(), nil
	}
	return vl, vl.RawVersions[i], true, false, nil
}

// Get returns a FileVersion that contains the given device and whether it has
// been found at all.
func (vl VersionList) Get(device []byte) (FileVersion, bool) {
	_, i, _, ok := vl.findDevice(device)
	if !ok {
		return FileVersion{}, false
	}
	return vl.RawVersions[i], true
}

func (vl VersionList) GetGlobal() (FileVersion, bool) {
	i := vl.findGlobal()
	if i == -1 {
		return FileVersion{}, false
	}
	return vl.RawVersions[i], true
}

func (vl VersionList) Empty() bool {
	return len(vl.RawVersions) == 0
}

func (vl VersionList) findGlobal() int {
	for i, fv := range vl.RawVersions {
		if !fv.IsInvalid() {
			return i
		}
	}
	if len(vl.RawVersions) == 0 {
		return -1
	}
	return 0
}

// findDevices returns whether the device is in InvalidVersions or Versions and
// in InvalidDevices or Devices (true for invalid), the positions in the version
// and device slices and whether it has been found at all.
func (vl VersionList) findDevice(device []byte) (bool, int, int, bool) {
	for i, v := range vl.RawVersions {
		if j := deviceIndex(v.Devices, device); j != -1 {
			return false, i, j, true
		}
		if j := deviceIndex(v.InvalidDevices, device); j != -1 {
			return true, i, j, true
		}
	}
	return false, -1, -1, false
}

func (vl VersionList) checkInsertAt(i int, folder, device, name []byte, version protocol.Vector, invalid, deleted bool, file protocol.FileIntf, haveFile bool, t readOnlyTransaction) (VersionList, bool, error) {
	ordering := vl.RawVersions[i].Version.Compare(version)
	if ordering == protocol.Equal {
		if !invalid {
			vl.RawVersions[i].Devices = append(vl.RawVersions[i].Devices, device)
		} else {
			vl.RawVersions[i].InvalidDevices = append(vl.RawVersions[i].InvalidDevices, device)
		}
		return vl, true, nil
	}
	insert, err := shouldInsertBefore(ordering, folder, device, vl.RawVersions[i].Devices[0], name, version, file, haveFile, t)
	if err != nil {
		return vl, false, err
	}
	if insert {
		vl = vl.insertAt(i, newFileVersion(device, version, invalid, deleted))
		return vl, true, nil
	}
	return vl, false, nil
}

func shouldInsertBefore(ordering protocol.Ordering, folder, device, existingDevice, name []byte, version protocol.Vector, file protocol.FileIntf, haveFile bool, t readOnlyTransaction) (bool, error) {
	switch ordering {
	case protocol.Lesser:
		// The version at this point in the list is lesser
		// ("older") than us. We insert ourselves in front of it.
		return true, nil

	case protocol.ConcurrentLesser, protocol.ConcurrentGreater:
		// The version in conflict with us. We must pull
		// the actual file metadata to determine who wins. If we win, we
		// insert ourselves in front of the loser here. (The "Lesser" and
		// "Greater" in the condition above is just based on the device
		// IDs in the version vector, which is not the only thing we use
		// to determine the winner.)
		of, ok, err := t.getFile(folder, existingDevice, name)
		if err != nil {
			return false, err
		}
		// A surprise missing file entry here is counted as a win for us.
		if !ok {
			return true, nil
		}
		if !haveFile {
			file, ok, err = t.getFile(folder, device, name)
			if !ok {
				return false, errEntryFromGlobalMissing
			}
		}
		if err != nil {
			return false, err
		}
		if protocol.WinsConflict(file, of) {
			return true, nil
		}
	}
	return false, nil
}

func deviceIndex(devices [][]byte, device []byte) int {
	for i, dev := range devices {
		if bytes.Equal(device, dev) {
			return i
		}
	}
	return -1
}

func popDeviceAt(devices [][]byte, i int) [][]byte {
	return append(devices[:i], devices[i+1:]...)
}

func popDevice(devices [][]byte, device []byte) ([][]byte, bool) {
	i := deviceIndex(devices, device)
	if i == -1 {
		return devices, false
	}
	return popDeviceAt(devices, i), true
}

func versionIndex(versions []FileVersion, version protocol.Vector) int {
	for i, v := range versions {
		if version.Equal(v.Version) {
			return i
		}
	}
	return -1
}

func popVersionAt(versions []FileVersion, i int) []FileVersion {
	return append(versions[:i], versions[i+1:]...)
}

func popVersion(versions []FileVersion, version protocol.Vector) ([]FileVersion, FileVersion, bool) {
	i := versionIndex(versions, version)
	if i == -1 {
		return versions, FileVersion{}, false
	}
	fv := versions[i]
	return popVersionAt(versions, i), fv, true
}

func newFileVersion(device []byte, version protocol.Vector, invalid, deleted bool) FileVersion {
	fv := FileVersion{
		Version: version,
		Deleted: deleted,
	}
	if invalid {
		fv.InvalidDevices = [][]byte{device}
	} else {
		fv.Devices = [][]byte{device}
	}
	return fv
}

func (fv FileVersion) FirstDevice() ([]byte, bool) {
	if len(fv.Devices) != 0 {
		return fv.Devices[0], true
	}
	if len(fv.InvalidDevices) != 0 {
		return fv.InvalidDevices[0], true
	}
	return nil, false
}

func (fv FileVersion) IsInvalid() bool {
	return len(fv.Devices) == 0
}

func (fv FileVersion) deviceCount() int {
	return len(fv.Devices) + len(fv.InvalidDevices)
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
