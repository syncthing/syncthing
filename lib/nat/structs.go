// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package nat

import (
	"fmt"
	"net"
	"time"

	"github.com/syncthing/syncthing/lib/sync"
)

type MappingChangeSubscriber func()

type Mapping struct {
	protocol Protocol
	address  Address

	extAddresses map[string][]Address // NAT ID -> Address
	expires      time.Time
	subscribers  []MappingChangeSubscriber
	mut          sync.RWMutex
}

func (m *Mapping) setAddressLocked(id string, addresses []Address) {
	l.Infof("New external port opened: external %s address(es) %v to local address %s.", m.protocol, addresses, m.address)
	m.extAddresses[id] = addresses
}

func (m *Mapping) removeAddressLocked(id string) {
	addresses, ok := m.extAddresses[id]
	if ok {
		l.Infof("Removing external open port: %s address(es) %v for gateway %s.", m.protocol, addresses, id)
		delete(m.extAddresses, id)
	}
}

func (m *Mapping) clearAddresses() {
	m.mut.Lock()
	change := len(m.extAddresses) > 0
	for id, addr := range m.extAddresses {
		l.Debugf("Clearing mapping %s: ID: %s Address: %s", m, id, addr)
		delete(m.extAddresses, id)
	}
	m.expires = time.Time{}
	m.mut.Unlock()
	if change {
		m.notify()
	}
}

func (m *Mapping) notify() {
	m.mut.RLock()
	for _, subscriber := range m.subscribers {
		subscriber()
	}
	m.mut.RUnlock()
}

func (m *Mapping) Protocol() Protocol {
	return m.protocol
}

func (m *Mapping) Address() Address {
	return m.address
}

func (m *Mapping) ExternalAddresses() []Address {
	m.mut.RLock()
	addrs := make([]Address, 0, len(m.extAddresses))
	for _, addr := range m.extAddresses {
		addrs = append(addrs, addr...)
	}
	m.mut.RUnlock()
	return addrs
}

func (m *Mapping) OnChanged(subscribed MappingChangeSubscriber) {
	m.mut.Lock()
	m.subscribers = append(m.subscribers, subscribed)
	m.mut.Unlock()
}

func (m *Mapping) String() string {
	return fmt.Sprintf("%s/%s", m.address, m.protocol)
}

func (m *Mapping) GoString() string {
	return m.String()
}

// Checks if the mappings local IP address matches the IP address of the gateway
// For example, if we are explicitly listening on 192.168.0.12, there is no
// point trying to acquire a mapping on a gateway to which the local IP is
// 10.0.0.1. Fallback to true if any of the IPs is not there.
func (m *Mapping) validGateway(ip net.IP) bool {
	if m.address.IP == nil || ip == nil || m.address.IP.IsUnspecified() || ip.IsUnspecified() {
		return true
	}
	return m.address.IP.Equal(ip)
}

// Address is essentially net.TCPAddr yet is more general, and has a few helper
// methods which reduce boilerplate code.
type Address struct {
	IP   net.IP
	Port int
}

func (a Address) Equal(b Address) bool {
	return a.Port == b.Port && a.IP.Equal(b.IP)
}

func (a Address) String() string {
	var ipStr string
	if a.IP == nil {
		ipStr = net.IPv4zero.String()
	} else {
		ipStr = a.IP.String()
	}
	return net.JoinHostPort(ipStr, fmt.Sprintf("%d", a.Port))
}

func (a Address) GoString() string {
	return a.String()
}
