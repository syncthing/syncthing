// Copyright (C) 2020 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"slices"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/rand"
)

type blockPullReorderer interface {
	Reorder(blocks []protocol.BlockInfo) []protocol.BlockInfo
}

func newBlockPullReorderer(order config.BlockPullOrder, id protocol.DeviceID, otherDevices []protocol.DeviceID) blockPullReorderer {
	switch order {
	case config.BlockPullOrderRandom:
		return randomOrderBlockPullReorderer{}
	case config.BlockPullOrderInOrder:
		return inOrderBlockPullReorderer{}
	case config.BlockPullOrderStandard:
		fallthrough
	default:
		return newStandardBlockPullReorderer(id, otherDevices)
	}
}

type inOrderBlockPullReorderer struct{}

func (inOrderBlockPullReorderer) Reorder(blocks []protocol.BlockInfo) []protocol.BlockInfo {
	return blocks
}

type randomOrderBlockPullReorderer struct{}

func (randomOrderBlockPullReorderer) Reorder(blocks []protocol.BlockInfo) []protocol.BlockInfo {
	rand.Shuffle(blocks)
	return blocks
}

type standardBlockPullReorderer struct {
	myIndex int
	count   int
	shuffle func(interface{}) // Used for test
}

func newStandardBlockPullReorderer(id protocol.DeviceID, otherDevices []protocol.DeviceID) *standardBlockPullReorderer {
	allDevices := append(otherDevices, id) //nolint:gocritic
	slices.SortFunc(allDevices, func(a, b protocol.DeviceID) int {
		return a.Compare(b)
	})
	// Find our index
	myIndex := -1
	for i, dev := range allDevices {
		if dev == id {
			myIndex = i
			break
		}
	}
	if myIndex < 0 {
		panic("bug: could not find my own index")
	}
	return &standardBlockPullReorderer{
		myIndex: myIndex,
		count:   len(allDevices),
		shuffle: rand.Shuffle,
	}
}

func (p *standardBlockPullReorderer) Reorder(blocks []protocol.BlockInfo) []protocol.BlockInfo {
	if len(blocks) == 0 {
		return blocks
	}

	// Split the blocks into len(allDevices) chunks. Chunk count might be less than device count, if there are more
	// devices than blocks.
	chunks := chunk(blocks, p.count)

	newBlocks := make([]protocol.BlockInfo, 0, len(blocks))

	// First add our own chunk. We might fall off the list if there are more devices than chunks...
	if p.myIndex < len(chunks) {
		newBlocks = append(newBlocks, chunks[p.myIndex]...)
	}

	// The rest of the chunks we fetch in a random order in whole chunks.
	// Generate chunk index slice and shuffle it
	indexes := make([]int, 0, len(chunks)-1)
	for i := range chunks {
		if i != p.myIndex {
			indexes = append(indexes, i)
		}
	}

	p.shuffle(indexes)

	// Append the chunks in the order of the index slices.
	for _, idx := range indexes {
		newBlocks = append(newBlocks, chunks[idx]...)
	}

	return newBlocks
}

func chunk(blocks []protocol.BlockInfo, partCount int) [][]protocol.BlockInfo {
	if partCount == 0 {
		return [][]protocol.BlockInfo{blocks}
	}
	count := len(blocks)
	chunkSize := (count + partCount - 1) / partCount
	parts := make([][]protocol.BlockInfo, 0, partCount)
	for i := 0; i < count; i += chunkSize {
		end := i + chunkSize
		if end > count {
			end = count
		}
		parts = append(parts, blocks[i:end])
	}
	return parts
}
