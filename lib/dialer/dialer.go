// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package dialer

import (
	"net"
	"os"
	"strings"
	"time"

	"golang.org/x/net/proxy"

	"github.com/syncthing/syncthing/lib/logger"
)

var (
	l          = logger.DefaultLogger.NewFacility("dialer", "Dialing connections")
	dialer     = proxy.FromEnvironment()
	usingProxy = dialer != proxy.Direct
)

func init() {
	l.SetDebug("dialer", strings.Contains(os.Getenv("STTRACE"), "dialer") || os.Getenv("STTRACE") == "all")
	if usingProxy {
		// Defer this, so that logging gets setup.
		go func() {
			time.Sleep(500 * time.Millisecond)
			l.Infoln("Proxy settings detected")
		}()
	}
}

// Dial tries dialing via proxy if a proxy is configured, and falls back to
// a direct connection if no proxy is defined, or connecting via proxy fails.
func Dial(network, addr string) (net.Conn, error) {
	if usingProxy {
		conn, err := dialer.Dial(network, addr)
		if err == nil {
			l.Debugf("Dialing %s address %s via proxy - success, %s -> %s", network, addr, conn.LocalAddr(), conn.RemoteAddr())
			return dialerConn{
				conn, newDialerAddr(network, addr),
			}, nil
		}
		l.Debugf("Dialing %s address %s via proxy - error %s", network, addr, err)
	}

	conn, err := proxy.Direct.Dial(network, addr)
	if err == nil {
		l.Debugf("Dialing %s address %s directly - success, %s -> %s", network, addr, conn.LocalAddr(), conn.RemoteAddr())
	} else {
		l.Debugf("Dialing %s address %s directly - error %s", network, addr, err)
	}
	return conn, err
}

type dialerConn struct {
	net.Conn
	addr net.Addr
}

func (c dialerConn) RemoteAddr() net.Addr {
	return c.addr
}

func newDialerAddr(network, addr string) net.Addr {
	netaddr, err := net.ResolveIPAddr(network, addr)
	if err == nil {
		return netaddr
	}
	return fallbackAddr{network, addr}
}

type fallbackAddr struct {
	network string
	addr    string
}

func (a fallbackAddr) Network() string {
	return a.network
}

func (a fallbackAddr) String() string {
	return a.addr
}
