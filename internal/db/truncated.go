// Copyright (C) 2014 The Syncthing Authors.
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

//go:generate -command genxdr go run ../../Godeps/_workspace/src/github.com/calmh/xdr/cmd/genxdr/main.go
//go:generate genxdr -o truncated_xdr.go truncated.go

package db

import (
	"fmt"

	"github.com/syncthing/protocol"
)

// Used for unmarshalling a FileInfo structure but skipping the block list.
type FileInfoTruncated struct {
	Name         string // max:8192
	Flags        uint32
	Modified     int64
	Version      int64
	LocalVersion int64
	NumBlocks    int32
}

func ToTruncated(file protocol.FileInfo) FileInfoTruncated {
	return FileInfoTruncated{
		Name:         file.Name,
		Flags:        file.Flags,
		Modified:     file.Modified,
		Version:      file.Version,
		LocalVersion: file.LocalVersion,
		NumBlocks:    int32(len(file.Blocks)),
	}
}

func (f FileInfoTruncated) String() string {
	return fmt.Sprintf("File{Name:%q, Flags:0%o, Modified:%d, Version:%d, Size:%d, NumBlocks:%d}",
		f.Name, f.Flags, f.Modified, f.Version, f.Size(), f.NumBlocks)
}

// Returns a statistical guess on the size, not the exact figure
func (f FileInfoTruncated) Size() int64 {
	if f.IsDeleted() || f.IsDirectory() {
		return 128
	}
	return BlocksToSize(int(f.NumBlocks))
}

func (f FileInfoTruncated) IsDeleted() bool {
	return f.Flags&protocol.FlagDeleted != 0
}

func (f FileInfoTruncated) IsInvalid() bool {
	return f.Flags&protocol.FlagInvalid != 0
}

func (f FileInfoTruncated) IsDirectory() bool {
	return f.Flags&protocol.FlagDirectory != 0
}

func (f FileInfoTruncated) IsSymlink() bool {
	return f.Flags&protocol.FlagSymlink != 0
}

func (f FileInfoTruncated) HasPermissionBits() bool {
	return f.Flags&protocol.FlagNoPermBits == 0
}

func BlocksToSize(num int) int64 {
	if num < 2 {
		return protocol.BlockSize / 2
	}
	return int64(num-1)*protocol.BlockSize + protocol.BlockSize/2
}
