// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package db

import "github.com/syncthing/protocol"

type FileInfoTruncated struct {
	protocol.FileInfo
	ActualSize int64
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
