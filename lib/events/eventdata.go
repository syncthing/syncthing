// Copyright (C) 2022 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package events

import (
	"net/url"

	modeltypes "github.com/syncthing/syncthing/lib/model/types"
	"github.com/syncthing/syncthing/lib/protocol"
)

type StartingEventData struct {
	Home string            `json:"home"`
	MyID protocol.DeviceID `json:"myID"`
}

type StartupCompleteEventData struct {
	MyID protocol.DeviceID `json:"myID"`
}

type DeviceDiscoveredEventData struct {
	Device    protocol.DeviceID `json:"device"`
	Addresses []string          `json:"addrs"`
}

type DeviceConnectedEventData struct {
	ID            protocol.DeviceID `json:"id"`
	DeviceName    string            `json:"deviceName"`
	ClientName    string            `json:"clientName"`
	ClientVersion string            `json:"clientVersion"`
	Type          string            `json:"type"`
	Address       *string           `json:"addr,omitempty"`
}

type DeviceDisconnectedEventData struct {
	ID    protocol.DeviceID `json:"id"`
	Error string            `json:"error"`
}

// DEPRECATED: DeviceRejected event replaced by PendingDevicesChanged
//type DeviceRejectedEventData modeltypes.UpdatedPendingDevice

// type PendingDevicesChangedEventData struct {
// 	Added   []modeltypes.UpdatedPendingDevice   `json:"added,omitempty"`
// 	Removed []modeltypes.PendingDeviceListEntry `json:"removed,omitempty"`
// }

type DevicePausedEventData struct {
	Device protocol.DeviceID `json:"device"`
}
type DeviceResumedEventData DevicePausedEventData

type ClusterConfigReceivedEventData struct {
	Device protocol.DeviceID `json:"device"`
}

type DiskChangeDetectedEventData struct {
	Folder     string `json:"folder"`
	FolderID   string `json:"folderID"` // incorrect, deprecated, kept for historical compliance
	Label      string `json:"label"`
	Action     string `json:"action"`
	Type       string `json:"type"`
	Path       string `json:"path"`
	ModifiedBy string `json:"modifiedBy"`
}
type LocalChangeDetectedEventData DiskChangeDetectedEventData
type RemoteChangeDetectedEventData DiskChangeDetectedEventData

type LocalIndexUpdatedEventData struct {
	Folder    string   `json:"folder"`
	Items     int      `json:"items"`
	Filenames []string `json:"filenames"`
	Sequence  int64    `json:"sequence"`
	Version   int64    `json:"version"` // DEPRECATED: legacy for Sequence
}

type RemoteIndexUpdatedEventData struct {
	Device   protocol.DeviceID `json:"device"`
	Folder   string            `json:"folder"`
	Items    int               `json:"items"`
	Sequence int64             `json:"sequence"`
	Version  int64             `json:"version"` // DEPRECATED: legacy for Sequence
}

type ItemStartedEventData struct {
	Folder string `json:"folder"`
	Item   string `json:"item"`
	Type   string `json:"type"`
	Action string `json:"action"`
}

type ItemFinishedEventData struct {
	ItemStartedEventData

	Error *string `json:"error,omitempty"`
}

type StateChangedEventData struct {
	Folder   string   `json:"folder"`
	To       string   `json:"to"`
	From     string   `json:"from"`
	Duration *float64 `json:"duration,omitempty"`
	Error    *string  `json:"error,omitempty"`
}

// DEPRECATED: FolderRejected event replaced by PendingFoldersChanged
type FolderRejectedEventData struct {
	Folder      string            `json:"folder"`
	FolderLabel string            `json:"folderLabel"`
	Device      protocol.DeviceID `json:"device"`
}

// type PendingFoldersChangedEventData struct {
// 	Added   []modeltypes.UpdatedPendingFolder   `json:"added,omitempty"`
// 	Removed []modeltypes.PendingFolderListEntry `json:"removed,omitempty"`
// }

//type ConfigSavedEventData config.Configuration

type DownloadProgressEventData map[string]map[string]*modeltypes.PullerProgress

type RemoteDownloadProgressEventData struct {
	Device protocol.DeviceID `json:"device"`
	Folder string            `json:"folder"`
	State  map[string]int    `json:"state"`
}

// type FolderSummaryEventData struct {
// 	Folder  string                    `json:"folder"`
// 	Summary *modeltypes.FolderSummary `json:"summary"`
// }

// type FolderCompletionEventData struct {
// 	Folder string            `json:"folder"`
// 	Device protocol.DeviceID `json:"device"`
// 	modeltypes.FolderCompletion
// }

type FolderErrorsEventData struct {
	Folder string      `json:"folder"`
	Errors []modeltypes.FileError `json:"errors,omitempty"`
}

type FolderScanProgressEventData struct {
	Folder  string  `json:"folder"`
	Current int64   `json:"current"`
	Total   int64   `json:"total"`
	Rate    float64 `json:"rate"` // bytes per second
}

type FolderPausedEventData struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}
type FolderResumedEventData FolderPausedEventData

type FolderWatchStateChangedEventData struct {
	Folder string  `json:"folder"`
	From   *string `json:"from,omitempty"`
	To     *string `json:"to,omitempty"`
}

type ListenAddressesChangedEventData struct {
	Address *url.URL   `json:"address"`
	LAN     []*url.URL `json:"lan"`
	WAN     []*url.URL `json:"wan"`
}

type LoginAttemptEventData struct {
	Success       bool   `json:"success"`
	Username      string `json:"username"`
	RemoteAddress string `json:"remoteAddress"`
}

type FailureEventData string
