// Copyright (C) 2018 The Syncthing Authors.
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
	Time  time.Time `xml:"time,attr" json:"time"`
	ID    string    `xml:"id,attr" json:"id"`
	Label string    `xml:"label,attr" json:"label"`
}

type ObservedDevice struct {
	Time    time.Time         `xml:"time,attr" json:"time"`
	ID      protocol.DeviceID `xml:"id,attr" json:"deviceID"`
	Name    string            `xml:"name,attr" json:"name"`
	Address string            `xml:"address,attr" json:"address"`
}
