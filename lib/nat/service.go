// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package nat

import (
	"fmt"
	"math/rand"
	"net"
	stdsync "sync"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/sync"
)

// Service runs a loop for discovery of IGDs (Internet Gateway Devices) and
// setup/renewal of a port mapping.
type Service struct {
	id        protocol.DeviceID
	cfg       *config.Wrapper
	stop      chan struct{}
	immediate chan chan struct{}
	timer     *time.Timer
	announce  *stdsync.Once

	mappings []*Mapping
	mut      sync.RWMutex
}

func NewService(id protocol.DeviceID, cfg *config.Wrapper) *Service {
	return &Service{
		id:  id,
		cfg: cfg,

		immediate: make(chan chan struct{}),
		timer:     time.NewTimer(time.Second),

		mut: sync.NewRWMutex(),
	}
}

func (s *Service) Serve() {
	s.timer.Reset(0)
	s.stop = make(chan struct{})
	s.announce = &stdsync.Once{}

	for {
		select {
		case result := <-s.immediate:
			s.process()
			close(result)
		case <-s.timer.C:
			s.process()
		case <-s.stop:
			s.timer.Stop()
			return
		}
	}
}

func (s *Service) process() {
	// toRenew are mappings which are due for renewal
	// toUpdate are the remaining mappings, which will only be updated if one of
	// the old IGDs has gone away, or a new IGD has appeared, but only if we
	// actually need to perform a renewal.
	var toRenew, toUpdate []*Mapping

	renewIn := time.Duration(s.cfg.Options().NATRenewalM) * time.Minute
	if renewIn == 0 {
		// We always want to do renewal so lets just pick a nice sane number.
		renewIn = 30 * time.Minute
	}

	s.mut.RLock()
	for _, mapping := range s.mappings {
		if mapping.expires.Before(time.Now()) {
			toRenew = append(toRenew, mapping)
		} else {
			toUpdate = append(toUpdate, mapping)
			mappingRenewIn := mapping.expires.Sub(time.Now())
			if mappingRenewIn < renewIn {
				renewIn = mappingRenewIn
			}
		}
	}
	s.mut.RUnlock()

	s.timer.Reset(renewIn)

	// Don't do anything, unless we really need to renew
	if len(toRenew) == 0 {
		return
	}

	nats := discoverAll(time.Duration(s.cfg.Options().NATTimeoutS) * time.Second)

	s.announce.Do(func() {
		suffix := "s"
		if len(nats) == 1 {
			suffix = ""
		}
		l.Infoln("Detected", len(nats), "NAT device"+suffix)
	})

	for _, mapping := range toRenew {
		s.updateMapping(mapping, nats, true)
	}

	for _, mapping := range toUpdate {
		s.updateMapping(mapping, nats, false)
	}
}

func (s *Service) Stop() {
	close(s.stop)
}

func (s *Service) NewMapping(protocol Protocol, ip net.IP, port int) *Mapping {
	mapping := &Mapping{
		protocol: protocol,
		address: Address{
			IP:   ip,
			Port: port,
		},
		extAddresses: make(map[string]Address),
		mut:          sync.NewRWMutex(),
	}

	s.mut.Lock()
	s.mappings = append(s.mappings, mapping)
	s.mut.Unlock()

	return mapping
}

// Sync forces the service to recheck all mappings.
func (s *Service) Sync() {
	wait := make(chan struct{})
	s.immediate <- wait
	<-wait
}

// updateMapping compares the addresses of the existing mapping versus the natds
// discovered, and removes any addresses of natds that do not exist, or tries to
// acquire mappings for natds which the mapping was unaware of before.
// Optionally takes renew flag which indicates whether or not we should renew
// mappings with existing natds
func (s *Service) updateMapping(mapping *Mapping, nats map[string]Device, renew bool) {
	var added, removed []Address

	newAdded, newRemoved := s.verifyExistingMappings(mapping, nats, renew)
	added = append(added, newAdded...)
	removed = append(removed, newRemoved...)

	newAdded, newRemoved = s.acquireNewMappings(mapping, nats)
	added = append(added, newAdded...)
	removed = append(removed, newRemoved...)

	if len(added) > 0 || len(removed) > 0 {
		mapping.notify(added, removed)
	}
}

func (s *Service) verifyExistingMappings(mapping *Mapping, nats map[string]Device, renew bool) ([]Address, []Address) {
	var added, removed []Address

	leaseTime := time.Duration(s.cfg.Options().NATLeaseM) * time.Minute

	for id, address := range mapping.addressMap() {
		// Delete addresses for NATDevice's that do not exist anymore
		nat, ok := nats[id]
		if !ok {
			mapping.removeAddress(id)
			removed = append(removed, address)
			continue
		} else if renew {
			// Only perform renewals on the nat's that have the right local IP
			// address
			localIP := nat.GetLocalIPAddress()
			if !mapping.validGateway(localIP) {
				l.Debugf("Skipping %s for %s because of IP mismatch. %s != %s", id, mapping, mapping.address.IP, localIP)
				continue
			}

			l.Debugf("Renewing %s -> %s mapping on %s", mapping, address, id)

			addr, err := s.tryNATDevice(nat, mapping.address.Port, address.Port, leaseTime)
			if err != nil {
				l.Debugf("Failed to renew %s -> mapping on %s", mapping, address, id)
				mapping.removeAddress(id)
				removed = append(removed, address)
				continue
			}

			l.Debugf("Renewed %s -> %s mapping on %s", mapping, address, id)

			mapping.expires = time.Now().Add(leaseTime)

			if !addr.Equal(address) {
				mapping.removeAddress(id)
				mapping.setAddress(id, addr)
				removed = append(removed, address)
				added = append(added, address)
			}
		}
	}

	return added, removed
}

func (s *Service) acquireNewMappings(mapping *Mapping, nats map[string]Device) ([]Address, []Address) {
	var added, removed []Address

	leaseTime := time.Duration(s.cfg.Options().NATLeaseM) * time.Minute
	addrMap := mapping.addressMap()

	for id, nat := range nats {
		if _, ok := addrMap[id]; ok {
			continue
		}

		// Only perform mappings on the nat's that have the right local IP
		// address
		localIP := nat.GetLocalIPAddress()
		if !mapping.validGateway(localIP) {
			l.Debugf("Skipping %s for %s because of IP mismatch. %s != %s", id, mapping, mapping.address.IP, localIP)
			continue
		}

		l.Debugf("Acquiring %s mapping on %s", mapping, id)

		addr, err := s.tryNATDevice(nat, mapping.address.Port, 0, leaseTime)
		if err != nil {
			l.Debugf("Failed to acquire %s mapping on %s", mapping, id)
			continue
		}

		l.Debugf("Acquired %s -> %s mapping on %s", mapping, addr, id)

		mapping.setAddress(id, addr)
		added = append(added, addr)
	}

	return added, removed
}

// tryNATDevice tries to acquire a port mapping for the given internal address to
// the given external port. If external port is 0, picks a pseudo-random port.
func (s *Service) tryNATDevice(natd Device, intPort, extPort int, leaseTime time.Duration) (Address, error) {
	var err error

	// Generate a predictable random which is based on device ID + local port
	// number so that the ports we'd try to acquire for the mapping would always
	// be the same.
	predictableRand := rand.New(rand.NewSource(int64(s.id.Short()) + int64(intPort)))

	if extPort != 0 {
		// First try renewing our existing mapping, if we have one.
		name := fmt.Sprintf("syncthing-%d", extPort)
		err = natd.AddPortMapping(TCP, intPort, extPort, name, leaseTime)
		if err == nil {
			goto findIP
		}
		l.Debugln("Error extending lease on", natd.ID(), err)
	}

	for i := 0; i < 10; i++ {
		// Then try up to ten random ports.
		extPort = 1024 + predictableRand.Intn(65535-1024)
		name := fmt.Sprintf("syncthing-%d", extPort)
		err = natd.AddPortMapping(TCP, intPort, extPort, name, leaseTime)
		if err == nil {
			goto findIP
		}
		l.Debugln("Error getting new lease on", natd.ID(), err)
	}

	return Address{}, err

findIP:
	ip, err := natd.GetExternalIPAddress()
	if err != nil {
		l.Debugln("Error getting external ip on", natd.ID(), err)
		ip = nil
	}
	return Address{
		IP:   ip,
		Port: extPort,
	}, nil
}
