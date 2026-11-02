// Copyright (C) 2026 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// Package udpmux shares UDP sockets between QUIC protocols and non-QUIC traffic.
package udpmux

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"net"
	"net/netip"
	"slices"
	"sync"

	"github.com/libp2p/go-libp2p/p2p/transport/quicreuse"
	"github.com/quic-go/quic-go"
)

type Manager struct {
	mut    sync.Mutex
	cm     *quicreuse.ConnManager
	leases map[*Lease]map[netip.AddrPort][]netip.AddrPort
}

func (m *Manager) Acquire() (*Lease, error) {
	m.mut.Lock()
	defer m.mut.Unlock()
	if m.cm == nil {
		var resetKey quic.StatelessResetKey
		var tokenKey quic.TokenGeneratorKey
		if _, err := rand.Read(resetKey[:]); err != nil {
			return nil, err
		}
		if _, err := rand.Read(tokenKey[:]); err != nil {
			return nil, err
		}
		cm, err := quicreuse.NewConnManager(resetKey, tokenKey, quicreuse.OverrideListenUDP(listenUDP))
		if err != nil {
			return nil, err
		}
		m.cm = cm
		m.leases = make(map[*Lease]map[netip.AddrPort][]netip.AddrPort)
	}
	lease := &Lease{manager: m}
	m.leases[lease] = make(map[netip.AddrPort][]netip.AddrPort)
	return lease, nil
}

// Lease keeps the manager alive. Close its connections and listeners before
// releasing it; the last lease closes the manager's sockets.
type Lease struct {
	manager *Manager
	socket  net.PacketConn // Optional reference keeping an accepted connection's socket alive.
	once    sync.Once
}

// QUICReuse is the constructor bridge for libp2p, whose transport requires this
// concrete type. Endpoint and mapping operations use the methods below.
func (l *Lease) QUICReuse() *quicreuse.ConnManager {
	return l.manager.cm
}

type Listener interface {
	Accept(context.Context) (*quic.Conn, error)
	Addr() net.Addr
	Close() error
}

func (l *Lease) Listen(local netip.AddrPort, cfg *tls.Config) (Listener, error) {
	addr, err := quicreuse.ToQuicMultiaddr(net.UDPAddrFromAddrPort(local), quic.Version1)
	if err != nil {
		return nil, err
	}
	return l.manager.cm.ListenQUIC(addr, cfg, func(*quic.Conn, uint64) bool { return true })
}

func (l *Lease) PacketConn(local netip.AddrPort) (net.PacketConn, error) {
	return l.manager.cm.SharedNonQUICPacketConn(udpNetwork(local), net.UDPAddrFromAddrPort(local))
}

func (l *Lease) Dial(ctx context.Context, remote netip.AddrPort, tlsCfg *tls.Config, cfg *quic.Config) (*quic.Conn, error) {
	addr := net.UDPAddrFromAddrPort(remote)
	transport, err := l.manager.cm.TransportForDial(udpNetwork(remote), addr)
	if err != nil {
		return nil, err
	}
	conn, err := transport.Dial(ctx, addr, tlsCfg, cfg)
	if err != nil {
		transport.DecreaseCount()
		return nil, err
	}
	go func() {
		<-conn.Context().Done()
		transport.DecreaseCount()
	}()
	return conn, nil
}

// Retain keeps the actual socket alive for an accepted connection, even after
// its protocol listener is closed. The packet connection is only a reference;
// it does not read packets or compete with STUN.
func (l *Lease) Retain(local netip.AddrPort) (*Lease, error) {
	socket, err := l.PacketConn(local)
	if err != nil {
		return nil, err
	}
	retained, err := l.manager.Acquire()
	if err != nil {
		socket.Close()
		return nil, err
	}
	retained.socket = socket
	return retained, nil
}

// SetExternalAddresses publishes current router mappings for a local endpoint.
func (l *Lease) SetExternalAddresses(local netip.AddrPort, addresses []netip.AddrPort) {
	m := l.manager
	m.mut.Lock()
	defer m.mut.Unlock()
	if mappings, ok := m.leases[l]; ok {
		if len(addresses) == 0 {
			delete(mappings, endpoint(local))
		} else {
			addresses = slices.Clone(addresses)
			for i, address := range addresses {
				addresses[i] = endpoint(address)
			}
			mappings[endpoint(local)] = addresses
		}
	}
}

// ExternalAddresses returns mappings for the specified local listening endpoints.
func (l *Lease) ExternalAddresses(locals ...netip.AddrPort) []netip.AddrPort {
	m := l.manager
	m.mut.Lock()
	defer m.mut.Unlock()
	var addresses []netip.AddrPort
	for _, mappings := range m.leases {
		for _, local := range locals {
			addresses = append(addresses, mappings[endpoint(local)]...)
		}
	}
	slices.SortFunc(addresses, netip.AddrPort.Compare)
	return slices.Compact(addresses)
}

func (l *Lease) Close() error {
	var err error
	l.once.Do(func() {
		if l.socket != nil {
			l.socket.Close()
		}
		m := l.manager
		m.mut.Lock()
		defer m.mut.Unlock()
		delete(m.leases, l)
		if len(m.leases) == 0 {
			err = m.cm.Close()
			m.cm = nil
		}
	})
	return err
}

func udpNetwork(addr netip.AddrPort) string {
	if addr.Addr().Unmap().Is4() {
		return "udp4"
	}
	return "udp6"
}

func endpoint(addr netip.AddrPort) netip.AddrPort {
	return netip.AddrPortFrom(addr.Addr().Unmap(), addr.Port())
}
