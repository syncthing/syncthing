// Copyright (C) 2020 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"reflect"
	"slices"
	"testing"

	"github.com/syncthing/syncthing/lib/protocol"
)

var someBlocks = []protocol.BlockInfo{{Offset: 1}, {Offset: 2}, {Offset: 3}}

func Test_chunk(t *testing.T) {
	type args struct {
		blocks    []protocol.BlockInfo
		partCount int
	}
	tests := []struct {
		name string
		args args
		want [][]protocol.BlockInfo
	}{
		{"one", args{someBlocks, 1}, [][]protocol.BlockInfo{someBlocks}},
		{"two", args{someBlocks, 2}, [][]protocol.BlockInfo{someBlocks[:2], someBlocks[2:]}},
		{"three", args{someBlocks, 3}, [][]protocol.BlockInfo{someBlocks[:1], someBlocks[1:2], someBlocks[2:]}},
		{"four", args{someBlocks, 4}, [][]protocol.BlockInfo{someBlocks[:1], someBlocks[1:2], someBlocks[2:]}},
		// Never happens as myIdx would be -1, so we'd return in order.
		{"zero", args{someBlocks, 0}, [][]protocol.BlockInfo{someBlocks}},
		{"empty-one", args{nil, 1}, [][]protocol.BlockInfo{}},
		{"empty-zero", args{nil, 0}, [][]protocol.BlockInfo{nil}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := chunk(tt.args.blocks, tt.args.partCount); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("chunk() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_inOrderBlockPullReorderer_Reorder(t *testing.T) {
	tests := []struct {
		name   string
		blocks []protocol.BlockInfo
		want   []protocol.BlockInfo
	}{
		{"basic", someBlocks, someBlocks},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := inOrderBlockPullReorderer{}
			if got := in.Reorder(tt.blocks); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Reorder() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_standardBlockPullReorderer_Reorder(t *testing.T) {
	// Order the devices, so we know their ordering ahead of time.
	devices := []protocol.DeviceID{myID, device1, device2}
	slices.SortFunc(devices, func(a, b protocol.DeviceID) int {
		return a.Compare(b)
	})

	blocks := func(i ...int) []protocol.BlockInfo {
		b := make([]protocol.BlockInfo, 0, len(i))
		for _, v := range i {
			b = append(b, protocol.BlockInfo{Offset: int64(v)})
		}
		return b
	}
	tests := []struct {
		name    string
		myId    protocol.DeviceID
		devices []protocol.DeviceID
		blocks  []protocol.BlockInfo
		want    []protocol.BlockInfo
	}{
		{"front", devices[0], []protocol.DeviceID{devices[1], devices[2]}, blocks(1, 2, 3), blocks(1, 2, 3)},
		{"back", devices[2], []protocol.DeviceID{devices[0], devices[1]}, blocks(1, 2, 3), blocks(3, 1, 2)},
		{"few-blocks", devices[2], []protocol.DeviceID{devices[0], devices[1]}, blocks(1), blocks(1)},
		{"more-than-one-block", devices[1], []protocol.DeviceID{devices[0]}, blocks(1, 2, 3, 4), blocks(3, 4, 1, 2)},
		{"empty-blocks", devices[0], []protocol.DeviceID{devices[1]}, blocks(), blocks()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newStandardBlockPullReorderer(tt.myId, tt.devices)
			p.shuffle = func(i interface{}) {} // Noop shuffle
			if got := p.Reorder(tt.blocks); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("reorderBlocksForDevices() = %v, want %v (my idx: %d, count %d)", got, tt.want, p.myIndex, p.count)
			}
		})
	}
}
