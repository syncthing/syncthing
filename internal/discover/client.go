// Copyright (C) 2014 The Syncthing Authors.
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

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
	Lookup(device protocol.DeviceID) []string
	StatusOK() bool
	Address() string
	Stop()
}
