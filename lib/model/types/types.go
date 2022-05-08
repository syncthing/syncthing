// Copyright (C) 2022 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package types

import (
	"time"
)

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

// FolderSummary replaces the previously used map[string]interface{}, and needs
// to keep the structure/naming for api backwards compatibility
type FolderSummary struct {
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

type FolderCompletion struct {
	CompletionPct float64           `json:"completion"`
	GlobalBytes   int64             `json:"globalBytes"`
	NeedBytes     int64             `json:"needBytes"`
	GlobalItems   int               `json:"globalItems"`
	NeedItems     int               `json:"needItems"`
	NeedDeletes   int               `json:"needDeletes"`
	Sequence      int64             `json:"sequence"`
	RemoteState   RemoteFolderState `json:"remoteState"`
}

func (comp *FolderCompletion) Add(other FolderCompletion) {
	comp.GlobalBytes += other.GlobalBytes
	comp.NeedBytes += other.NeedBytes
	comp.GlobalItems += other.GlobalItems
	comp.NeedItems += other.NeedItems
	comp.NeedDeletes += other.NeedDeletes
	comp.SetComplectionPct()
}

func (comp *FolderCompletion) SetComplectionPct() {
	if comp.GlobalBytes == 0 {
		comp.CompletionPct = 100
	} else {
		needRatio := float64(comp.NeedBytes) / float64(comp.GlobalBytes)
		comp.CompletionPct = 100 * (1 - needRatio)
	}

	// If the completion is 100% but there are deletes we need to handle,
	// drop it down a notch. Hack for consumers that look only at the
	// percentage (our own GUI does the same calculation as here on its own
	// and needs the same fixup).
	if comp.NeedBytes == 0 && comp.NeedDeletes > 0 {
		comp.CompletionPct = 95 // chosen by fair dice roll
	}
}

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
