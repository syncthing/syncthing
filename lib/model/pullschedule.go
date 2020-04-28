// Copyright (C) 2020 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/rand"
	"sort"
)

type pullSchedule interface {
	Reorder(connectedWithFile, connectedSharingFolder []protocol.DeviceID, blocks []protocol.BlockInfo) []protocol.BlockInfo
}

type inOrderPullSchedule struct{}

func (inOrderPullSchedule) Reorder(_, _ []protocol.DeviceID, blocks []protocol.BlockInfo) []protocol.BlockInfo {
	return blocks
}

type randomPullSchedule struct{}

func (randomPullSchedule) Reorder(_, _ []protocol.DeviceID, blocks []protocol.BlockInfo) []protocol.BlockInfo {
	rand.Shuffle(blocks)
	return blocks
}

func (m *model) getCommonDevicesSharingTheFolder(folder string) map[protocol.DeviceID][]protocol.DeviceID {
	m.fmut.RLock()
	folderCfg, ok := m.folderCfgs[folder]
	m.fmut.RUnlock()
	if !ok {
		return nil
	}

	m.pmut.RLock()
	deviceDevices := make(map[protocol.DeviceID][]protocol.DeviceID, len(folderCfg.Devices))
	for _, device := range folderCfg.DeviceIDs() {
		cc, ok := m.ccMessages[device]
		if !ok {
			continue
		}

		for _, ccFolder := range cc.Folders {
			if ccFolder.ID != folder {
				continue
			}
			for _, ccFolderDevice := range ccFolder.Devices {
				if !folderCfg.SharedWith(ccFolderDevice.ID) {
					continue
				}
				deviceDevices[device] = append(deviceDevices[device], ccFolderDevice.ID)
			}
			break
		}
	}
	m.pmut.RUnlock()
	return deviceDevices
}

func newPullSchedule(schedule config.PullSchedule, id protocol.DeviceID, commonDevices map[protocol.DeviceID][]protocol.DeviceID) pullSchedule {
	switch schedule {
	case config.PullScheduleRandom:
		return randomPullSchedule{}
	case config.PullScheduleInOrder:
		return inOrderPullSchedule{}
	case config.PullScheduleStandard:
		fallthrough
	default:
		return &standardPullSchedule{
			myId:          id,
			commonDevices: commonDevices,
			shuffle:       rand.Shuffle,
		}
	}
}

type standardPullSchedule struct {
	myId          protocol.DeviceID
	commonDevices map[protocol.DeviceID][]protocol.DeviceID
	shuffle       func(interface{}) // Used for test
}

func (p *standardPullSchedule) Reorder(connectedWithFile, connectedSharingFolder []protocol.DeviceID, blocks []protocol.BlockInfo) []protocol.BlockInfo {
	// Obviously the list of "blocks" might vary per device based on what they find locally and what they have in a
	// temp file, etc. But this is best effort.
	devices := p.devicesThatNeedTheFileAndCanGetItDirectly(connectedWithFile, connectedSharingFolder)
	// Sort the device ids
	sort.Slice(devices, func(i, j int) bool {
		return devices[i].Compare(devices[j]) == -1
	})
	return p.reorderBlocksForDevices(devices, blocks)
}

func (p *standardPullSchedule) reorderBlocksForDevices(devices []protocol.DeviceID, blocks []protocol.BlockInfo) []protocol.BlockInfo {
	// Maybe we're the only ones who need it, or maybe this thing doesn't work.
	if len(devices) < 2 || len(blocks) == 0 {
		return blocks
	}

	// Find our index
	myIndex := -1
	for i, dev := range devices {
		if dev == p.myId {
			myIndex = i
			break
		}
	}

	if myIndex == -1 {
		// Wat?
		return blocks
	}

	// Split the blocks into len(devices) chunks. Chunk count might be less than device count, if there are more
	// devices than blocks.
	chunks := chunk(blocks, len(devices))

	newBlocks := make([]protocol.BlockInfo, 0, len(blocks))

	// First add our own chunk. We might fall off the list if there are more devices than chunks...
	if myIndex < len(chunks) {
		newBlocks = append(newBlocks, chunks[myIndex]...)
	}

	// The rest of the chunks we fetch in a random order in whole chunks.
	// Generate chunk index slice and shuffle it
	indexes := make([]int, 0, len(chunks)-1)
	for i := 0; i < len(chunks); i++ {
		if i != myIndex {
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

func (p *standardPullSchedule) devicesThatNeedTheFileAndCanGetItDirectly(connectedWithFile, connectedSharingFolder []protocol.DeviceID) []protocol.DeviceID {
	// This is a list of devices which need the file which we can reach, and who can also reach the available sources
	// of the file.
	// Include ourselves to work out the ordering of the first chunk.
	devices := []protocol.DeviceID{p.myId}

	for _, connectedDevice := range connectedSharingFolder {
		if contains(connectedWithFile, connectedDevice) {
			// If's one of the source devices, so it's not a candidate for work sharing.
			continue
		}
		for _, deviceWithFile := range connectedWithFile {
			if p.sharesFolderBothWays(deviceWithFile, connectedDevice) {
				devices = append(devices, connectedDevice)
				break
			}
		}
	}

	return devices
}

func (p *standardPullSchedule) sharesFolderBothWays(one protocol.DeviceID, other protocol.DeviceID) bool {
	return contains(p.commonDevices[one], other) && contains(p.commonDevices[other], one)
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

func contains(devices []protocol.DeviceID, id protocol.DeviceID) bool {
	for _, dev := range devices {
		if dev == id {
			return true
		}
	}
	return false
}
