// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package db

import (
	"bytes"
	"fmt"

	"github.com/syncthing/syncthing/lib/protocol"
)

func (vl VersionList) String() string {
	var b bytes.Buffer
	var id protocol.DeviceID
	b.WriteString("{")
	for i, v := range vl.Versions {
		if i > 0 {
			b.WriteString(", ")
		}
		copy(id[:], v.Device)
		fmt.Fprintf(&b, "{%v, %v, %v}", v.Version, id, v.Invalid)
	}
	b.WriteString("}")
	return b.String()
}

// update brings the VersionList up to date with file. It returns the updated
// VersionList, a potentially removed old FileVersion and its index, as well as
// the index where the new FileVersion was inserted.
func (vl VersionList) update(folder, device []byte, file protocol.FileInfo, db *instance) (_ VersionList, removedFV FileVersion, removedAt int, insertedAt int) {
	removedAt, insertedAt = -1, -1
	for i, v := range vl.Versions {
		if bytes.Equal(v.Device, device) {
			removedAt = i
			removedFV = v
			vl.Versions = append(vl.Versions[:i], vl.Versions[i+1:]...)
			break
		}
	}

	nv := FileVersion{
		Device:  device,
		Version: file.Version,
		Invalid: file.IsInvalid(),
	}
	for i, v := range vl.Versions {
		switch v.Version.Compare(file.Version) {
		case protocol.Equal:
			if nv.Invalid {
				continue
			}
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
			if of, ok := db.getFile(db.keyer.GenerateDeviceFileKey(nil, folder, v.Device, []byte(file.Name))); !ok || file.WinsConflict(of) {
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

// Flush batches to disk when they contain this many records.
const batchFlushSize = 64
