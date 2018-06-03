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
)

// Value must be implemented by every type that is to be stored in a disk spilling container.
type Value interface {
	Size() int64 // estimated size in memory in bytes
	Marshal() []byte
	Unmarshal([]byte)
}

// Common is the interface implemented by all disk spilling containers
// Always call Close() once an instance is out of use to register released memory.
type Common interface {
	Close()
	Length() int
}

// ValueFileInfo implements Value for protocol.FileInfo
type ValueFileInfo struct{ protocol.FileInfo }

func (s ValueFileInfo) Size() int64 {
	return int64(s.ProtoSize())
}

func (s ValueFileInfo) Marshal() []byte {
	data, err := s.FileInfo.Marshal()
	if err != nil {
		panic("bug: marshalling FileInfo should never fail: " + err.Error())
	}
	return data
}

func (s ValueFileInfo) Unmarshal(v []byte) {
	if err := s.FileInfo.Unmarshal(v); err != nil {
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

func (s ValueFileInfoSlice) Size() int64 {
	return int64(s.ProtoSize())
}

func (s ValueFileInfoSlice) Marshal() []byte {
	data, err := s.Index.Marshal()
	if err != nil {
		panic("bug: marshalling FileInfo should never fail: " + err.Error())
	}
	return data
}

func (s ValueFileInfoSlice) Unmarshal(v []byte) {
	if err := s.Index.Unmarshal(v); err != nil {
		panic("unmarshal failed: " + err.Error())
	}
}

const minCompactionSize = 10 << protocol.MiB

type common interface {
	close()
	length() int
}
