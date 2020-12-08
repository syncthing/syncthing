// Copyright (C) 2020 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// +build noquic go1.16

package connections

import (
	"crypto/tls"
	"net/url"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/nat"
)

func init() {
	factory := &quicUnsupportedListenerFactory{}
	for _, scheme := range []string{"quic", "quic4", "quic6"} {
		listeners[scheme] = factory
	}
}

type quicUnsupportedListenerFactory struct{}

func (quicUnsupportedListenerFactory) Valid(config.Configuration) error {
	return errNotIncluded
}

func (quicUnsupportedListenerFactory) New(_ *url.URL, _ config.Wrapper, _ *tls.Config, _ chan internalConn, _ *nat.Service) genericListener {
	panic("should never be called")
}

func (quicUnsupportedListenerFactory) Enabled(_ config.Configuration) bool {
	return false
}

func init() {
	factory := &quicUnsupportedDialerFactory{}
	for _, scheme := range []string{"quic", "quic4", "quic6"} {
		dialers[scheme] = factory
	}
}

type quicUnsupportedDialerFactory struct {
}

func (quicUnsupportedDialerFactory) New(_ config.OptionsConfiguration, _ *tls.Config) genericDialer {
	panic("should never be called")
}

func (quicUnsupportedDialerFactory) Priority() int {
	return 0
}

func (quicUnsupportedDialerFactory) AlwaysWAN() bool {
	return false
}

func (quicUnsupportedDialerFactory) Valid(_ config.Configuration) error {
	return errNotIncluded
}

func (quicUnsupportedDialerFactory) String() string {
	return "QUIC Dialer"
}
