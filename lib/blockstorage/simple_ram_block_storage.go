// Copyright (C) 2024 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package blockstorage

import "encoding/hex"

func hashToStringMapKey(hash []byte) string {
	return hex.EncodeToString(hash)
}

type HashedBlockMapInMemory struct {
	blockMap map[string][]byte
}

func NewHashedBlockMapInMemory() *HashedBlockMapInMemory {
	return &HashedBlockMapInMemory{
		blockMap: make(map[string][]byte),
	}
}

func (hm *HashedBlockMapInMemory) Get(hash []byte) (data []byte, ok bool) {
	data, ok = hm.blockMap[hashToStringMapKey(hash)]
	return
}

func (hm *HashedBlockMapInMemory) Set(hash []byte, data []byte) {
	hm.blockMap[hashToStringMapKey(hash)] = data
}

func (hm *HashedBlockMapInMemory) Delete(hash []byte) {
	delete(hm.blockMap, hashToStringMapKey(hash))
}
