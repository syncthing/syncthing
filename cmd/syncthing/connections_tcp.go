// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package main

import (
	"crypto/tls"
	"net"
	"net/url"
	"strings"
)

func init() {
	dialers["tcp"] = tcpDialer
	listeners["tcp"] = tcpListener
}

func tcpDialer(uri *url.URL, tlsCfg *tls.Config) (*tls.Conn, error) {
	host, port, err := net.SplitHostPort(uri.Host)
	if err != nil && strings.HasPrefix(err.Error(), "missing port") {
		// addr is on the form "1.2.3.4"
		uri.Host = net.JoinHostPort(uri.Host, "22000")
	} else if err == nil && port == "" {
		// addr is on the form "1.2.3.4:"
		uri.Host = net.JoinHostPort(host, "22000")
	}

	raddr, err := net.ResolveTCPAddr("tcp", uri.Host)
	if err != nil {
		if debugNet {
			l.Debugln(err)
		}
		return nil, err
	}

	conn, err := net.DialTCP("tcp", nil, raddr)
	if err != nil {
		if debugNet {
			l.Debugln(err)
		}
		return nil, err
	}

	setTCPOptions(conn)

	tc := tls.Client(conn, tlsCfg)
	err = tc.Handshake()
	if err != nil {
		tc.Close()
		return nil, err
	}

	return tc, nil
}

func tcpListener(uri *url.URL, tlsCfg *tls.Config, conns chan<- *tls.Conn) {
	tcaddr, err := net.ResolveTCPAddr("tcp", uri.Host)
	if err != nil {
		l.Fatalln("listen (BEP/tcp):", err)
		return
	}
	listener, err := net.ListenTCP("tcp", tcaddr)
	if err != nil {
		l.Fatalln("listen (BEP/tcp):", err)
		return
	}

	for {
		conn, err := listener.Accept()
		if err != nil {
			l.Warnln("Accepting connection (BEP/tcp):", err)
			continue
		}

		if debugNet {
			l.Debugln("connect from", conn.RemoteAddr())
		}

		tcpConn := conn.(*net.TCPConn)
		setTCPOptions(tcpConn)

		tc := tls.Server(conn, tlsCfg)
		err = tc.Handshake()
		if err != nil {
			l.Infoln("TLS handshake (BEP/tcp):", err)
			tc.Close()
			continue
		}

		conns <- tc
	}
}
