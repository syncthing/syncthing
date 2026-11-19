// Copyright (C) 2026 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build !dhtdiagnostics

package dht

import (
	"github.com/libp2p/go-libp2p/core/peer"
)

func setupLibraryLogging()                                {}
func (s *DHT) startDiagnostics(bootstrap []peer.AddrInfo) {}
