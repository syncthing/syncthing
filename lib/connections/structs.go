// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package connections

import (
	"crypto/tls"
	"net"
	"net/url"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/nat"
	"github.com/syncthing/syncthing/lib/protocol"
)

type IntermediateConnection struct {
	*tls.Conn
	Type     string
	Priority int
}

type Connection struct {
	IntermediateConnection
	protocol.Connection
}

type dialerFactory interface {
	New(*config.Wrapper, *tls.Config) genericDialer
	Priority() int
	Enabled(config.Configuration) bool
	String() string
}

type genericDialer interface {
	Dial(protocol.DeviceID, *url.URL) (IntermediateConnection, error)
	RedialFrequency() time.Duration
}

type listenerFactory interface {
	New(*url.URL, *config.Wrapper, *tls.Config, chan IntermediateConnection, *nat.Service) genericListener
	Enabled(config.Configuration) bool
}

type genericListener interface {
	Serve()
	Stop()
	URI() *url.URL
	// A given address can potentially be mutated by the listener.
	// For example we bind to tcp://0.0.0.0, but that for example might return
	// tcp://gateway1.ip and tcp://gateway2.ip as WAN addresses due to there
	// being multiple gateways, and us managing to get a UPnP mapping on both
	// and tcp://192.168.0.1 and tcp://10.0.0.1 due to there being multiple
	// network interfaces. (The later case for LAN addresses is made up just
	// to provide an example)
	WANAddresses() []*url.URL
	LANAddresses() []*url.URL
	Error() error
	OnAddressesChanged(func(genericListener))
	String() string
	Factory() listenerFactory
}

type Model interface {
	protocol.Model
	AddConnection(conn Connection, hello protocol.HelloMessage)
	ConnectedTo(remoteID protocol.DeviceID) bool
	IsPaused(remoteID protocol.DeviceID) bool
	OnHello(protocol.DeviceID, net.Addr, protocol.HelloMessage)
	GetHello(protocol.DeviceID) protocol.HelloMessage
}

// serviceFunc wraps a function to create a suture.Service without stop
// functionality.
type serviceFunc func()

func (f serviceFunc) Serve() { f() }
func (f serviceFunc) Stop()  {}

type onAddressesChangedNotifier struct {
	callbacks []func(genericListener)
}

func (o *onAddressesChangedNotifier) OnAddressesChanged(callback func(genericListener)) {
	o.callbacks = append(o.callbacks, callback)
}

func (o *onAddressesChangedNotifier) notifyAddressesChanged(l genericListener) {
	for _, callback := range o.callbacks {
		callback(l)
	}
}
