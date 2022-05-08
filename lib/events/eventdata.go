// Copyright (C) 2022 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package events

import (
	"net/url"
	"time"

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

type DownloadProgressEventData map[string]map[string]*PullerProgress

// A momentary state representing the progress of the puller
type PullerProgress struct {
	Total                   int   `json:"total"`
	Reused                  int   `json:"reused"`
	CopiedFromOrigin        int   `json:"copiedFromOrigin"`
	CopiedFromOriginShifted int   `json:"copiedFromOriginShifted"`
	CopiedFromElsewhere     int   `json:"copiedFromElsewhere"`
	Pulled                  int   `json:"pulled"`
	Pulling                 int   `json:"pulling"`
	BytesDone               int64 `json:"bytesDone"`
	BytesTotal              int64 `json:"bytesTotal"`
}

type RemoteDownloadProgressEventData struct {
	Device protocol.DeviceID `json:"device"`
	Folder string            `json:"folder"`
	State  map[string]int    `json:"state"`
}

type FolderSummaryEventData struct {
	Folder  string              `json:"folder"`
	Summary FolderSummaryFields `json:"summary"`
}

// FolderSummaryFields replaces the previously used map[string]interface{}, and
// needs to keep the structure/naming for api backwards compatibility
type FolderSummaryFields struct {
	Errors     int `json:"errors"`
	PullErrors int `json:"pullErrors"` // deprecated

	Invalid string `json:"invalid"` // deprecated

	GlobalFiles       int   `json:"globalFiles"`
	GlobalDirectories int   `json:"globalDirectories"`
	GlobalSymlinks    int   `json:"globalSymlinks"`
	GlobalDeleted     int   `json:"globalDeleted"`
	GlobalBytes       int64 `json:"globalBytes"`
	GlobalTotalItems  int   `json:"globalTotalItems"`

	LocalFiles       int   `json:"localFiles"`
	LocalDirectories int   `json:"localDirectories"`
	LocalSymlinks    int   `json:"localSymlinks"`
	LocalDeleted     int   `json:"localDeleted"`
	LocalBytes       int64 `json:"localBytes"`
	LocalTotalItems  int   `json:"localTotalItems"`

	NeedFiles       int   `json:"needFiles"`
	NeedDirectories int   `json:"needDirectories"`
	NeedSymlinks    int   `json:"needSymlinks"`
	NeedDeletes     int   `json:"needDeletes"`
	NeedBytes       int64 `json:"needBytes"`
	NeedTotalItems  int   `json:"needTotalItems"`

	ReceiveOnlyChangedFiles       int   `json:"receiveOnlyChangedFiles"`
	ReceiveOnlyChangedDirectories int   `json:"receiveOnlyChangedDirectories"`
	ReceiveOnlyChangedSymlinks    int   `json:"receiveOnlyChangedSymlinks"`
	ReceiveOnlyChangedDeletes     int   `json:"receiveOnlyChangedDeletes"`
	ReceiveOnlyChangedBytes       int64 `json:"receiveOnlyChangedBytes"`
	ReceiveOnlyTotalItems         int   `json:"receiveOnlyTotalItems"`

	InSyncFiles int   `json:"inSyncFiles"`
	InSyncBytes int64 `json:"inSyncBytes"`

	State        string    `json:"state"`
	StateChanged time.Time `json:"stateChanged"`
	Error        string    `json:"error"`

	Version  int64 `json:"version"` // deprecated
	Sequence int64 `json:"sequence"`

	IgnorePatterns bool   `json:"ignorePatterns"`
	WatchError     string `json:"watchError"`
}

type FolderCompletionEventData struct {
	Folder string            `json:"folder"`
	Device protocol.DeviceID `json:"device"`
	FolderCompletionFields
}

type FolderCompletionFields struct {
	CompletionPct float64           `json:"completion"`
	GlobalBytes   int64             `json:"globalBytes"`
	NeedBytes     int64             `json:"needBytes"`
	GlobalItems   int               `json:"globalItems"`
	NeedItems     int               `json:"needItems"`
	NeedDeletes   int               `json:"needDeletes"`
	Sequence      int64             `json:"sequence"`
	RemoteState   RemoteFolderState `json:"remoteState"`
}

type RemoteFolderState int

const (
	RemoteFolderUnknown RemoteFolderState = iota
	RemoteFolderNotSharing
	RemoteFolderPaused
	RemoteFolderValid
)

func (s RemoteFolderState) String() string {
	switch s {
	case RemoteFolderUnknown:
		return "unknown"
	case RemoteFolderNotSharing:
		return "notSharing"
	case RemoteFolderPaused:
		return "paused"
	case RemoteFolderValid:
		return "valid"
	default:
		return "unknown"
	}
}

func (s RemoteFolderState) MarshalText() ([]byte, error) {
	return []byte(s.String()), nil
}

type FolderErrorsEventData struct {
	Folder string      `json:"folder"`
	Errors []FileError `json:"errors,omitempty"`
}

// A []FileError is sent as part of an event and will be JSON serialized.
type FileError struct {
	Path string `json:"path"`
	Err  string `json:"error"`
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
