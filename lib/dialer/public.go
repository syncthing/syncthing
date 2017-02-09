// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package dialer

import (
	"fmt"
	"net"
	"time"

	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

// Dial tries dialing via proxy if a proxy is configured, and falls back to
// a direct connection if no proxy is defined, or connecting via proxy fails.
func Dial(network, addr string) (net.Conn, error) {
	if usingProxy {
		return dialWithFallback(proxyDialer.Dial, net.Dial, network, addr)
	}
	return net.Dial(network, addr)
}

// DialTimeout tries dialing via proxy with a timeout if a proxy is configured,
// and falls back to a direct connection if no proxy is defined, or connecting
// via proxy fails. The timeout can potentially be applied twice, once trying
// to connect via the proxy connection, and second time trying to connect
// directly.
func DialTimeout(network, addr string, timeout time.Duration) (net.Conn, error) {
	if usingProxy {
		// Because the proxy package is poorly structured, we have to
		// construct a struct that matches proxy.Dialer but has a timeout
		// and reconstrcut the proxy dialer using that, in order to be able to
		// set a timeout.
		dd := &timeoutDirectDialer{
			timeout: timeout,
		}
		// Check if the dialer we are getting is not timeoutDirectDialer we just
		// created. It could happen that usingProxy is true, but getDialer
		// returns timeoutDirectDialer due to env vars changing.
		if timeoutProxyDialer := getDialer(dd); timeoutProxyDialer != dd {
			directDialFunc := func(inetwork, iaddr string) (net.Conn, error) {
				return net.DialTimeout(inetwork, iaddr, timeout)
			}
			return dialWithFallback(timeoutProxyDialer.Dial, directDialFunc, network, addr)
		}
	}
	return net.DialTimeout(network, addr, timeout)
}

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

	case dialerConn:
		return SetTCPOptions(conn.Conn)

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

	case dialerConn:
		return SetTrafficClass(conn.Conn, class)

	default:
		return fmt.Errorf("unknown connection type %T", conn)
	}
}
