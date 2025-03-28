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

	"google.golang.org/protobuf/proto"

	"github.com/syncthing/syncthing/internal/gen/dbproto"
	"github.com/syncthing/syncthing/lib/protocol"
)

type CountsSet struct {
	Counts  []Counts
	Created int64 // unix nanos
}

type Counts struct {
	Files       int
	Directories int
	Symlinks    int
	Deleted     int
	Bytes       int64
	Sequence    int64             // zero for the global state
	DeviceID    protocol.DeviceID // device ID for remote devices, or special values for local/global
	LocalFlags  uint32            // the local flag for this count bucket
}

func (c Counts) toWire() *dbproto.Counts {
	return &dbproto.Counts{
		Files:       int32(c.Files),
		Directories: int32(c.Directories),
		Symlinks:    int32(c.Symlinks),
		Deleted:     int32(c.Deleted),
		Bytes:       c.Bytes,
		Sequence:    c.Sequence,
		DeviceId:    c.DeviceID[:],
		LocalFlags:  c.LocalFlags,
	}
}

func countsFromWire(w *dbproto.Counts) Counts {
	return Counts{
		Files:       int(w.Files),
		Directories: int(w.Directories),
		Symlinks:    int(w.Symlinks),
		Deleted:     int(w.Deleted),
		Bytes:       w.Bytes,
		Sequence:    w.Sequence,
		DeviceID:    protocol.DeviceID(w.DeviceId),
		LocalFlags:  w.LocalFlags,
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
		DeviceID:    protocol.EmptyDeviceID,
		LocalFlags:  c.LocalFlags | other.LocalFlags,
	}
}

func (c Counts) TotalItems() int {
	return c.Files + c.Directories + c.Symlinks + c.Deleted
}

func (c Counts) String() string {
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
	return fmt.Sprintf("{Device:%v, Files:%d, Dirs:%d, Symlinks:%d, Del:%d, Bytes:%d, Seq:%d, Flags:%s}", c.DeviceID, c.Files, c.Directories, c.Symlinks, c.Deleted, c.Bytes, c.Sequence, flags.String())
}

// Equal compares the numbers only, not sequence/dev/flags.
func (c Counts) Equal(o Counts) bool {
	return c.Files == o.Files && c.Directories == o.Directories && c.Symlinks == o.Symlinks && c.Deleted == o.Deleted && c.Bytes == o.Bytes
}

// update brings the VersionList up to date with file. It returns the updated
// VersionList, a device that has the global/newest version, a device that previously
// had the global/newest version, a boolean indicating if the global version has
// changed and if any error occurred (only possible in db interaction).
func vlUpdate(vl *dbproto.VersionList, folder, device []byte, file protocol.FileInfo, t readOnlyTransaction) (*dbproto.FileVersion, *dbproto.FileVersion, *dbproto.FileVersion, bool, bool, bool, error) {
	if len(vl.Versions) == 0 {
		nv := newFileVersion(device, file.FileVersion(), file.IsInvalid(), file.IsDeleted())
		vl.Versions = append(vl.Versions, nv)
		return nv, nil, nil, false, false, true, nil
	}

	// Get the current global (before updating)
	oldFV, haveOldGlobal := vlGetGlobal(vl)
	oldFV = fvCopy(oldFV)

	// Remove ourselves first
	removedFV, haveRemoved, _ := vlPop(vl, device)
	// Find position and insert the file
	err := vlInsert(vl, folder, device, file, t)
	if err != nil {
		return nil, nil, nil, false, false, false, err
	}

	newFV, _ := vlGetGlobal(vl) // We just inserted something above, can't be empty

	if !haveOldGlobal {
		return newFV, nil, removedFV, false, haveRemoved, true, nil
	}

	globalChanged := true
	if fvIsInvalid(oldFV) == fvIsInvalid(newFV) && protocol.VectorFromWire(oldFV.Version).Equal(protocol.VectorFromWire(newFV.Version)) {
		globalChanged = false
	}

	return newFV, oldFV, removedFV, true, haveRemoved, globalChanged, nil
}

func vlInsert(vl *dbproto.VersionList, folder, device []byte, file protocol.FileInfo, t readOnlyTransaction) error {
	var added bool
	var err error
	i := 0
	for ; i < len(vl.Versions); i++ {
		// Insert our new version
		added, err = vlCheckInsertAt(vl, i, folder, device, file, t)
		if err != nil {
			return err
		}
		if added {
			break
		}
	}
	if i == len(vl.Versions) {
		// Append to the end
		vl.Versions = append(vl.Versions, newFileVersion(device, file.FileVersion(), file.IsInvalid(), file.IsDeleted()))
	}
	return nil
}

func vlInsertAt(vl *dbproto.VersionList, i int, v *dbproto.FileVersion) {
	vl.Versions = append(vl.Versions, &dbproto.FileVersion{})
	copy(vl.Versions[i+1:], vl.Versions[i:])
	vl.Versions[i] = v
}

// pop removes the given device from the VersionList and returns the FileVersion
// before removing the device, whether it was found/removed at all and whether
// the global changed in the process.
func vlPop(vl *dbproto.VersionList, device []byte) (*dbproto.FileVersion, bool, bool) {
	invDevice, i, j, ok := vlFindDevice(vl, device)
	if !ok {
		return nil, false, false
	}
	globalPos := vlFindGlobal(vl)

	fv := vl.Versions[i]
	if fvDeviceCount(fv) == 1 {
		vlPopVersionAt(vl, i)
		return fv, true, globalPos == i
	}

	oldFV := fvCopy(fv)
	if invDevice {
		vl.Versions[i].InvalidDevices = popDeviceAt(vl.Versions[i].InvalidDevices, j)
		return oldFV, true, false
	}
	vl.Versions[i].Devices = popDeviceAt(vl.Versions[i].Devices, j)
	// If the last valid device of the previous global was removed above,
	// the global changed.
	return oldFV, true, len(vl.Versions[i].Devices) == 0 && globalPos == i
}

// Get returns a FileVersion that contains the given device and whether it has
// been found at all.
func vlGet(vl *dbproto.VersionList, device []byte) (*dbproto.FileVersion, bool) {
	_, i, _, ok := vlFindDevice(vl, device)
	if !ok {
		return &dbproto.FileVersion{}, false
	}
	return vl.Versions[i], true
}

// GetGlobal returns the current global FileVersion. The returned FileVersion
// may be invalid, if all FileVersions are invalid. Returns false only if
// VersionList is empty.
func vlGetGlobal(vl *dbproto.VersionList) (*dbproto.FileVersion, bool) {
	i := vlFindGlobal(vl)
	if i == -1 {
		return nil, false
	}
	return vl.Versions[i], true
}

// findGlobal returns the first version that isn't invalid, or if all versions are
// invalid just the first version (i.e. 0) or -1, if there's no versions at all.
func vlFindGlobal(vl *dbproto.VersionList) int {
	for i := range vl.Versions {
		if !fvIsInvalid(vl.Versions[i]) {
			return i
		}
	}
	if len(vl.Versions) == 0 {
		return -1
	}
	return 0
}

// findDevice returns whether the device is in InvalidVersions or Versions and
// in InvalidDevices or Devices (true for invalid), the positions in the version
// and device slices and whether it has been found at all.
func vlFindDevice(vl *dbproto.VersionList, device []byte) (bool, int, int, bool) {
	for i, v := range vl.Versions {
		if j := deviceIndex(v.Devices, device); j != -1 {
			return false, i, j, true
		}
		if j := deviceIndex(v.InvalidDevices, device); j != -1 {
			return true, i, j, true
		}
	}
	return false, -1, -1, false
}

func vlPopVersionAt(vl *dbproto.VersionList, i int) {
	vl.Versions = append(vl.Versions[:i], vl.Versions[i+1:]...)
}

// checkInsertAt determines if the given device and associated file should be
// inserted into the FileVersion at position i or into a new FileVersion at
// position i.
func vlCheckInsertAt(vl *dbproto.VersionList, i int, folder, device []byte, file protocol.FileInfo, t readOnlyTransaction) (bool, error) {
	fv := vl.Versions[i]
	ordering := protocol.VectorFromWire(fv.Version).Compare(file.FileVersion())
	if ordering == protocol.Equal {
		if !file.IsInvalid() {
			fv.Devices = append(fv.Devices, device)
		} else {
			fv.InvalidDevices = append(fv.InvalidDevices, device)
		}
		return true, nil
	}
	existingDevice, _ := fvFirstDevice(fv)
	insert, err := shouldInsertBefore(ordering, folder, existingDevice, fvIsInvalid(fv), file, t)
	if err != nil {
		return false, err
	}
	if insert {
		vlInsertAt(vl, i, newFileVersion(device, file.FileVersion(), file.IsInvalid(), file.IsDeleted()))
		return true, nil
	}
	return false, nil
}

// shouldInsertBefore determines whether the file comes before an existing
// entry, given the version ordering (existing compared to new one), existing
// device and if the existing version is invalid.
func shouldInsertBefore(ordering protocol.Ordering, folder, existingDevice []byte, existingInvalid bool, file protocol.FileInfo, t readOnlyTransaction) (bool, error) {
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
		if file.WinsConflict(of) {
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

func newFileVersion(device []byte, version protocol.Vector, invalid, deleted bool) *dbproto.FileVersion {
	fv := &dbproto.FileVersion{
		Version: version.ToWire(),
		Deleted: deleted,
	}
	if invalid {
		fv.InvalidDevices = [][]byte{device}
	} else {
		fv.Devices = [][]byte{device}
	}
	return fv
}

func fvFirstDevice(fv *dbproto.FileVersion) ([]byte, bool) {
	if len(fv.Devices) != 0 {
		return fv.Devices[0], true
	}
	if len(fv.InvalidDevices) != 0 {
		return fv.InvalidDevices[0], true
	}
	return nil, false
}

func fvIsInvalid(fv *dbproto.FileVersion) bool {
	return fv == nil || len(fv.Devices) == 0
}

func fvDeviceCount(fv *dbproto.FileVersion) int {
	return len(fv.Devices) + len(fv.InvalidDevices)
}

func fvCopy(fv *dbproto.FileVersion) *dbproto.FileVersion {
	return proto.Clone(fv).(*dbproto.FileVersion)
}
