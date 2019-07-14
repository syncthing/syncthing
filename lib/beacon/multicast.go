// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package beacon

import (
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/thejerf/suture"
	"golang.org/x/net/ipv6"

	"github.com/syncthing/syncthing/lib/util"
)

type Multicast struct {
	*suture.Supervisor
	inbox  chan []byte
	outbox chan recv
	mr     *multicastReader
	mw     *multicastWriter
}

func NewMulticast(addr string) *Multicast {
	m := &Multicast{
		Supervisor: suture.New("multicastBeacon", suture.Spec{
			// Don't retry too frenetically: an error to open a socket or
			// whatever is usually something that is either permanent or takes
			// a while to get solved...
			FailureThreshold: 2,
			FailureBackoff:   60 * time.Second,
			// Only log restarts in debug mode.
			Log: func(line string) {
				l.Debugln(line)
			},
			PassThroughPanics: true,
		}),
		inbox:  make(chan []byte),
		outbox: make(chan recv, 16),
	}

	m.mr = &multicastReader{
		addr:   addr,
		outbox: m.outbox,
	}
	m.mr.Service = util.AsService(m.mr.serve)
	m.Add(m.mr)

	m.mw = &multicastWriter{
		addr:  addr,
		inbox: m.inbox,
	}
	m.mw.Service = util.AsService(m.mw.serve)
	m.Add(m.mw)

	return m
}

func (m *Multicast) Send(data []byte) {
	m.inbox <- data
}

func (m *Multicast) Recv() ([]byte, net.Addr) {
	recv := <-m.outbox
	return recv.data, recv.src
}

func (m *Multicast) Error() error {
	if err := m.mr.Error(); err != nil {
		return err
	}
	return m.mw.Error()
}

type multicastWriter struct {
	suture.Service
	addr  string
	inbox <-chan []byte
	errorHolder
}

func (w *multicastWriter) serve(stop chan struct{}) {
	l.Debugln(w, "starting")
	defer l.Debugln(w, "stopping")

	gaddr, err := net.ResolveUDPAddr("udp6", w.addr)
	if err != nil {
		l.Debugln(err)
		w.setError(err)
		return
	}

	conn, err := net.ListenPacket("udp6", ":0")
	if err != nil {
		l.Debugln(err)
		w.setError(err)
		return
	}

	pconn := ipv6.NewPacketConn(conn)

	wcm := &ipv6.ControlMessage{
		HopLimit: 1,
	}

	for {
		var bs []byte
		select {
		case bs = <-w.inbox:
		case <-stop:
			return
		}

		intfs, err := net.Interfaces()
		if err != nil {
			l.Debugln(err)
			w.setError(err)
			return
		}

		success := 0
		for _, intf := range intfs {
			wcm.IfIndex = intf.Index
			pconn.SetWriteDeadline(time.Now().Add(time.Second))
			_, err = pconn.WriteTo(bs, wcm, gaddr)
			pconn.SetWriteDeadline(time.Time{})

			if err != nil {
				l.Debugln(err, "on write to", gaddr, intf.Name)
				w.setError(err)
				continue
			}

			l.Debugf("sent %d bytes to %v on %s", len(bs), gaddr, intf.Name)

			success++

			select {
			case <-stop:
				return
			default:
			}
		}

		if success > 0 {
			w.setError(nil)
		} else {
			l.Debugln(err)
			w.setError(err)
		}
	}
}

func (w *multicastWriter) String() string {
	return fmt.Sprintf("multicastWriter@%p", w)
}

type multicastReader struct {
	suture.Service
	addr   string
	outbox chan<- recv
	errorHolder
}

func (r *multicastReader) serve(stop chan struct{}) {
	l.Debugln(r, "starting")
	defer l.Debugln(r, "stopping")

	gaddr, err := net.ResolveUDPAddr("udp6", r.addr)
	if err != nil {
		l.Debugln(err)
		r.setError(err)
		return
	}

	conn, err := net.ListenPacket("udp6", r.addr)
	if err != nil {
		l.Debugln(err)
		r.setError(err)
		return
	}

	intfs, err := net.Interfaces()
	if err != nil {
		l.Debugln(err)
		r.setError(err)
		return
	}

	pconn := ipv6.NewPacketConn(conn)
	joined := 0
	for _, intf := range intfs {
		err := pconn.JoinGroup(&intf, &net.UDPAddr{IP: gaddr.IP})
		if err != nil {
			l.Debugln("IPv6 join", intf.Name, "failed:", err)
		} else {
			l.Debugln("IPv6 join", intf.Name, "success")
		}
		joined++
	}

	if joined == 0 {
		l.Debugln("no multicast interfaces available")
		r.setError(errors.New("no multicast interfaces available"))
		return
	}

	bs := make([]byte, 65536)
	for {
		n, _, addr, err := pconn.ReadFrom(bs)
		if err != nil {
			l.Debugln(err)
			r.setError(err)
			continue
		}
		l.Debugf("recv %d bytes from %s", n, addr)

		c := make([]byte, n)
		copy(c, bs)
		select {
		case r.outbox <- recv{c, addr}:
		case <-stop:
			return
		default:
			l.Debugln("dropping message")
		}
	}
}

func (r *multicastReader) String() string {
	return fmt.Sprintf("multicastReader@%p", r)
}
