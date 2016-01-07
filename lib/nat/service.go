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

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/sync"
	"github.com/syncthing/syncthing/lib/util"
)

// Service runs a loop for discovery of natds (Internet Gateway Devices) and
// setup/renewal of a port mapping.
type Service struct {
	cfg       *config.Wrapper
	stop      chan struct{}
	immediate chan chan struct{}

	mappings []*Mapping
	mut      sync.RWMutex
}

func NewService(cfg *config.Wrapper) *Service {
	return &Service{
		cfg:      cfg,
		stop:     make(chan struct{}),
		mappings: make([]*Mapping, 0),
		mut:      sync.NewRWMutex(),
	}
}

func (s *Service) Serve() {
	first := true
	needToRenew := make([]*Mapping, 0)
	needToUpdate := make([]*Mapping, 0)

	timer := time.NewTimer(time.Second)

	s.mut.Lock()
	s.immediate = make(chan chan struct{})
	s.mut.Unlock()

	process := func() {
		needToRenew = needToRenew[:0]
		needToUpdate = needToUpdate[:0]

		renewIn := time.Duration(s.cfg.Options().UPnPRenewalM) * time.Minute
		if renewIn == 0 {
			// We always want to do renewal so lets just pick a nice sane number.
			renewIn = 30 * time.Minute
		}

		s.mut.RLock()
		for _, mapping := range s.mappings {
			if mapping.expires.Before(time.Now()) {
				needToRenew = append(needToRenew, mapping)
			} else {
				needToUpdate = append(needToUpdate, mapping)
				mappingRenewIn := mapping.expires.Sub(time.Now())
				if mappingRenewIn < renewIn {
					renewIn = mappingRenewIn
				}
			}
		}
		s.mut.RUnlock()

		timer.Reset(renewIn)

		if len(needToRenew) == 0 {
			return
		}

		natds := discoverAll(time.Duration(s.cfg.Options().UPnPTimeoutS) * time.Second)

		if first {
			l.Infoln("Detected", len(natds), "NAT devices")
			first = false
		}

		for _, mapping := range needToRenew {
			s.updateMapping(mapping, natds, true)
		}

		for _, mapping := range needToUpdate {
			s.updateMapping(mapping, natds, false)
		}
	}

	for {
		select {
		case result := <-s.immediate:
			process()
			close(result)
		case <-timer.C:
			process()
		case <-s.stop:
			timer.Stop()
			s.mut.Lock()
			s.immediate = nil
			s.mut.Unlock()
			return
		}
	}
}

func (s *Service) Stop() {
	close(s.stop)
}

func (s *Service) NewMapping(protocol Protocol, ip net.IP, port int) *Mapping {
	mapping := &Mapping{
		protocol:  protocol,
		ip:        ip,
		port:      port,
		addresses: make(map[string]Address),
		mut:       sync.NewRWMutex(),
	}
	s.mut.Lock()
	s.mappings = append(s.mappings, mapping)
	if s.immediate != nil {
		wait := make(chan struct{})
		s.immediate <- wait
		<-wait
	}
	s.mut.Unlock()

	return mapping
}

// updateMapping compares the addresses the mapping has versus the natds
// discovered, and removes any addresses of natds that do not exist, or tries to
// acquire mappings for natds which the mapping was unaware of before.
// Optionally takes renew flag which indicates whether or not we should renew
// mappings with existing natds
func (s *Service) updateMapping(mapping *Mapping, natds map[string]NATDevice, renew bool) {
	leaseTime := time.Duration(s.cfg.Options().UPnPLeaseM) * time.Minute
	modified := false

	addrMap := mapping.addressMap()

	for natID, address := range addrMap {
		// Delete mappings for NATDevice's that do not exist anymore
		natd, ok := natds[natID]
		if !ok {
			mapping.removeAddress(natID)
			modified = true
			continue
		} else if renew {

			// Only perform mappings on the nat's that have the right local IP
			// address
			localIP := natd.GetLocalIPAddress()
			if mapping.validGateway(localIP) {
				l.Debugln("Skipping", natID, "because of IP mismatch", mapping.ip, "!=", localIP)
				continue
			}

			mapping.expires = time.Now().Add(leaseTime)
			addr, err := s.tryNATDevice(natd, mapping.ip, mapping.port, address.Port, leaseTime)
			if err != nil {
				mapping.removeAddress(natID)
				modified = true
				continue
			}

			if !addr.Equal(address) {
				mapping.setAddress(natID, addr)
				modified = true
			}
		}
	}

	for natID, natd := range natds {
		_, ok := addrMap[natID]
		if ok {
			continue
		}

		// Only perform mappings on the nat's that have the right local IP
		// address
		localIP := natd.GetLocalIPAddress()
		if mapping.validGateway(localIP) {
			l.Debugln("Skipping", natID, "because of IP mismatch", mapping.ip, "!=", localIP)
			continue
		}

		addr, err := s.tryNATDevice(natd, mapping.ip, mapping.port, 0, leaseTime)
		if err != nil {
			continue
		}

		mapping.setAddress(natID, addr)
		modified = true
	}

	if modified {
		mapping.notify()
	}
}

// tryNATDevice tries to acquire a port mapping for the given internal port to
// the given external port. If external port is 0, picks a pseudo-random port.
func (s *Service) tryNATDevice(natd NATDevice, ip net.IP, intPort, extPort int, leaseTime time.Duration) (Address, error) {
	var err error

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
		extPort = 1024 + util.PredictableRandom.Intn(65535-1024)
		name := fmt.Sprintf("syncthing-%d", extPort)
		err = natd.AddPortMapping(TCP, intPort, extPort, name, leaseTime)
		if err == nil {
			goto findIP
		}
		l.Debugln("Error getting new lease on", natd.ID(), err)
	}

	return Address{}, err

findIP:
	ip, err = natd.GetExternalIPAddress()
	if err != nil {
		l.Debugln("Error getting external ip on", natd.ID(), err)
		ip = nil
	}
	return Address{
		IP:   ip,
		Port: extPort,
	}, nil
}
