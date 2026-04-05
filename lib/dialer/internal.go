// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package dialer

import (
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"golang.org/x/net/proxy"
)

var noFallback = os.Getenv("ALL_PROXY_NO_FALLBACK") != ""

func init() {
	proxy.RegisterDialerType("socks", socksDialerFunction)

	if proxyDialer := proxy.FromEnvironment(); proxyDialer != proxy.Direct {
		http.DefaultTransport = &http.Transport{
			DialContext:         DialContext,
			Proxy:               http.ProxyFromEnvironment,
			TLSHandshakeTimeout: 10 * time.Second,
		}

		// Defer this, so that logging gets set up.
		go func() {
			time.Sleep(500 * time.Millisecond)
			slog.Info("Proxy settings detected")
			if noFallback {
				slog.Info("Proxy fallback disabled")
			}
		}()
	} else {
		go func() {
			time.Sleep(500 * time.Millisecond)
			slog.Debug("Dialer logging disabled, as no proxy was detected")
		}()
	}
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

// dialerConn is needed because proxy dialed connections have RemoteAddr() pointing at the proxy,
// which then screws up various things such as IsLAN checks, and "let's populate the relay invitation address from
// existing connection" shenanigans.
type dialerConn struct {
	net.Conn

	addr net.Addr
}

func (c dialerConn) RemoteAddr() net.Addr {
	return c.addr
}

func newDialerAddr(network, addr string) net.Addr {
	netAddr, err := net.ResolveIPAddr(network, addr)
	if err == nil {
		return netAddr
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
