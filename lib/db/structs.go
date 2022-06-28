// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package db

import (
	"bytes"
	"fmt"
	"strings"
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
	case protocol.FileInfoTypeSymlink, protocol.FileInfoTypeSymlinkDirectory, protocol.FileInfoTypeSymlinkFile:
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
	case protocol.FileInfoTypeSymlink, protocol.FileInfoTypeSymlinkDirectory, protocol.FileInfoTypeSymlinkFile:
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

func (f FileInfoTruncated) ConvertToIgnoredFileInfo() protocol.FileInfo {
	file := f.copyToFileInfo()
	file.SetIgnored()
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

func (f FileInfoTruncated) LoadOSData(os protocol.OS, dst interface{ Unmarshal([]byte) error }) bool {
	bs, ok := f.OSData[os]
	if !ok {
		return false
	}
	return dst.Unmarshal(bs) == nil
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

func (c Counts) TotalItems() int {
	return c.Files + c.Directories + c.Symlinks + c.Deleted
}

func (c Counts) String() string {
	dev, _ := protocol.DeviceIDFromBytes(c.DeviceID)
	var flags strings.Builder
	if c.LocalFlags&needFlag != 0 {
		flags.WriteString("Need")
	}
	if c.LocalFlags&protocol.FlagLocalIgnored != 0 {
		flags.WriteString("Ignored")
	}
	if c.LocalFlags&protocol.FlagLocalMustRescan != 0 {
		flags.WriteString("Rescan")
	}
	if c.LocalFlags&protocol.FlagLocalReceiveOnly != 0 {
		flags.WriteString("Recvonly")
	}
	if c.LocalFlags&protocol.FlagLocalUnsupported != 0 {
		flags.WriteString("Unsupported")
	}
	if c.LocalFlags != 0 {
		flags.WriteString(fmt.Sprintf("(%x)", c.LocalFlags))
	}
	if flags.Len() == 0 {
		flags.WriteString("---")
	}
	return fmt.Sprintf("{Device:%v, Files:%d, Dirs:%d, Symlinks:%d, Del:%d, Bytes:%d, Seq:%d, Flags:%s}", dev, c.Files, c.Directories, c.Symlinks, c.Deleted, c.Bytes, c.Sequence, flags.String())
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
		fmt.Fprintf(&b, "{Version:%v, Deleted:%v, Devices:{", v.Version, v.Deleted)
		for j, dev := range v.Devices {
			if j > 0 {
				b.WriteString(", ")
			}
			copy(id[:], dev)
			fmt.Fprint(&b, id.Short())
		}
		b.WriteString("}, Invalid:{")
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
func (vl *VersionList) update(folder, device []byte, file protocol.FileIntf, t readOnlyTransaction) (FileVersion, FileVersion, FileVersion, bool, bool, bool, error) {
	if len(vl.RawVersions) == 0 {
		nv := newFileVersion(device, file.FileVersion(), file.IsInvalid(), file.IsDeleted())
		vl.RawVersions = append(vl.RawVersions, nv)
		return nv, FileVersion{}, FileVersion{}, false, false, true, nil
	}

	// Get the current global (before updating)
	oldFV, haveOldGlobal := vl.GetGlobal()
	oldFV = oldFV.copy()

	// Remove ourselves first
	removedFV, haveRemoved, _ := vl.pop(device)
	// Find position and insert the file
	err := vl.insert(folder, device, file, t)
	if err != nil {
		return FileVersion{}, FileVersion{}, FileVersion{}, false, false, false, err
	}

	newFV, _ := vl.GetGlobal() // We just inserted something above, can't be empty

	if !haveOldGlobal {
		return newFV, FileVersion{}, removedFV, false, haveRemoved, true, nil
	}

	globalChanged := true
	if oldFV.IsInvalid() == newFV.IsInvalid() && oldFV.Version.Equal(newFV.Version) {
		globalChanged = false
	}

	return newFV, oldFV, removedFV, true, haveRemoved, globalChanged, nil
}

func (vl *VersionList) insert(folder, device []byte, file protocol.FileIntf, t readOnlyTransaction) error {
	var added bool
	var err error
	i := 0
	for ; i < len(vl.RawVersions); i++ {
		// Insert our new version
		added, err = vl.checkInsertAt(i, folder, device, file, t)
		if err != nil {
			return err
		}
		if added {
			break
		}
	}
	if i == len(vl.RawVersions) {
		// Append to the end
		vl.RawVersions = append(vl.RawVersions, newFileVersion(device, file.FileVersion(), file.IsInvalid(), file.IsDeleted()))
	}
	return nil
}

func (vl *VersionList) insertAt(i int, v FileVersion) {
	vl.RawVersions = append(vl.RawVersions, FileVersion{})
	copy(vl.RawVersions[i+1:], vl.RawVersions[i:])
	vl.RawVersions[i] = v
}

// pop removes the given device from the VersionList and returns the FileVersion
// before removing the device, whether it was found/removed at all and whether
// the global changed in the process.
func (vl *VersionList) pop(device []byte) (FileVersion, bool, bool) {
	invDevice, i, j, ok := vl.findDevice(device)
	if !ok {
		return FileVersion{}, false, false
	}
	globalPos := vl.findGlobal()

	if vl.RawVersions[i].deviceCount() == 1 {
		fv := vl.RawVersions[i]
		vl.popVersionAt(i)
		return fv, true, globalPos == i
	}

	oldFV := vl.RawVersions[i].copy()
	if invDevice {
		vl.RawVersions[i].InvalidDevices = popDeviceAt(vl.RawVersions[i].InvalidDevices, j)
		return oldFV, true, false
	}
	vl.RawVersions[i].Devices = popDeviceAt(vl.RawVersions[i].Devices, j)
	// If the last valid device of the previous global was removed above,
	// the global changed.
	return oldFV, true, len(vl.RawVersions[i].Devices) == 0 && globalPos == i
}

// Get returns a FileVersion that contains the given device and whether it has
// been found at all.
func (vl *VersionList) Get(device []byte) (FileVersion, bool) {
	_, i, _, ok := vl.findDevice(device)
	if !ok {
		return FileVersion{}, false
	}
	return vl.RawVersions[i], true
}

// GetGlobal returns the current global FileVersion. The returned FileVersion
// may be invalid, if all FileVersions are invalid. Returns false only if
// VersionList is empty.
func (vl *VersionList) GetGlobal() (FileVersion, bool) {
	i := vl.findGlobal()
	if i == -1 {
		return FileVersion{}, false
	}
	return vl.RawVersions[i], true
}

func (vl *VersionList) Empty() bool {
	return len(vl.RawVersions) == 0
}

// findGlobal returns the first version that isn't invalid, or if all versions are
// invalid just the first version (i.e. 0) or -1, if there's no versions at all.
func (vl *VersionList) findGlobal() int {
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
func (vl *VersionList) findDevice(device []byte) (bool, int, int, bool) {
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

func (vl *VersionList) popVersionAt(i int) {
	vl.RawVersions = append(vl.RawVersions[:i], vl.RawVersions[i+1:]...)
}

// checkInsertAt determines if the given device and associated file should be
// inserted into the FileVersion at position i or into a new FileVersion at
// position i.
func (vl *VersionList) checkInsertAt(i int, folder, device []byte, file protocol.FileIntf, t readOnlyTransaction) (bool, error) {
	ordering := vl.RawVersions[i].Version.Compare(file.FileVersion())
	if ordering == protocol.Equal {
		if !file.IsInvalid() {
			vl.RawVersions[i].Devices = append(vl.RawVersions[i].Devices, device)
		} else {
			vl.RawVersions[i].InvalidDevices = append(vl.RawVersions[i].InvalidDevices, device)
		}
		return true, nil
	}
	existingDevice, _ := vl.RawVersions[i].FirstDevice()
	insert, err := shouldInsertBefore(ordering, folder, existingDevice, vl.RawVersions[i].IsInvalid(), file, t)
	if err != nil {
		return false, err
	}
	if insert {
		vl.insertAt(i, newFileVersion(device, file.FileVersion(), file.IsInvalid(), file.IsDeleted()))
		return true, nil
	}
	return false, nil
}

// shouldInsertBefore determines whether the file comes before an existing
// entry, given the version ordering (existing compared to new one), existing
// device and if the existing version is invalid.
func shouldInsertBefore(ordering protocol.Ordering, folder, existingDevice []byte, existingInvalid bool, file protocol.FileIntf, t readOnlyTransaction) (bool, error) {
	switch ordering {
	case protocol.Lesser:
		// The version at this point in the list is lesser
		// ("older") than us. We insert ourselves in front of it.
		return true, nil

	case protocol.ConcurrentLesser, protocol.ConcurrentGreater:
		// The version in conflict with us.
		// Check if we can shortcut due to one being invalid.
		if existingInvalid != file.IsInvalid() {
			return existingInvalid, nil
		}
		// We must pull the actual file metadata to determine who wins.
		// If we win, we insert ourselves in front of the loser here.
		// (The "Lesser" and "Greater" in the condition above is just
		// based on the device IDs in the version vector, which is not
		// the only thing we use to determine the winner.)
		of, ok, err := t.getFile(folder, existingDevice, []byte(file.FileName()))
		if err != nil {
			return false, err
		}
		// A surprise missing file entry here is counted as a win for us.
		if !ok {
			return true, nil
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

func (fv FileVersion) copy() FileVersion {
	n := fv
	n.Version = fv.Version.Copy()
	n.Devices = append([][]byte{}, fv.Devices...)
	n.InvalidDevices = append([][]byte{}, fv.InvalidDevices...)
	return n
}
