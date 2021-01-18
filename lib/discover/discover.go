// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package discover

import (
	"context"
	"time"

	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/thejerf/suture/v4"
)

// A Finder provides lookup services of some kind.
type Finder interface {
	Lookup(ctx context.Context, deviceID protocol.DeviceID) (address []string, err error)
	Error() error
	String() string
	Cache() map[protocol.DeviceID]CacheEntry
}

type CacheEntry struct {
	Addresses  []string  `json:"addresses"`
	when       time.Time // When did we get the result
	found      bool      // Is it a success (cacheTime applies) or a failure (negCacheTime applies)?
	validUntil time.Time // Validity time, overrides normal calculation
	instanceID int64     // for local discovery, the instance ID (random on each restart)
}

// A FinderService is a Finder that has background activity and must be run as
// a suture.Service.
type FinderService interface {
	Finder
	suture.Service
}

// The AddressLister answers questions about what addresses we are listening
// on.
type AddressLister interface {
	ExternalAddresses() []string
	AllAddresses() []string
}
