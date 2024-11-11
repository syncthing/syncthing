// Copyright (C) 2024 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package blockstorage

type HashBlockStorageI interface {
	Get(hash []byte) (data []byte, ok bool)
	Set(hash []byte, data []byte)
	Delete(hash []byte)
	GetMeta(name string) (data []byte, ok bool)
	SetMeta(name string, data []byte)
	DeleteMeta(name string)
}
