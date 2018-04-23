// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// Package diskoverflow provides several data container types which are limited
// in their memory usage. Once the total memory limit is reached, all new data
// is written to disk.
// Do not use any instances of these types concurrently!
package diskoverflow

import (
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

// Value must be implemented by every type that is to be stored in a disk spilling container.
type Value interface {
	Bytes() int64 // estimated size in memory in bytes
	ToByte() []byte
	FromByte([]byte)
}

// Common is the interface implemented by all disk spilling containers
// Always call Close() once an instance is out of use to register released memory.
type Common interface {
	Close()
	Length() int
}

// ValueFileInfo implements Value for protocol.FileInfo
type ValueFileInfo struct{ protocol.FileInfo }

func (s ValueFileInfo) Bytes() int64 {
	return int64(s.ProtoSize())
}

func (s ValueFileInfo) ToByte() []byte {
	data, err := s.Marshal()
	if err != nil {
		panic("bug: marshalling FileInfo should never fail: " + err.Error())
	}
	return data
}

func (s ValueFileInfo) FromByte(v []byte) {
	var f protocol.FileInfo
	if err := f.Unmarshal(v); err != nil {
		panic("unmarshal failed: " + err.Error())
	}
}

// ValueFileInfoSlice implements Value for []protocol.FileInfo by abusing protocol.Index
type ValueFileInfoSlice struct{ protocol.Index }

func NewValueFileInfoSlice(files []protocol.FileInfo) ValueFileInfoSlice {
	return ValueFileInfoSlice{protocol.Index{Files: files}}
}

func (s ValueFileInfoSlice) Files() []protocol.FileInfo {
	return s.Index.Files
}

func (s ValueFileInfoSlice) Append(f protocol.FileInfo) ValueFileInfoSlice {
	s.Index.Files = append(s.Index.Files, f)
	return s
}

func (s ValueFileInfoSlice) Bytes() int64 {
	return int64(s.ProtoSize())
}

func (s ValueFileInfoSlice) ToByte() []byte {
	data, err := s.Marshal()
	if err != nil {
		panic("bug: marshalling FileInfo should never fail: " + err.Error())
	}
	return data
}

func (s ValueFileInfoSlice) FromByte(v []byte) {
	var f protocol.FileInfo
	if err := f.Unmarshal(v); err != nil {
		panic("unmarshal failed: " + err.Error())
	}
}

const minCompactionSize = 10 << 20

// levelDB options to minimize resource usage.
var levelDBOpts = &opt.Options{
	OpenFilesCacheCapacity: 10,
	WriteBuffer:            512 << 10,
}

type common interface {
	close()
	length() int
}
