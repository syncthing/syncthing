// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package connections

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/url"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/connections/registry"
	"github.com/syncthing/syncthing/lib/nat"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/stats"

	"github.com/thejerf/suture/v4"
)

type tlsConn interface {
	io.ReadWriteCloser
	ConnectionState() tls.ConnectionState
	RemoteAddr() net.Addr
	SetDeadline(time.Time) error
	SetWriteDeadline(time.Time) error
	LocalAddr() net.Addr
}

// internalConn is the raw TLS connection plus some metadata on where it
// came from (type, priority).
type internalConn struct {
	tlsConn
	connType      connType
	isLocal       bool
	priority      int
	establishedAt time.Time
	connectionID  string
}

type connType int

const (
	connTypeRelayClient connType = iota
	connTypeRelayServer
	connTypeTCPClient
	connTypeTCPServer
	connTypeQUICClient
	connTypeQUICServer
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
	case connTypeQUICClient:
		return "quic-client"
	case connTypeQUICServer:
		return "quic-server"
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
	case connTypeQUICClient, connTypeQUICServer:
		return "quic"
	default:
		return "unknown"
	}
}

func newInternalConn(tc tlsConn, connType connType, isLocal bool, priority int) internalConn {
	now := time.Now()
	return internalConn{
		tlsConn:       tc,
		connType:      connType,
		isLocal:       isLocal,
		priority:      priority,
		establishedAt: now.Truncate(time.Second),
		connectionID:  newConnectionID(tc, connType, now),
	}
}

// newConnectionID generates a connection ID. The connection ID is designed
// to be 1) unique for each connection (even those reusing the same socket
// address on both sides), 2) sortable so that the connection with the
// lowest ID will be the primary one. This also coincides with being the
// oldest connection.
func newConnectionID(tc tlsConn, connType connType, now time.Time) string {
	buf := binary.BigEndian.AppendUint64(nil, uint64(now.UnixNano()))
	hash := sha256.Sum224([]byte(fmt.Sprintf("%v-%v-%v", tc.LocalAddr(), connType.Transport(), tc.RemoteAddr())))
	buf = append(buf, hash[:]...)
	return base32.HexEncoding.WithPadding(base32.NoPadding).EncodeToString(buf)
}

func (c internalConn) Close() error {
	// *tls.Conn.Close() does more than it says on the tin. Specifically, it
	// sends a TLS alert message, which might block forever if the
	// connection is dead and we don't have a deadline set.
	_ = c.SetWriteDeadline(time.Now().Add(250 * time.Millisecond))
	return c.tlsConn.Close()
}

func (c internalConn) Type() string {
	return c.connType.String()
}

func (c internalConn) IsLocal() bool {
	return c.isLocal
}

func (c internalConn) Priority() int {
	return c.priority
}

func (c internalConn) Crypto() string {
	cs := c.ConnectionState()
	return fmt.Sprintf("%s-%s", tlsVersionNames[cs.Version], tlsCipherSuiteNames[cs.CipherSuite])
}

func (c internalConn) Transport() string {
	transport := c.connType.Transport()
	ip, err := osutil.IPFromAddr(c.LocalAddr())
	if err != nil {
		return transport
	}
	if ip.To4() != nil {
		return transport + "4"
	}
	return transport + "6"
}

func (c internalConn) EstablishedAt() time.Time {
	return c.establishedAt
}

func (c internalConn) ConnectionID() string {
	return c.connectionID
}

func (c internalConn) String() string {
	t := "WAN"
	if c.isLocal {
		t = "LAN"
	}
	return fmt.Sprintf("%s-%s/%s/%s/%s-P%d", c.LocalAddr(), c.RemoteAddr(), c.Type(), c.Crypto(), t, c.Priority())
}

type dialerFactory interface {
	New(config.OptionsConfiguration, *tls.Config, *registry.Registry, *lanChecker) genericDialer
	AlwaysWAN() bool
	Valid(config.Configuration) error
	String() string
}

type commonDialer struct {
	trafficClass      int
	reconnectInterval time.Duration
	tlsCfg            *tls.Config
	lanChecker        *lanChecker
	lanPriority       int
	wanPriority       int
	allowsMultiConns  bool
}

func (d *commonDialer) RedialFrequency() time.Duration {
	return d.reconnectInterval
}

func (d *commonDialer) Priority(host string) int {
	if d.lanChecker.isLANHost(host) {
		return d.lanPriority
	}
	return d.wanPriority
}

func (d *commonDialer) AllowsMultiConns() bool {
	return d.allowsMultiConns
}

type genericDialer interface {
	Dial(context.Context, protocol.DeviceID, *url.URL) (internalConn, error)
	RedialFrequency() time.Duration
	Priority(host string) int
	AllowsMultiConns() bool
}

type listenerFactory interface {
	New(*url.URL, config.Wrapper, *tls.Config, chan internalConn, *nat.Service, *registry.Registry, *lanChecker) genericListener
	Valid(config.Configuration) error
}

type ListenerAddresses struct {
	URI          *url.URL
	WANAddresses []*url.URL
	LANAddresses []*url.URL
}

type genericListener interface {
	suture.Service
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
	OnAddressesChanged(func(ListenerAddresses))
	String() string
	Factory() listenerFactory
	NATType() string
}

type Model interface {
	protocol.Model
	AddConnection(conn protocol.Connection, hello protocol.Hello)
	Connection(remoteID protocol.DeviceID) (protocol.Connection, bool)
	OnHello(protocol.DeviceID, net.Addr, protocol.Hello) error
	GetHello(protocol.DeviceID) protocol.HelloIntf
	DeviceStatistics() (map[protocol.DeviceID]stats.DeviceStatistics, error)
}

type onAddressesChangedNotifier struct {
	callbacks []func(ListenerAddresses)
}

func (o *onAddressesChangedNotifier) OnAddressesChanged(callback func(ListenerAddresses)) {
	o.callbacks = append(o.callbacks, callback)
}

func (o *onAddressesChangedNotifier) notifyAddressesChanged(l genericListener) {
	o.notifyAddresses(ListenerAddresses{
		URI:          l.URI(),
		WANAddresses: l.WANAddresses(),
		LANAddresses: l.LANAddresses(),
	})
}

func (o *onAddressesChangedNotifier) clearAddresses(l genericListener) {
	o.notifyAddresses(ListenerAddresses{
		URI: l.URI(),
	})
}

func (o *onAddressesChangedNotifier) notifyAddresses(l ListenerAddresses) {
	for _, callback := range o.callbacks {
		callback(l)
	}
}

type dialTarget struct {
	addr     string
	dialer   genericDialer
	priority int
	uri      *url.URL
	deviceID protocol.DeviceID
}

func (t dialTarget) Dial(ctx context.Context) (internalConn, error) {
	l.Debugln("dialing", t.deviceID, t.uri, "prio", t.priority)
	return t.dialer.Dial(ctx, t.deviceID, t.uri)
}
