// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package dialer

import (
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"golang.org/x/net/proxy"

	"github.com/syncthing/syncthing/lib/logger"
)

var (
	l           = logger.DefaultLogger.NewFacility("dialer", "Dialing connections")
	proxyDialer = getDialer(proxy.Direct)
	usingProxy  = proxyDialer != proxy.Direct
	noFallback  = os.Getenv("ALL_PROXY_NO_FALLBACK") != ""
)

type dialFunc func(network, addr string) (net.Conn, error)

func init() {
	l.SetDebug("dialer", strings.Contains(os.Getenv("STTRACE"), "dialer") || os.Getenv("STTRACE") == "all")
	if usingProxy {
		http.DefaultTransport = &http.Transport{
			Dial:                Dial,
			Proxy:               http.ProxyFromEnvironment,
			TLSHandshakeTimeout: 10 * time.Second,
		}

		// Defer this, so that logging gets setup.
		go func() {
			time.Sleep(500 * time.Millisecond)
			l.Infoln("Proxy settings detected")
			if noFallback {
				l.Infoln("Proxy fallback disabled")
			}
		}()
	} else {
		go func() {
			time.Sleep(500 * time.Millisecond)
			l.Debugln("Dialer logging disabled, as no proxy was detected")
		}()
	}
}

func dialWithFallback(proxyDialFunc dialFunc, fallbackDialFunc dialFunc, network, addr string) (net.Conn, error) {
	conn, err := proxyDialFunc(network, addr)
	if err == nil {
		l.Debugf("Dialing %s address %s via proxy - success, %s -> %s", network, addr, conn.LocalAddr(), conn.RemoteAddr())
		SetTCPOptions(conn)
		return dialerConn{
			conn, newDialerAddr(network, addr),
		}, nil
	}
	l.Debugf("Dialing %s address %s via proxy - error %s", network, addr, err)

	if noFallback {
		return conn, err
	}

	conn, err = fallbackDialFunc(network, addr)
	if err == nil {
		l.Debugf("Dialing %s address %s via fallback - success, %s -> %s", network, addr, conn.LocalAddr(), conn.RemoteAddr())
		SetTCPOptions(conn)
	} else {
		l.Debugf("Dialing %s address %s via fallback - error %s", network, addr, err)
	}
	return conn, err
}

// This is a rip off of proxy.FromEnvironment with a custom forward dialer
func getDialer(forward proxy.Dialer) proxy.Dialer {
	allProxy := os.Getenv("all_proxy")
	if len(allProxy) == 0 {
		return forward
	}

	proxyURL, err := url.Parse(allProxy)
	if err != nil {
		return forward
	}
	prxy, err := proxy.FromURL(proxyURL, forward)
	if err != nil {
		return forward
	}

	noProxy := os.Getenv("no_proxy")
	if len(noProxy) == 0 {
		return prxy
	}

	perHost := proxy.NewPerHost(prxy, forward)
	perHost.AddFromString(noProxy)
	return perHost
}

type timeoutDirectDialer struct {
	timeout time.Duration
}

func (d *timeoutDirectDialer) Dial(network, addr string) (net.Conn, error) {
	return net.DialTimeout(network, addr, d.timeout)
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
