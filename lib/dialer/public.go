// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package dialer

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/syncthing/syncthing/lib/connections/registry"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
	"golang.org/x/net/proxy"
)

// SetTCPOptions sets our default TCP options on a TCP connection, possibly
// digging through dialerConn to extract the *net.TCPConn
func SetTCPOptions(conn net.Conn) error {
	switch conn := conn.(type) {
	case *net.TCPConn:
		var err error
		if err = conn.SetLinger(0); err != nil {
			return err
		}
		if err = conn.SetNoDelay(false); err != nil {
			return err
		}
		if err = conn.SetKeepAlivePeriod(60 * time.Second); err != nil {
			return err
		}
		if err = conn.SetKeepAlive(true); err != nil {
			return err
		}
		return nil
	default:
		return fmt.Errorf("unknown connection type %T", conn)
	}
}

func SetTrafficClass(conn net.Conn, class int) error {
	switch conn := conn.(type) {
	case *net.TCPConn:
		e1 := ipv4.NewConn(conn).SetTOS(class)
		e2 := ipv6.NewConn(conn).SetTrafficClass(class)

		if e1 != nil {
			return e1
		}
		return e2
	default:
		return fmt.Errorf("unknown connection type %T", conn)
	}
}

func dialContextWithFallback(ctx context.Context, fallback proxy.ContextDialer, network, addr string) (net.Conn, error) {
	dialer := proxy.FromEnvironment().(proxy.ContextDialer)
	if dialer != proxy.Direct {
		// Capture the existing timeout by checking the deadline
		var timeout time.Duration
		if deadline, ok := ctx.Deadline(); ok {
			timeout = deadline.Sub(deadline)
		}
		if conn, err := dialer.DialContext(ctx, network, addr); noFallback || err == nil {
			return conn, err
		}
		// If the deadline was set, reset it again for the next dial attempt.
		if timeout != 0 {
			ctx, _ = context.WithTimeout(ctx, timeout)
		}
	}
	return fallback.DialContext(ctx, network, addr)
}

// DialContext tries dialing via proxy if a proxy is configured, and falls back to
// a direct connection if no proxy is defined, or connecting via proxy fails.
// If the context has a timeout, the timeout might be applied twice.
func DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	return dialContextWithFallback(ctx, proxy.Direct, network, addr)
}

// DialContextReusePort tries dialing via proxy if a proxy is configured, and falls back to
// a direct connection reusing the port from the connections registry, if no proxy is defined, or connecting via proxy
// fails. If the context has a timeout, the timeout might be applied twice.
func DialContextReusePort(ctx context.Context, network, addr string) (net.Conn, error) {
	dialer := &net.Dialer{
		Control: ReusePortControl,
	}
	localAddrInterface := registry.Get(network, tcpAddrLess)
	if localAddrInterface != nil {
		dialer.LocalAddr = localAddrInterface.(*net.TCPAddr)
	}

	return dialContextWithFallback(ctx, dialer, network, addr)
}
