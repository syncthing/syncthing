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
)

type pullSchedule interface {
	Reorder(devicesWithFile, connectedDevices []protocol.DeviceID, blocks []protocol.BlockInfo)
}

type inOrderPullSchedule struct{}

func (inOrderPullSchedule) Reorder(_, _ []protocol.DeviceID, _ []protocol.BlockInfo) {
	// Nothing
}

type randomPullSchedule struct{}

func (randomPullSchedule) Reorder(_, _ []protocol.DeviceID, blocks []protocol.BlockInfo) {
	rand.Shuffle(blocks)
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
		return standardPullSchedule{
			id:            id,
			commonDevices: commonDevices,
		}
	}
}

type standardPullSchedule struct {
	id            protocol.DeviceID
	commonDevices map[protocol.DeviceID][]protocol.DeviceID
}

func (p standardPullSchedule) Reorder(devicesWithFile, connected []protocol.DeviceID, blocks []protocol.BlockInfo) {
	rand.Shuffle(blocks)
}
