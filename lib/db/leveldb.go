// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

//go:generate -command genxdr go run ../../vendor/github.com/calmh/xdr/cmd/genxdr/main.go
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
	KeyTypeFolderIdx
	KeyTypeDeviceIdx
)

type fileVersion struct {
	version protocol.Vector
	device  []byte
}

type VersionList struct {
	versions []fileVersion
}

func (l VersionList) String() string {
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

func getFile(db dbReader, key []byte) (protocol.FileInfo, bool) {
	bs, err := db.Get(key, nil)
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
