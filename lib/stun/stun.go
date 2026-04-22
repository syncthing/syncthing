// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package stun

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/ccding/go-stun/stun"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/svcutil"
)

const stunRetryInterval = 5 * time.Minute

type (
	Host    = stun.Host
	NATType = stun.NATType
)

// NAT types.

const (
	NATError                = stun.NATError
	NATUnknown              = stun.NATUnknown
	NATNone                 = stun.NATNone
	NATBlocked              = stun.NATBlocked
	NATFull                 = stun.NATFull
	NATSymmetric            = stun.NATSymmetric
	NATRestricted           = stun.NATRestricted
	NATPortRestricted       = stun.NATPortRestricted
	NATSymmetricUDPFirewall = stun.NATSymmetricUDPFirewall
)

var errNotPunchable = errors.New("not punchable")

type Subscriber interface {
	OnNATTypeChanged(natType NATType)
	OnExternalAddressChanged(address *Host, via string)
}

type Service struct {
	name       string
	cfg        config.Wrapper
	subscriber Subscriber
	client     *stun.Client

	natType NATType
	addr    *Host
}

func New(cfg config.Wrapper, subscriber Subscriber, conn net.PacketConn) *Service {
	// Construct the client to use the stun conn
	client := stun.NewClientWithConnection(conn)
	client.SetSoftwareName("") // Explicitly unset this, seems to freak some servers out.

	// Return the service and the other conn to the client
	name := "Stun@"
	if local := conn.LocalAddr(); local != nil {
		name += local.Network() + "://" + local.String()
	} else {
		name += "unknown"
	}
	s := &Service{
		name: name,

		cfg:        cfg,
		subscriber: subscriber,
		client:     client,

		natType: NATUnknown,
		addr:    nil,
	}
	return s
}

func (s *Service) Serve(ctx context.Context) error {
	defer func() {
		s.setNATType(NATUnknown)
		s.setExternalAddress(nil, "")
	}()

	timer := time.NewTimer(time.Millisecond)

	for {
	disabled:
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}

		if s.cfg.Options().IsStunDisabled() {
			timer.Reset(time.Second)
			continue
		}

		l.Debugf("Starting stun for %s", s)

		for _, addr := range s.cfg.Options().StunServers() {
			// This blocks until we hit an exit condition or there are
			// issues with the STUN server.
			if err := s.runStunForServer(ctx, addr); errors.Is(err, errNotPunchable) {
				break // we will sleep for a while
			}

			// Have we been asked to stop?
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			// Are we disabled?
			if s.cfg.Options().IsStunDisabled() {
				slog.InfoContext(ctx, "STUN disabled")
				s.setNATType(NATUnknown)
				s.setExternalAddress(nil, "")
				goto disabled
			}
		}

		// We failed to contact all provided stun servers or the nat is not punchable.
		// Chillout for a while.
		timer.Reset(stunRetryInterval)
	}
}

func (s *Service) runStunForServer(ctx context.Context, addr string) error {
	l.Debugf("Running stun for %s via %s", s, addr)

	// Resolve the address, so that in case the server advertises two
	// IPs, we always hit the same one, as otherwise, the mapping might
	// expire as we hit the other address, and cause us to flip flop
	// between servers/external addresses, as a result flooding discovery
	// servers.
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		l.Debugf("%s stun addr resolution on %s: %s", s, addr, err)
		return err
	}
	s.client.SetServerAddr(udpAddr.String())

	var natType stun.NATType
	var extAddr *stun.Host
	err = svcutil.CallWithContext(ctx, func() error {
		natType, extAddr, err = s.client.Discover()
		return err
	})
	if err != nil {
		l.Debugf("%s stun discovery on %s: %v", s, addr, err)
		return err
	} else if extAddr == nil {
		l.Debugf("%s stun discovery on %s resulted in no address", s, addr)
		return fmt.Errorf("%s: no address", addr)
	}

	// The stun server is most likely borked, try another one.
	if natType == NATError || natType == NATUnknown || natType == NATBlocked {
		l.Debugf("%s stun discovery on %s resolved to %s", s, addr, natType)
		return fmt.Errorf("%s: bad result: %v", addr, natType)
	}

	s.setNATType(natType)
	l.Debugf("%s detected NAT type: %s via %s", s, natType, addr)

	// We can't punch through this one, so no point doing keepalives
	// and such, just let the caller check the nat type and work it out themselves.
	if !s.isCurrentNATTypePunchable() {
		l.Debugf("%s cannot punch %s, skipping", s, natType)
		return errNotPunchable
	}

	s.setExternalAddress(extAddr, addr)
	return s.stunKeepAlive(ctx, addr, extAddr)
}

func (s *Service) stunKeepAlive(ctx context.Context, addr string, extAddr *Host) error {
	var err error
	nextSleep := time.Duration(s.cfg.Options().StunKeepaliveStartS) * time.Second

	l.Debugf("%s starting stun keepalive via %s, next sleep %s", s, addr, nextSleep)

	var ourLastWrite time.Time
	for {
		if areDifferent(s.addr, extAddr) {
			// If the port has changed (addresses are not equal but the hosts are equal),
			// we're probably spending too much time between keepalives, reduce the sleep.
			if s.addr != nil && extAddr != nil && s.addr.IP() == extAddr.IP() {
				nextSleep /= 2
				l.Debugf("%s stun port change (%s to %s), next sleep %s", s, s.addr.TransportAddr(), extAddr.TransportAddr(), nextSleep)
			}

			s.setExternalAddress(extAddr, addr)

			// The stun server is probably stuffed, we've gone beyond min timeout, yet the address keeps changing.
			minSleep := time.Duration(s.cfg.Options().StunKeepaliveMinS) * time.Second
			if nextSleep < minSleep {
				l.Debugf("%s keepalive aborting, sleep below min: %s < %s", s, nextSleep, minSleep)
				return fmt.Errorf("unreasonably low keepalive: %v", minSleep)
			}
		}

		// Adjust the keepalives to fire only nextSleep after last write.
		minSleep := time.Duration(s.cfg.Options().StunKeepaliveMinS) * time.Second
		if nextSleep < minSleep {
			nextSleep = minSleep
		}
		sleepFor := nextSleep

		timeUntilNextKeepalive := time.Until(ourLastWrite.Add(sleepFor))
		if timeUntilNextKeepalive > 0 {
			sleepFor = timeUntilNextKeepalive
		}

		l.Debugf("%s stun sleeping for %s", s, sleepFor)

		select {
		case <-time.After(sleepFor):
		case <-ctx.Done():
			l.Debugf("%s stopping, aborting stun", s)
			return ctx.Err()
		}

		if s.cfg.Options().IsStunDisabled() {
			// Disabled, give up
			l.Debugf("%s disabled, aborting stun ", s)
			return errors.New("disabled")
		}

		l.Debugf("%s stun keepalive", s)

		extAddr, err = s.client.Keepalive()
		if err != nil {
			l.Debugf("%s stun keepalive on %s: %s (%v)", s, addr, err, extAddr)
			return err
		}
		ourLastWrite = time.Now()
	}
}

func (s *Service) setNATType(natType NATType) {
	if natType != s.natType {
		l.Debugf("Notifying %s of NAT type change: %s", s.subscriber, natType)
		s.subscriber.OnNATTypeChanged(natType)
	}
	s.natType = natType
}

func (s *Service) setExternalAddress(addr *Host, via string) {
	if areDifferent(s.addr, addr) {
		l.Debugf("Notifying %s of address change: %s via %s", s.subscriber, addr, via)
		s.subscriber.OnExternalAddressChanged(addr, via)
	}
	s.addr = addr
}

func (s *Service) String() string {
	return s.name
}

func (s *Service) isCurrentNATTypePunchable() bool {
	return s.natType == NATNone || s.natType == NATPortRestricted || s.natType == NATRestricted || s.natType == NATFull || s.natType == NATSymmetricUDPFirewall
}

func areDifferent(first, second *Host) bool {
	if (first == nil) != (second == nil) {
		return true
	}
	if first != nil {
		return first.TransportAddr() != second.TransportAddr()
	}
	return false
}
