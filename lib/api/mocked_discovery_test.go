// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package api

import (
	"context"

	"github.com/syncthing/syncthing/lib/discover"
	"github.com/syncthing/syncthing/lib/protocol"
)

type mockedCachingMux struct{}

// from suture.Service

func (m *mockedCachingMux) Serve(ctx context.Context) error {
	select {}
}

// from events.Finder

func (m *mockedCachingMux) Lookup(ctx context.Context, deviceID protocol.DeviceID) (direct []string, err error) {
	return nil, nil
}

func (m *mockedCachingMux) Error() error {
	return nil
}

func (m *mockedCachingMux) String() string {
	return "mockedCachingMux"
}

func (m *mockedCachingMux) Cache() map[protocol.DeviceID]discover.CacheEntry {
	return nil
}

// from events.Manager

func (m *mockedCachingMux) ChildErrors() map[string]error {
	return nil
}
