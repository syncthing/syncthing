// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package main

import (
	"fmt"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/sync"
	"github.com/syncthing/syncthing/lib/upnp"
)

// The UPnP service runs a loop for discovery of IGDs (Internet Gateway
// Devices) and setup/renewal of a port mapping.
type upnpSvc struct {
	cfg        *config.Wrapper
	localPort  int
	extPort    int
	extPortMut sync.Mutex
	stop       chan struct{}
}

func newUPnPSvc(cfg *config.Wrapper, localPort int) *upnpSvc {
	return &upnpSvc{
		cfg:        cfg,
		localPort:  localPort,
		extPortMut: sync.NewMutex(),
	}
}

func (s *upnpSvc) Serve() {
	foundIGD := true
	s.stop = make(chan struct{})

	for {
		igds := upnp.Discover(time.Duration(s.cfg.Options().UPnPTimeoutS) * time.Second)
		if len(igds) > 0 {
			foundIGD = true
			s.extPortMut.Lock()
			oldExtPort := s.extPort
			s.extPortMut.Unlock()

			newExtPort := s.tryIGDs(igds, oldExtPort)

			s.extPortMut.Lock()
			s.extPort = newExtPort
			s.extPortMut.Unlock()
		} else if foundIGD {
			// Only print a notice if we've previously found an IGD or this is
			// the first time around.
			foundIGD = false
			l.Infof("No UPnP device detected")
		}

		d := time.Duration(s.cfg.Options().UPnPRenewalM) * time.Minute
		if d == 0 {
			// We always want to do renewal so lets just pick a nice sane number.
			d = 30 * time.Minute
		}

		select {
		case <-s.stop:
			return
		case <-time.After(d):
		}
	}
}

func (s *upnpSvc) Stop() {
	close(s.stop)
}

func (s *upnpSvc) ExternalPort() int {
	s.extPortMut.Lock()
	port := s.extPort
	s.extPortMut.Unlock()
	return port
}

func (s *upnpSvc) tryIGDs(igds []upnp.IGD, prevExtPort int) int {
	// Lets try all the IGDs we found and use the first one that works.
	// TODO: Use all of them, and sort out the resulting mess to the
	// discovery announcement code...
	for _, igd := range igds {
		extPort, err := s.tryIGD(igd, prevExtPort)
		if err != nil {
			l.Warnf("Failed to set UPnP port mapping: external port %d on device %s.", extPort, igd.FriendlyIdentifier())
			continue
		}

		if extPort != prevExtPort {
			l.Infof("New UPnP port mapping: external port %d to local port %d.", extPort, s.localPort)
			events.Default.Log(events.ExternalPortMappingChanged, map[string]int{"port": extPort})
		}
		l.Debugf("Created/updated UPnP port mapping for external port %d on device %s.", extPort, igd.FriendlyIdentifier())
		return extPort
	}

	return 0
}

func (s *upnpSvc) tryIGD(igd upnp.IGD, suggestedPort int) (int, error) {
	var err error
	leaseTime := s.cfg.Options().UPnPLeaseM * 60

	if suggestedPort != 0 {
		// First try renewing our existing mapping.
		name := fmt.Sprintf("syncthing-%d", suggestedPort)
		err = igd.AddPortMapping(upnp.TCP, suggestedPort, s.localPort, name, leaseTime)
		if err == nil {
			return suggestedPort, nil
		}
	}

	for i := 0; i < 10; i++ {
		// Then try up to ten random ports.
		extPort := 1024 + predictableRandom.Intn(65535-1024)
		name := fmt.Sprintf("syncthing-%d", extPort)
		err = igd.AddPortMapping(upnp.TCP, extPort, s.localPort, name, leaseTime)
		if err == nil {
			return extPort, nil
		}
	}

	return 0, err
}
