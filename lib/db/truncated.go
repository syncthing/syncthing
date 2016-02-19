// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package db

import (
	"github.com/calmh/xdr"
	"github.com/syncthing/syncthing/lib/protocol"
)

type FileInfoTruncated struct {
	protocol.FileInfo
}

func (o *FileInfoTruncated) UnmarshalXDR(bs []byte) error {
	return o.UnmarshalXDRFrom(&xdr.Unmarshaller{Data: bs})
}

func (o *FileInfoTruncated) UnmarshalXDRFrom(u *xdr.Unmarshaller) error {
	o.Name = u.UnmarshalStringMax(8192)
	o.Flags = u.UnmarshalUint32()
	o.Modified = int64(u.UnmarshalUint64())
	(&o.Version).UnmarshalXDRFrom(u)
	o.LocalVersion = int64(u.UnmarshalUint64())
	_BlocksSize := int(u.UnmarshalUint32())
	if _BlocksSize < 0 {
		return xdr.ElementSizeExceeded("Blocks", _BlocksSize, 10000000)
	} else if _BlocksSize == 0 {
		o.Blocks = nil
	} else {
		if _BlocksSize > 10000000 {
			return xdr.ElementSizeExceeded("Blocks", _BlocksSize, 10000000)
		}
		for i := 0; i < _BlocksSize; i++ {
			size := int64(u.UnmarshalUint32())
			o.CachedSize += size
			u.UnmarshalBytes()
		}
	}

	return u.Error
}

func BlocksToSize(num int) int64 {
	if num < 2 {
		return protocol.BlockSize / 2
	}
	return int64(num-1)*protocol.BlockSize + protocol.BlockSize/2
}
