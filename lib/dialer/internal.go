// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package dialer

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	"golang.org/x/net/proxy"
)

var noFallback = os.Getenv("ALL_PROXY_NO_FALLBACK") != ""

func init() {
	proxy.RegisterDialerType("socks", socksDialerFunction)
	proxy.RegisterDialerType("http", httpDialerFunction)
	proxy.RegisterDialerType("https", httpDialerFunction)

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

type httpProxyDialer struct {
	proxyURL      *url.URL
	forwardDialer proxy.Dialer
}

func httpDialerFunction(u *url.URL, forward proxy.Dialer) (proxy.Dialer, error) {
	return &httpProxyDialer{
		proxyURL:      u,
		forwardDialer: forward,
	}, nil
}

func (h *httpProxyDialer) Dial(network, addr string) (net.Conn, error) {
	return h.DialContext(context.Background(), network, addr)
}

type bufferedConn struct {
	net.Conn
	*bufio.Reader
}

func (c *bufferedConn) Read(b []byte) (int, error) {
	return c.Reader.Read(b)
}

var warnCleartextProxyAuthOnce sync.Once

func (h *httpProxyDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	var conn net.Conn
	var err error

	if network != "tcp" && network != "tcp4" && network != "tcp6" {
		return nil, fmt.Errorf("unsupported network for http proxy: %s", network)
	}

	if cd, ok := h.forwardDialer.(proxy.ContextDialer); ok {
		conn, err = cd.DialContext(ctx, "tcp", h.proxyURL.Host)
	} else {
		conn, err = h.forwardDialer.Dial("tcp", h.proxyURL.Host)
	}
	if err != nil {
		return nil, err
	}

	if deadline, ok := ctx.Deadline(); ok {
		conn.SetDeadline(deadline)
		defer conn.SetDeadline(time.Time{})
	}

	if h.proxyURL.Scheme == "https" {
		tlsConn := tls.Client(conn, &tls.Config{
			ServerName: h.proxyURL.Hostname(),
		})
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			conn.Close()
			return nil, err
		}
		conn = tlsConn
	}

	req := &http.Request{
		Method: "CONNECT",
		URL:    &url.URL{Host: addr},
		Host:   addr,
		Header: make(http.Header),
	}
	req = req.WithContext(ctx)

	if u := h.proxyURL.User; u != nil {
		if h.proxyURL.Scheme == "http" {
			warnCleartextProxyAuthOnce.Do(func() {
				slog.Warn(
					"Using basic auth over cleartext HTTP proxy",
					"proxy", h.proxyURL.Redacted(),
				)
			})
		}
		username := u.Username()
		password, _ := u.Password()
		auth := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
		req.Header.Set("Proxy-Authorization", "Basic "+auth)
	}

	if err := req.Write(conn); err != nil {
		conn.Close()
		return nil, err
	}

	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, req)
	if err != nil {
		conn.Close()
		return nil, err
	}
	resp.Body.Close()

	if resp.StatusCode != 200 {
		conn.Close()
		return nil, fmt.Errorf("http proxy CONNECT failed: %s", resp.Status)
	}

	// wrapper to return whatever the server sent after CONNECT
	return &bufferedConn{conn, br}, nil
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
