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

package db

import "github.com/syncthing/protocol"

type FileInfoTruncated struct {
	protocol.FileInfo
	ActualSize int64
}

func ToTruncated(file protocol.FileInfo) FileInfoTruncated {
	t := FileInfoTruncated{
		FileInfo:   file,
		ActualSize: file.Size(),
	}
	t.FileInfo.Blocks = nil
	return t
}

func (f *FileInfoTruncated) UnmarshalXDR(bs []byte) error {
	err := f.FileInfo.UnmarshalXDR(bs)
	f.ActualSize = f.FileInfo.Size()
	f.FileInfo.Blocks = nil
	return err
}

func (f FileInfoTruncated) Size() int64 {
	return f.ActualSize
}

func BlocksToSize(num int) int64 {
	if num < 2 {
		return protocol.BlockSize / 2
	}
	return int64(num-1)*protocol.BlockSize + protocol.BlockSize/2
}
