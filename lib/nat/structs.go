// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package nat

import (
	"fmt"
	"net"
	"time"

	"github.com/syncthing/syncthing/lib/sync"
)

type Mapping struct {
	// Local ip/port
	protocol Protocol
	ip       net.IP
	port     int

	addresses map[string]Address // NAT ID -> Address
	expires   time.Time
	mut       sync.RWMutex
}

func (m *Mapping) setAddress(natID string, address Address) {
	m.mut.Lock()
	if existing, ok := m.addresses[natID]; !ok || !existing.Equal(address) {
		l.Infof("New NAT port mapping: external address %s to local port %d.", address, m.port)
		m.addresses[natID] = address
	}
	m.mut.Unlock()
}

func (m *Mapping) removeAddress(natID string) {
	m.mut.Lock()
	addr, ok := m.addresses[natID]
	if ok {
		l.Debugf("Removing port mapping %s to %s, as NAT %s is no longer available.", m.port, addr, natID)
		delete(m.addresses, natID)
	}
	m.mut.Unlock()
}

func (m *Mapping) notify() {
	// TODO: AUD implement (not sure if we need this)
}

func (m *Mapping) addressMap() map[string]Address {
	m.mut.RLock()
	addrMap := m.addresses
	m.mut.RUnlock()
	return addrMap
}

func (m *Mapping) ExternalAddresses() []Address {
	m.mut.RLock()
	addrs := make([]Address, len(m.addresses))
	for _, addr := range m.addresses {
		addrs = append(addrs, addr)
	}
	m.mut.RUnlock()
	return addrs
}

func (m *Mapping) validGateway(ip net.IP) bool {
	if m.ip == nil || ip == nil || m.ip.IsUnspecified() || ip.IsUnspecified() {
		return true
	}
	return m.ip.Equal(ip)
}

type Address struct {
	IP   net.IP
	Port int
}

func (a Address) Equal(b Address) bool {
	return a.Port == b.Port && a.IP.Equal(b.IP)
}

func (a Address) String() string {
	ipStr := ""
	if a.IP != nil && a.IP.IsUnspecified() {
		ipStr = a.IP.String()
	}
	return net.JoinHostPort(ipStr, fmt.Sprintf("%d", a.Port))
}

func (a Address) GoString() string {
	return a.String()
}
