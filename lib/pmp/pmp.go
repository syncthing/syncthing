// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package pmp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/jackpal/gateway"
	natpmp "github.com/jackpal/go-nat-pmp"

	"github.com/syncthing/syncthing/lib/nat"
	"github.com/syncthing/syncthing/lib/util"
)

func init() {
	nat.Register(Discover)
}

func Discover(ctx context.Context, renewal, timeout time.Duration) []nat.Device {
	var ip net.IP
	err := util.CallWithContext(ctx, func() error {
		var err error
		ip, err = gateway.DiscoverGateway()
		return err
	})
	if err != nil {
		l.Debugln("Failed to discover gateway", err)
		return nil
	}
	if ip == nil || ip.IsUnspecified() {
		return nil
	}

	l.Debugln("Discovered gateway at", ip)

	c := natpmp.NewClientWithTimeout(ip, timeout)
	// Try contacting the gateway, if it does not respond, assume it does not
	// speak NAT-PMP.
	err = util.CallWithContext(ctx, func() error {
		_, ierr := c.GetExternalAddress()
		return ierr
	})
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil
		}
		if strings.Contains(err.Error(), "Timed out") {
			l.Debugln("Timeout trying to get external address, assume no NAT-PMP available")
			return nil
		}
	}

	var localIP net.IP
	// Port comes from the natpmp package
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(timeoutCtx, "udp", net.JoinHostPort(ip.String(), "5351"))
	if err == nil {
		conn.Close()
		localIPAddress, _, err := net.SplitHostPort(conn.LocalAddr().String())
		if err == nil {
			localIP = net.ParseIP(localIPAddress)
		} else {
			l.Debugln("Failed to lookup local IP", err)
		}
	}

	return []nat.Device{&wrapper{
		renewal:   renewal,
		localIP:   localIP,
		gatewayIP: ip,
		client:    c,
	}}
}

type wrapper struct {
	renewal   time.Duration
	localIP   net.IP
	gatewayIP net.IP
	client    *natpmp.Client
}

func (w *wrapper) ID() string {
	return fmt.Sprintf("NAT-PMP@%s", w.gatewayIP.String())
}

func (w *wrapper) GetLocalIPAddress() net.IP {
	return w.localIP
}

func (w *wrapper) AddPortMapping(ctx context.Context, protocol nat.Protocol, internalPort, externalPort int, _ string, duration time.Duration) (int, error) {
	// NAT-PMP says that if duration is 0, the mapping is actually removed
	// Swap the zero with the renewal value, which should make the lease for the
	// exact amount of time between the calls.
	if duration == 0 {
		duration = w.renewal
	}
	var result *natpmp.AddPortMappingResult
	err := util.CallWithContext(ctx, func() error {
		var err error
		result, err = w.client.AddPortMapping(strings.ToLower(string(protocol)), internalPort, externalPort, int(duration/time.Second))
		return err
	})
	port := 0
	if result != nil {
		port = int(result.MappedExternalPort)
	}
	return port, err
}

func (w *wrapper) GetExternalIPAddress(ctx context.Context) (net.IP, error) {
	var result *natpmp.GetExternalAddressResult
	err := util.CallWithContext(ctx, func() error {
		var err error
		result, err = w.client.GetExternalAddress()
		return err
	})
	ip := net.IPv4zero
	if result != nil {
		ip = net.IPv4(
			result.ExternalIPAddress[0],
			result.ExternalIPAddress[1],
			result.ExternalIPAddress[2],
			result.ExternalIPAddress[3],
		)
	}
	return ip, err
}
