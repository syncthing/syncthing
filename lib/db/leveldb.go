// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

//go:generate -command genxdr go run ../../Godeps/_workspace/src/github.com/calmh/xdr/cmd/genxdr/main.go
//go:generate genxdr -o leveldb_xdr.go leveldb.go

package db

import (
	"bytes"
	"fmt"

	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/sync"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

var (
	clockTick int64
	clockMut  = sync.NewMutex()
)

func clock(v int64) int64 {
	clockMut.Lock()
	defer clockMut.Unlock()
	if v > clockTick {
		clockTick = v + 1
	} else {
		clockTick++
	}
	return clockTick
}

const (
	KeyTypeDevice = iota
	KeyTypeGlobal
	KeyTypeBlock
	KeyTypeDeviceStatistic
	KeyTypeFolderStatistic
	KeyTypeVirtualMtime
)

type fileVersion struct {
	version protocol.Vector
	device  []byte
}

type versionList struct {
	versions []fileVersion
}

func (l versionList) String() string {
	var b bytes.Buffer
	var id protocol.DeviceID
	b.WriteString("{")
	for i, v := range l.versions {
		if i > 0 {
			b.WriteString(", ")
		}
		copy(id[:], v.device)
		fmt.Fprintf(&b, "{%d, %v}", v.version, id)
	}
	b.WriteString("}")
	return b.String()
}

type fileList []protocol.FileInfo

func (l fileList) Len() int {
	return len(l)
}

func (l fileList) Swap(a, b int) {
	l[a], l[b] = l[b], l[a]
}

func (l fileList) Less(a, b int) bool {
	return l[a].Name < l[b].Name
}

type dbReader interface {
	Get([]byte, *opt.ReadOptions) ([]byte, error)
}

// Flush batches to disk when they contain this many records.
const batchFlushSize = 64

// deviceKey returns a byte slice encoding the following information:
//	   keyTypeDevice (1 byte)
//	   folder (64 bytes)
//	   device (32 bytes)
//	   name (variable size)
func deviceKey(folder, device, file []byte) []byte {
	return deviceKeyInto(nil, folder, device, file)
}

func deviceKeyInto(k []byte, folder, device, file []byte) []byte {
	reqLen := 1 + 64 + 32 + len(file)
	if len(k) < reqLen {
		k = make([]byte, reqLen)
	}
	k[0] = KeyTypeDevice
	if len(folder) > 64 {
		panic("folder name too long")
	}
	copy(k[1:], []byte(folder))
	copy(k[1+64:], device[:])
	copy(k[1+64+32:], []byte(file))
	return k[:reqLen]
}

func deviceKeyName(key []byte) []byte {
	return key[1+64+32:]
}

func deviceKeyFolder(key []byte) []byte {
	folder := key[1 : 1+64]
	izero := bytes.IndexByte(folder, 0)
	if izero < 0 {
		return folder
	}
	return folder[:izero]
}

func deviceKeyDevice(key []byte) []byte {
	return key[1+64 : 1+64+32]
}

// globalKey returns a byte slice encoding the following information:
//	   keyTypeGlobal (1 byte)
//	   folder (64 bytes)
//	   name (variable size)
func globalKey(folder, file []byte) []byte {
	k := make([]byte, 1+64+len(file))
	k[0] = KeyTypeGlobal
	if len(folder) > 64 {
		panic("folder name too long")
	}
	copy(k[1:], []byte(folder))
	copy(k[1+64:], []byte(file))
	return k
}

func globalKeyName(key []byte) []byte {
	return key[1+64:]
}

func globalKeyFolder(key []byte) []byte {
	folder := key[1 : 1+64]
	izero := bytes.IndexByte(folder, 0)
	if izero < 0 {
		return folder
	}
	return folder[:izero]
}

func getFile(db dbReader, folder, device, file []byte) (protocol.FileInfo, bool) {
	nk := deviceKey(folder, device, file)
	bs, err := db.Get(nk, nil)
	if err == leveldb.ErrNotFound {
		return protocol.FileInfo{}, false
	}
	if err != nil {
		panic(err)
	}

	var f protocol.FileInfo
	err = f.UnmarshalXDR(bs)
	if err != nil {
		panic(err)
	}
	return f, true
}
