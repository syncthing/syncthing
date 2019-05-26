// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package dialer

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/syncthing/syncthing/lib/connections/registry"
	"golang.org/x/net/proxy"
)

var (
	proxyDialer proxy.Dialer
	usingProxy  bool
	noFallback  = os.Getenv("ALL_PROXY_NO_FALLBACK") != ""
)

type dialFunc func(network, addr string) (net.Conn, error)

func init() {
	proxy.RegisterDialerType("socks", socksDialerFunction)
	proxyDialer = getDialer(proxy.Direct)
	usingProxy = proxyDialer != proxy.Direct

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

// This is a rip off of proxy.FromURL for "socks" URL scheme
func socksDialerFunction(u *url.URL, forward proxy.Dialer) (proxy.Dialer, error) {
	var auth *proxy.Auth
	if u.User != nil {
		auth = new(proxy.Auth)
		auth.User = u.User.Username()
		if p, ok := u.User.Password(); ok {
			auth.Password = p
		}
	}

	return proxy.SOCKS5("tcp", u.Host, auth, forward)
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
	return dialTimeout(network, addr, d.timeout)
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

// Sort available tcp address, preferring unspecified address.
func tcpAddrLess(i interface{}, j interface{}) bool {
	iIsUnspecified := false
	jIsUnspecified := false
	iAddr := i.(*net.TCPAddr)
	jAddr := j.(*net.TCPAddr)

	if host, _, err := net.SplitHostPort(iAddr.String()); err == nil {
		iIsUnspecified = host == "" || net.ParseIP(host).IsUnspecified()
	}
	if host, _, err := net.SplitHostPort(jAddr.String()); err == nil {
		jIsUnspecified = host == "" || net.ParseIP(host).IsUnspecified()
	}

	if jIsUnspecified != iIsUnspecified {
		return len(iAddr.Network()) <= len(jAddr.Network())
	}

	return iIsUnspecified
}

func dial(network, addr string) (net.Conn, error) {
	localAddrInterface := registry.Get(network, tcpAddrLess)
	if localAddrInterface == nil {
		return net.Dial(network, addr)
	}
	dialer := net.Dialer{
		LocalAddr: localAddrInterface.(*net.TCPAddr),
		Control:   ReusePortControl,
	}
	return dialer.Dial(network, addr)
}

func dialTimeout(network, addr string, timeout time.Duration) (net.Conn, error) {
	localAddrInterface := registry.Get(network, tcpAddrLess)
	if localAddrInterface == nil {
		return net.DialTimeout(network, addr, timeout)
	}
	dialer := net.Dialer{
		LocalAddr: localAddrInterface.(*net.TCPAddr),
		Control:   ReusePortControl,
	}
	ctx, _ := context.WithTimeout(context.Background(), timeout)
	return dialer.DialContext(ctx, network, addr)
}
