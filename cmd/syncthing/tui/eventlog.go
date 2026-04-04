// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package tui

import (
	"charm.land/lipgloss/v2"
)

type eventLogModel struct {
	scrollOffset int
}

func eventTypeStyle(typ string, styles Styles) lipgloss.Style {
	switch typ {
	case "DeviceConnected":
		return styles.DeviceConnected
	case "DeviceDisconnected":
		return styles.DeviceDisconnected
	case "DevicePaused":
		return styles.DevicePaused
	case "DeviceResumed":
		return styles.DeviceConnected
	case "StateChanged":
		return styles.StateSyncing
	case "FolderSummary":
		return styles.StateIdle
	case "FolderCompletion":
		return styles.StateSyncing
	case "FolderErrors":
		return styles.StateError
	case "ItemFinished":
		return styles.StateIdle
	case "ItemStarted":
		return styles.StateSyncing
	case "FolderPaused":
		return styles.StatePaused
	case "FolderResumed":
		return styles.StateIdle
	case "PendingDevicesChanged", "PendingFoldersChanged":
		return styles.PendingBadge
	case "ConfigSaved":
		return styles.Label
	default:
		return styles.Muted
	}
}
