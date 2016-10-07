// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package db

import (
	"bytes"
	"encoding/hex"
	"fmt"

	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

const (
	KeyTypeDevice = iota
	KeyTypeGlobal
	KeyTypeBlock
	KeyTypeDeviceStatistic
	KeyTypeFolderStatistic
	KeyTypeVirtualMtime
	KeyTypeFolderIdx
	KeyTypeDeviceIdx
	KeyTypeIndexID
)

func (l VersionList) String() string {
	var b bytes.Buffer
	var id protocol.DeviceID
	b.WriteString("{")
	for i, v := range l.Versions {
		if i > 0 {
			b.WriteString(", ")
		}
		copy(id[:], v.Device)
		fmt.Fprintf(&b, "{%d, %v}", v.Version, id)
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
	err = f.Unmarshal(bs)
	if err != nil {
		l.Infoln("Woops, unmarshal error!")
		l.Infoln(hex.Dump(key))
		l.Infoln(hex.Dump(bs))
		panic(err)
	}
	return f, true
}
