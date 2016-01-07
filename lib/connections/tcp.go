// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package connections

import (
	"crypto/tls"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/syncthing/syncthing/lib/dialer"
	"github.com/syncthing/syncthing/lib/nat"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/protocol"
)

var (
	tcpPriority = 10
)

func init() {
	dialers["tcp"] = &tcpDialer{}
	listeners["tcp"] = newTCPListener
}

type tcpDialer struct{}

func (*tcpDialer) Dial(id protocol.DeviceID, uri *url.URL, tlsCfg *tls.Config) (IntermediateConnection, error) {
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
		l.Debugln(err)
		return IntermediateConnection{}, err
	}

	conn, err := dialer.DialTimeout(raddr.Network(), raddr.String(), 10*time.Second)
	if err != nil {
		l.Debugln(err)
		return IntermediateConnection{}, err
	}

	tc := tls.Client(conn, tlsCfg)
	err = tc.Handshake()
	if err != nil {
		tc.Close()
		return IntermediateConnection{}, err
	}

	return IntermediateConnection{tc, "tcp-dial", tcpPriority}, nil
}

func (tcpDialer) Priority() int {
	return tcpPriority
}

type tcpListener struct {
	uri      *url.URL
	tlsCfg   *tls.Config
	stop     chan struct{}
	stopped  chan struct{}
	conns    chan IntermediateConnection
	listener *net.TCPListener

	natService *nat.Service
	mapping    *nat.Mapping

	address *url.URL
	err     error
	mut     sync.RWMutex
}

func (t *tcpListener) Serve() {
	t.mut.Lock()
	t.err = nil
	t.mut.Unlock()

	tcaddr, err := net.ResolveTCPAddr("tcp", t.uri.Host)
	if err != nil {
		t.mut.Lock()
		t.err = err
		t.mut.Unlock()
		l.Fatalln("listen (BEP/tcp):", err)
		return
	}

	if t.natService != nil {
		t.mapping = t.natService.NewMapping(nat.TCP, tcaddr.IP, tcaddr.Port)
	}

	t.listener, err = net.ListenTCP("tcp", tcaddr)
	if err != nil {
		t.mut.Lock()
		t.err = err
		t.mut.Unlock()
		l.Fatalln("listen (BEP/tcp):", err)
		return
	}

	t.stop = make(chan struct{})

	for {
		conn, err := t.listener.Accept()
		if err != nil {
			select {
			case <-t.stop:
				close(t.stopped)
				return
			default:
			}
			l.Warnln("Accepting connection (BEP/tcp):", err)
			continue
		}

		l.Debugln("connect from", conn.RemoteAddr())

		err = osutil.SetTCPOptions(conn.(*net.TCPConn))
		if err != nil {
			l.Infoln(err)
		}

		tc := tls.Server(conn, t.tlsCfg)
		err = tc.Handshake()
		if err != nil {
			l.Infoln("TLS handshake (BEP/tcp):", err)
			tc.Close()
			continue
		}

		t.conns <- IntermediateConnection{tc, "tcp-listen", tcpPriority}
	}
}

func (t *tcpListener) Stop() {
	t.stopped = make(chan struct{})
	close(t.stop)
	t.listener.Close()
	<-t.stopped
}

func (t *tcpListener) WANAddresses() []*url.URL {
	if t.mapping != nil {
		addrs := t.mapping.ExternalAddresses()
		uris := make([]*url.URL, len(addrs))
		for _, addr := range addrs {
			uri := *t.uri
			// Does net.JoinHostPort internally
			uri.Host = addr.String()
			uris = append(uris, &uri)
		}
		return uris
	}
	return t.LANAddresses()
}

func (t *tcpListener) LANAddresses() []*url.URL {
	return []*url.URL{t.uri}
}

func (t *tcpListener) Error() error {
	t.mut.RLock()
	err := t.err
	t.mut.RUnlock()
	return err
}

func (*tcpListener) Details() interface{} {
	return nil
}

func newTCPListener(uri *url.URL, tlsCfg *tls.Config, conns chan IntermediateConnection, natService *nat.Service) genericListener {
	fixupPort(uri)
	return &tcpListener{
		uri:        uri,
		tlsCfg:     tlsCfg,
		conns:      conns,
		natService: natService,
	}
}

func isPublicIPv4(ip net.IP) bool {
	ip = ip.To4()
	if ip == nil {
		// Not an IPv4 address (IPv6)
		return false
	}

	// IsGlobalUnicast below only checks that it's not link local or
	// multicast, and we want to exclude private (NAT:ed) addresses as well.
	rfc1918 := []net.IPNet{
		{IP: net.IP{10, 0, 0, 0}, Mask: net.IPMask{255, 0, 0, 0}},
		{IP: net.IP{172, 16, 0, 0}, Mask: net.IPMask{255, 240, 0, 0}},
		{IP: net.IP{192, 168, 0, 0}, Mask: net.IPMask{255, 255, 0, 0}},
	}
	for _, n := range rfc1918 {
		if n.Contains(ip) {
			return false
		}
	}

	return ip.IsGlobalUnicast()
}

func isPublicIPv6(ip net.IP) bool {
	if ip.To4() != nil {
		// Not an IPv6 address (IPv4)
		// (To16() returns a v6 mapped v4 address so can't be used to check
		// that it's an actual v6 address)
		return false
	}

	return ip.IsGlobalUnicast()
}

func fixupPort(uri *url.URL) {
	host, port, err := net.SplitHostPort(uri.Host)
	if err != nil && strings.HasPrefix(err.Error(), "missing port") {
		// addr is on the form "1.2.3.4"
		uri.Host = net.JoinHostPort(host, "22000")
	} else if err == nil && port == "" {
		// addr is on the form "1.2.3.4:"
		uri.Host = net.JoinHostPort(host, "22000")
	}
}
