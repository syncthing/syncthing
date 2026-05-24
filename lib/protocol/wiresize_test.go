// Copyright (C) 2026 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package protocol

import (
	"testing"

	"google.golang.org/protobuf/proto"
)

func BenchmarkFileInfoSize(b *testing.B) {
	fi := benchmarkFileInfo()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = proto.Size(fi.ToWire(true))
	}
}

func benchmarkFileInfo() FileInfo {
	blocks := make([]BlockInfo, 32)
	for i := range blocks {
		blocks[i] = BlockInfo{
			Offset: int64(i * (128 << 10)),
			Size:   128 << 10,
			Hash:   make([]byte, 32),
		}
	}

	return FileInfo{
		Name:               "dir/subdir/file.bin",
		Size:               int64(len(blocks) * (128 << 10)),
		ModifiedS:          1710000000,
		ModifiedBy:         123456789,
		Version:            Vector{Counters: []Counter{{ID: 1, Value: 10}, {ID: 2, Value: 20}}},
		Sequence:           99,
		Blocks:             blocks,
		BlocksHash:         make([]byte, 32),
		PreviousBlocksHash: make([]byte, 32),
		Encrypted:          []byte("enc"),
		Permissions:        0o644,
		ModifiedNs:         123456789,
		RawBlockSize:       128 << 10,
		LocalFlags:         FlagLocalRemoteInvalid,
		Platform: PlatformData{
			Unix: &UnixData{
				OwnerName: "alice",
				GroupName: "staff",
				UID:       1000,
				GID:       1000,
			},
			Linux: &XattrData{
				Xattrs: []Xattr{
					{Name: "user.comment", Value: []byte("hello")},
					{Name: "user.mime", Value: []byte("application/octet-stream")},
				},
			},
		},
	}
}
