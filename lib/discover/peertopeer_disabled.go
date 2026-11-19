// Copyright (C) 2026 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build nolibp2p

package discover

import (
	"crypto/tls"
	"errors"

	"github.com/syncthing/syncthing/internal/db"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/connections/registry"
	"github.com/syncthing/syncthing/lib/events"
)

const p2pDiscoveryAvailable = false

func NewPeerToPeerDiscovery(tls.Certificate, AddressLister, events.Logger, db.KV, *registry.UDPReuseManager, config.OptionsConfiguration) (Finder, error) {
	return nil, errors.New("peer-to-peer discovery is disabled in this build")
}
