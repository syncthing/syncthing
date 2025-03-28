// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package config

import (
	"time"

	"github.com/syncthing/syncthing/lib/protocol"
)

type ObservedFolder struct {
	Time  time.Time `json:"time" xml:"time,attr"`
	ID    string    `json:"id" xml:"id,attr"`
	Label string    `json:"label" xml:"label,attr"`
}

type ObservedDevice struct {
	Time    time.Time         `json:"time" xml:"time,attr"`
	ID      protocol.DeviceID `json:"deviceID" xml:"id,attr"`
	Name    string            `json:"name" xml:"name,attr"`
	Address string            `json:"address" xml:"address,attr"`
}
