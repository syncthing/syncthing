// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"testing"

	"github.com/syncthing/syncthing/lib/protocol"
)

func TestDeviceDownloadState(t *testing.T) {
	v1 := (protocol.Vector{}).Update(0)
	v2 := (protocol.Vector{}).Update(1)

	// file 1 version 1 part 1
	f1v1p1 := protocol.FileDownloadProgressUpdate{UpdateType: protocol.FileDownloadProgressUpdateTypeAppend, Name: "f1", Version: v1, BlockIndexes: []int{0, 1, 2}}
	f1v1p2 := protocol.FileDownloadProgressUpdate{UpdateType: protocol.FileDownloadProgressUpdateTypeAppend, Name: "f1", Version: v1, BlockIndexes: []int{3, 4, 5}}
	f1v1del := protocol.FileDownloadProgressUpdate{UpdateType: protocol.FileDownloadProgressUpdateTypeForget, Name: "f1", Version: v1, BlockIndexes: nil}
	f1v2p1 := protocol.FileDownloadProgressUpdate{UpdateType: protocol.FileDownloadProgressUpdateTypeAppend, Name: "f1", Version: v2, BlockIndexes: []int{10, 11, 12}}
	f1v2p2 := protocol.FileDownloadProgressUpdate{UpdateType: protocol.FileDownloadProgressUpdateTypeAppend, Name: "f1", Version: v2, BlockIndexes: []int{13, 14, 15}}
	f1v2del := protocol.FileDownloadProgressUpdate{UpdateType: protocol.FileDownloadProgressUpdateTypeForget, Name: "f1", Version: v2, BlockIndexes: nil}

	f2v1p1 := protocol.FileDownloadProgressUpdate{UpdateType: protocol.FileDownloadProgressUpdateTypeAppend, Name: "f2", Version: v1, BlockIndexes: []int{20, 21, 22}}
	f2v1p2 := protocol.FileDownloadProgressUpdate{UpdateType: protocol.FileDownloadProgressUpdateTypeAppend, Name: "f2", Version: v1, BlockIndexes: []int{23, 24, 25}}
	f2v1del := protocol.FileDownloadProgressUpdate{UpdateType: protocol.FileDownloadProgressUpdateTypeForget, Name: "f2", Version: v1, BlockIndexes: nil}

	tests := []struct {
		updates                  []protocol.FileDownloadProgressUpdate
		shouldHaveIndexesFrom    []protocol.FileDownloadProgressUpdate
		shouldNotHaveIndexesFrom []protocol.FileDownloadProgressUpdate
	}{
		{ // 1
			[]protocol.FileDownloadProgressUpdate{f1v1p1},
			[]protocol.FileDownloadProgressUpdate{f1v1p1},
			[]protocol.FileDownloadProgressUpdate{f1v1p2, f1v2p1, f1v2p2},
		},
		{ // 2
			[]protocol.FileDownloadProgressUpdate{f1v1p1, f1v1p2},
			[]protocol.FileDownloadProgressUpdate{f1v1p1, f1v1p2},
			[]protocol.FileDownloadProgressUpdate{f1v2p1, f1v2p2},
		},
		{ // 3
			[]protocol.FileDownloadProgressUpdate{f1v1p1, f1v1p2, f1v1del},
			nil,
			[]protocol.FileDownloadProgressUpdate{f1v1p1, f1v1p2, f1v2p1, f1v2p2},
		},
		{ // 4
			[]protocol.FileDownloadProgressUpdate{f1v1p1, f1v1p2, f1v2del},
			[]protocol.FileDownloadProgressUpdate{f1v1p1, f1v1p2},
			[]protocol.FileDownloadProgressUpdate{f1v2p1, f1v2p2},
		},
		{ // 5
			// v2 replaces old v1 data
			[]protocol.FileDownloadProgressUpdate{f1v1p1, f1v1p2, f1v2p1},
			[]protocol.FileDownloadProgressUpdate{f1v2p1},
			[]protocol.FileDownloadProgressUpdate{f1v1p1, f1v1p2, f1v2p2},
		},
		{ // 6
			// v1 delete on v2 data does nothing
			[]protocol.FileDownloadProgressUpdate{f1v1p1, f1v1p2, f1v2p1, f1v1del},
			[]protocol.FileDownloadProgressUpdate{f1v2p1},
			[]protocol.FileDownloadProgressUpdate{f1v1p1, f1v1p2, f1v2p2},
		},
		{ // 7
			// v2 replacees v1, v2 gets deleted, and v2 part 2 gets added.
			[]protocol.FileDownloadProgressUpdate{f1v1p1, f1v1p2, f1v2p1, f1v2del, f1v2p2},
			[]protocol.FileDownloadProgressUpdate{f1v2p2},
			[]protocol.FileDownloadProgressUpdate{f1v1p1, f1v1p2, f1v2p1},
		},
		// Multiple files in one go
		{ // 8
			[]protocol.FileDownloadProgressUpdate{f1v1p1, f2v1p1},
			[]protocol.FileDownloadProgressUpdate{f1v1p1, f2v1p1},
			[]protocol.FileDownloadProgressUpdate{f1v1p2, f2v1p2},
		},
		{ // 9
			[]protocol.FileDownloadProgressUpdate{f1v1p1, f2v1p1, f2v1del},
			[]protocol.FileDownloadProgressUpdate{f1v1p1},
			[]protocol.FileDownloadProgressUpdate{f2v1p1, f2v1p1},
		},
		{ // 10
			[]protocol.FileDownloadProgressUpdate{f1v1p1, f2v1del, f2v1p1},
			[]protocol.FileDownloadProgressUpdate{f1v1p1, f2v1p1},
			[]protocol.FileDownloadProgressUpdate{f1v1p2, f2v1p2},
		},
	}

	for i, test := range tests {
		s := newDeviceDownloadState()
		s.Update("folder", test.updates)

		for _, expected := range test.shouldHaveIndexesFrom {
			for _, n := range expected.BlockIndexes {
				if !s.Has("folder", expected.Name, expected.Version, n) {
					t.Error("Test", i+1, "error:", expected.Name, expected.Version, "missing", n)
				}
			}
		}

		for _, unexpected := range test.shouldNotHaveIndexesFrom {
			for _, n := range unexpected.BlockIndexes {
				if s.Has("folder", unexpected.Name, unexpected.Version, n) {
					t.Error("Test", i+1, "error:", unexpected.Name, unexpected.Version, "has extra", n)
				}
			}
		}
	}
}
