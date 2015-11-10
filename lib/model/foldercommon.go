// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package model

import (
	"errors"
	"os"
	"time"

	"github.com/syncthing/syncthing/lib/protocol"
)

const (
	dbUpdateHandleDir = iota
	dbUpdateDeleteDir
	dbUpdateHandleFile
	dbUpdateDeleteFile
	dbUpdateShortcutFile
)

const (
	defaultCopiers     = 1
	defaultPullers     = 16
	defaultPullerSleep = 10 * time.Second
	defaultPullerPause = 60 * time.Second
)

var (
	activity    = newDeviceActivity()
	errNoDevice = errors.New("peers who had this file went away, or the file has changed while syncing. will retry later")
)

type dbUpdateJob struct {
	file    protocol.FileInfo
	jobType int
}

type rescanRequest struct {
	subs []string
	err  chan error
}

// A pullBlockState is passed to the puller routine for each block that needs
// to be fetched.
type pullBlockState struct {
	*sharedPullerState
	block protocol.BlockInfo
}

// A copyBlocksState is passed to copy routine if the file has blocks to be
// copied.
type copyBlocksState struct {
	*sharedPullerState
	blocks []protocol.BlockInfo
}

// Which filemode bits to preserve
const retainBits = os.ModeSetgid | os.ModeSetuid | os.ModeSticky

// A []fileError is sent as part of an event and will be JSON serialized.
type fileError struct {
	Path string `json:"path"`
	Err  string `json:"error"`
}

type fileErrorList []fileError

func (l fileErrorList) Len() int {
	return len(l)
}

func (l fileErrorList) Less(a, b int) bool {
	return l[a].Path < l[b].Path
}

func (l fileErrorList) Swap(a, b int) {
	l[a], l[b] = l[b], l[a]
}

// FIXME What is it doing here? What was it doing in defaultfolder.go?
func removeDevice(devices []protocol.DeviceID, device protocol.DeviceID) []protocol.DeviceID {
	for i := range devices {
		if devices[i] == device {
			devices[i] = devices[len(devices)-1]
			return devices[:len(devices)-1]
		}
	}
	return devices
}
