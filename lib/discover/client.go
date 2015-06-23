// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package discover

import (
	"fmt"
	"net/url"
	"time"

	"github.com/syncthing/protocol"
)

type Factory func(*url.URL, *Announce) (Client, error)

var (
	factories                      = make(map[string]Factory)
	DefaultErrorRetryInternval     = 60 * time.Second
	DefaultGlobalBroadcastInterval = 1800 * time.Second
)

func Register(proto string, factory Factory) {
	factories[proto] = factory
}

func New(addr string, pkt *Announce) (Client, error) {
	uri, err := url.Parse(addr)
	if err != nil {
		return nil, err
	}
	factory, ok := factories[uri.Scheme]
	if !ok {
		return nil, fmt.Errorf("Unsupported scheme: %s", uri.Scheme)
	}
	client, err := factory(uri, pkt)
	if err != nil {
		return nil, err
	}
	return client, nil
}

type Client interface {
	Lookup(device protocol.DeviceID) (Announce, error)
	StatusOK() bool
	Address() string
	Stop()
}
