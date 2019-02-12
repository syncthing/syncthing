// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package api

import (
	"time"

	"github.com/syncthing/syncthing/lib/events"
)

type mockedEventSub struct{}

func (s *mockedEventSub) Since(id int, into []events.Event, timeout time.Duration) []events.Event {
	select {}
}
