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
	mut  sync.Mutex
	cm   *quicreuse.ConnManager
	exts map[netip.AddrPort][]netip.AddrPort
}

func NewManager() *Manager {
	var resetKey quic.StatelessResetKey
	var tokenKey quic.TokenGeneratorKey
	if _, err := rand.Read(resetKey[:]); err != nil {
		panic("can't happen: random failure")
	}
	if _, err := rand.Read(tokenKey[:]); err != nil {
		panic("can't happen: random failure")
	}

	// NewConnManager only returns an error for bad options
	cm, _ := quicreuse.NewConnManager(resetKey, tokenKey, quicreuse.OverrideListenUDP(listenUDP))
	return &Manager{
		cm:   cm,
		exts: make(map[netip.AddrPort][]netip.AddrPort),
	}
}

func (m *Manager) Dial(ctx context.Context, remote netip.AddrPort, tlsCfg *tls.Config, cfg *quic.Config) (*quic.Conn, error) {
	addr := net.UDPAddrFromAddrPort(remote)
	transport, err := m.cm.TransportForDial(udpNetwork(remote), addr)
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

func (m *Manager) Listen(local netip.AddrPort, cfg *tls.Config) (Listener, error) {
	addr, err := quicreuse.ToQuicMultiaddr(net.UDPAddrFromAddrPort(local), quic.Version1)
	if err != nil {
		return nil, err
	}
	return m.cm.ListenQUIC(addr, cfg, func(*quic.Conn, uint64) bool { return true })
}

func (m *Manager) PacketConn(local netip.AddrPort) (net.PacketConn, error) {
	return m.cm.SharedNonQUICPacketConn(udpNetwork(local), net.UDPAddrFromAddrPort(local))
}

func (m *Manager) QUICReuse() *quicreuse.ConnManager {
	return m.cm
}

type Listener interface {
	Accept(context.Context) (*quic.Conn, error)
	Addr() net.Addr
	Close() error
}

func (m *Manager) SetExternalAddresses(local netip.AddrPort, addresses []netip.AddrPort) {
	m.mut.Lock()
	defer m.mut.Unlock()
	m.exts[local] = addresses
}

func (m *Manager) ExternalAddresses(locals ...netip.AddrPort) []netip.AddrPort {
	m.mut.Lock()
	defer m.mut.Unlock()
	var addresses []netip.AddrPort
	for _, local := range locals {
		addresses = append(addresses, m.exts[local]...)
	}
	slices.SortFunc(addresses, netip.AddrPort.Compare)
	return slices.Compact(addresses)
}

func udpNetwork(addr netip.AddrPort) string {
	if addr.Addr().Unmap().Is4() {
		return "udp4"
	}
	return "udp6"
}
