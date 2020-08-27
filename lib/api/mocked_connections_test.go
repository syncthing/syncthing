// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package api

import (
	"context"

	"github.com/syncthing/syncthing/lib/connections"
)

type mockedConnections struct{}

func (m *mockedConnections) ListenerStatus() map[string]connections.ListenerStatusEntry {
	return nil
}

func (m *mockedConnections) ConnectionStatus() map[string]connections.ConnectionStatusEntry {
	return nil
}

func (m *mockedConnections) NATType() string {
	return ""
}

func (m *mockedConnections) Serve(ctx context.Context) error { return nil }

func (m *mockedConnections) ExternalAddresses() []string { return nil }

func (m *mockedConnections) AllAddresses() []string { return nil }
