// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package connections

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/url"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/nat"
	"github.com/syncthing/syncthing/lib/protocol"
)

// Connection is what we expose to the outside. It is a protocol.Connection
// that can be closed and has some metadata.
type Connection interface {
	protocol.Connection
	io.Closer
	Type() string
	Transport() string
	RemoteAddr() net.Addr
	Priority() int
	String() string
}

// completeConn is the aggregation of an internalConn and the
// protocol.Connection running on top of it. It implements the Connection
// interface.
type completeConn struct {
	internalConn
	protocol.Connection
}

// internalConn is the raw TLS connection plus some metadata on where it
// came from (type, priority).
type internalConn struct {
	*tls.Conn
	connType connType
	priority int
}

type connType int

const (
	connTypeRelayClient connType = iota
	connTypeRelayServer
	connTypeTCPClient
	connTypeTCPServer
)

func (t connType) String() string {
	switch t {
	case connTypeRelayClient:
		return "relay-client"
	case connTypeRelayServer:
		return "relay-server"
	case connTypeTCPClient:
		return "tcp-client"
	case connTypeTCPServer:
		return "tcp-server"
	default:
		return "unknown-type"
	}
}

func (t connType) Transport() string {
	switch t {
	case connTypeRelayClient, connTypeRelayServer:
		return "relay"
	case connTypeTCPClient, connTypeTCPServer:
		return "tcp"
	default:
		return "unknown"
	}
}

func (c internalConn) Type() string {
	return c.connType.String()
}

func (c internalConn) Priority() int {
	return c.priority
}

func (c internalConn) Transport() string {
	transport := c.connType.Transport()
	host, _, err := net.SplitHostPort(c.LocalAddr().String())
	if err != nil {
		return transport
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return transport
	}
	if ip.To4() != nil {
		return transport + "4"
	}
	return transport + "6"
}

func (c internalConn) String() string {
	return fmt.Sprintf("%s-%s/%s", c.LocalAddr(), c.RemoteAddr(), c.Type())
}

type dialerFactory interface {
	New(*config.Wrapper, *tls.Config) genericDialer
	Priority() int
	AlwaysWAN() bool
	Valid(config.Configuration) error
	String() string
}

type genericDialer interface {
	Dial(protocol.DeviceID, *url.URL) (internalConn, error)
	RedialFrequency() time.Duration
}

type listenerFactory interface {
	New(*url.URL, *config.Wrapper, *tls.Config, chan internalConn, *nat.Service) genericListener
	Valid(config.Configuration) error
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
	NATType() string
}

type Model interface {
	protocol.Model
	AddConnection(conn Connection, hello protocol.HelloResult)
	Connection(remoteID protocol.DeviceID) (Connection, bool)
	OnHello(protocol.DeviceID, net.Addr, protocol.HelloResult) error
	GetHello(protocol.DeviceID) protocol.HelloIntf
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

type dialTarget struct {
	dialer   genericDialer
	priority int
	uri      *url.URL
	deviceID protocol.DeviceID
}

func (t dialTarget) Dial() (internalConn, error) {
	l.Debugln("dialing", t.deviceID, t.uri, "prio", t.priority)
	conn, err := t.dialer.Dial(t.deviceID, t.uri)
	if err != nil {
		l.Debugln("dialing", t.deviceID, t.uri, "error:", err)
	} else {
		l.Debugln("dialing", t.deviceID, t.uri, "success:", conn)
	}
	return conn, err
}
