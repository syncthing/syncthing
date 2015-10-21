// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package db

import (
	"bytes"

	"github.com/calmh/xdr"
	"github.com/syncthing/syncthing/lib/protocol"
)

type FileInfoTruncated struct {
	protocol.FileInfo
}

func (o *FileInfoTruncated) UnmarshalXDR(bs []byte) error {
	var br = bytes.NewReader(bs)
	var xr = xdr.NewReader(br)
	return o.DecodeXDRFrom(xr)
}

func (o *FileInfoTruncated) DecodeXDRFrom(xr *xdr.Reader) error {
	o.Name = xr.ReadStringMax(8192)
	o.Flags = xr.ReadUint32()
	o.Modified = int64(xr.ReadUint64())
	(&o.Version).DecodeXDRFrom(xr)
	o.LocalVersion = int64(xr.ReadUint64())
	_BlocksSize := int(xr.ReadUint32())
	if _BlocksSize < 0 {
		return xdr.ElementSizeExceeded("Blocks", _BlocksSize, 1000000)
	}
	if _BlocksSize > 1000000 {
		return xdr.ElementSizeExceeded("Blocks", _BlocksSize, 1000000)
	}

	buf := make([]byte, 64)
	for i := 0; i < _BlocksSize; i++ {
		size := xr.ReadUint32()
		o.CachedSize += int64(size)
		xr.ReadBytesMaxInto(64, buf)
	}
	return xr.Error()
}

func BlocksToSize(num int) int64 {
	if num < 2 {
		return protocol.BlockSize / 2
	}
	return int64(num-1)*protocol.BlockSize + protocol.BlockSize/2
}
