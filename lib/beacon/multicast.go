// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package beacon

import (
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/thejerf/suture"
	"golang.org/x/net/ipv6"
	"golang.org/x/net/trace"
)

type Multicast struct {
	*suture.Supervisor
	addr   *net.UDPAddr
	inbox  chan []byte
	outbox chan recv
	mr     *multicastReader
	mw     *multicastWriter
	trace.EventLog
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
				if debug {
					l.Debugln(line)
				}
			},
		}),
		inbox:    make(chan []byte),
		outbox:   make(chan recv, 16),
		EventLog: trace.NewEventLog("beacon.Multicast", addr),
	}

	m.mr = &multicastReader{
		addr:     addr,
		outbox:   m.outbox,
		stop:     make(chan struct{}),
		EventLog: m.EventLog,
	}
	m.Add(m.mr)

	m.mw = &multicastWriter{
		addr:     addr,
		inbox:    m.inbox,
		stop:     make(chan struct{}),
		EventLog: m.EventLog,
	}
	m.Add(m.mw)

	return m
}

func (m *Multicast) Send(data []byte) {
	m.Printf("Send %d bytes", len(data))
	m.inbox <- data
}

func (m *Multicast) Recv() ([]byte, net.Addr) {
	recv := <-m.outbox
	m.Printf("Recv %d bytes from %v", len(recv.data), recv.src)
	return recv.data, recv.src
}

func (m *Multicast) Error() error {
	if err := m.mr.Error(); err != nil {
		return err
	}
	return m.mw.Error()
}

type multicastWriter struct {
	addr  string
	inbox <-chan []byte
	errorHolder
	stop chan struct{}
	trace.EventLog
}

func (w *multicastWriter) Serve() {
	if debug {
		l.Debugln(w, "starting")
		defer l.Debugln(w, "stopping")
	}
	w.Printf("%v starting", w)
	defer w.Printf("%v stopping", w)

	gaddr, err := net.ResolveUDPAddr("udp6", w.addr)
	if err != nil {
		if debug {
			l.Debugln(err)
		}
		w.setError(err)
		w.Errorf("ResolveUDPAddr: %v", err)
		return
	}

	conn, err := net.ListenPacket("udp6", ":0")
	if err != nil {
		if debug {
			l.Debugln(err)
		}
		w.setError(err)
		w.Errorf("ListenPacket: %v", err)
		return
	}

	pconn := ipv6.NewPacketConn(conn)

	wcm := &ipv6.ControlMessage{
		HopLimit: 1,
	}

	for bs := range w.inbox {
		intfs, err := net.Interfaces()
		if err != nil {
			if debug {
				l.Debugln(err)
			}
			w.setError(err)
			w.Errorf("Interfaces: %v", err)
			return
		}

		var success int

		for _, intf := range intfs {
			wcm.IfIndex = intf.Index
			pconn.SetWriteDeadline(time.Now().Add(time.Second))
			_, err = pconn.WriteTo(bs, wcm, gaddr)
			pconn.SetWriteDeadline(time.Time{})
			if err != nil && debug {
				l.Debugln(err, "on write to", gaddr, intf.Name)
				w.Errorf("WriteTo %v on %v: %v", gaddr, intf.Name, err)
				continue
			} else if debug {
				l.Debugf("sent %d bytes to %v on %s", len(bs), gaddr, intf.Name)
			}
			success++
			w.Printf("WriteTo %v on %v: %d bytes", gaddr, intf.Name, len(bs))
		}

		if success > 0 {
			w.setError(nil)
		} else {
			if debug {
				l.Debugln(err)
			}
			w.setError(err)
		}
	}
}

func (w *multicastWriter) Stop() {
	close(w.stop)
}

func (w *multicastWriter) String() string {
	return fmt.Sprintf("multicastWriter@%p", w)
}

type multicastReader struct {
	addr   string
	outbox chan<- recv
	errorHolder
	stop chan struct{}
	trace.EventLog
}

func (r *multicastReader) Serve() {
	if debug {
		l.Debugln(r, "starting")
		defer l.Debugln(r, "stopping")
	}
	r.Printf("%v starting", r)
	defer r.Printf("%v stopping", r)

	gaddr, err := net.ResolveUDPAddr("udp6", r.addr)
	if err != nil {
		if debug {
			l.Debugln(err)
		}
		r.setError(err)
		r.Errorf("ResolveUDPAddr: %v", err)
		return
	}

	conn, err := net.ListenPacket("udp6", r.addr)
	if err != nil {
		if debug {
			l.Debugln(err)
		}
		r.setError(err)
		r.Errorf("ListenPacket: %v", err)
		return
	}

	intfs, err := net.Interfaces()
	if err != nil {
		if debug {
			l.Debugln(err)
		}
		r.setError(err)
		r.Errorf("Interfaces: %v", err)
		return
	}

	pconn := ipv6.NewPacketConn(conn)
	joined := 0
	for _, intf := range intfs {
		err := pconn.JoinGroup(&intf, &net.UDPAddr{IP: gaddr.IP})
		if err != nil {
			r.Errorf("JoinGroup %v on %v: %v", gaddr.IP, intf.Name, err)
			continue
		}
		r.Printf("JoinGroup %v on %v", gaddr.IP, intf.Name)
		if debug {
			if err != nil {
				l.Debugln("IPv6 join", intf.Name, "failed:", err)
			} else {
				l.Debugln("IPv6 join", intf.Name, "success")
			}
		}
		joined++
	}

	if joined == 0 {
		if debug {
			l.Debugln("no multicast interfaces available")
		}
		r.setError(errors.New("no multicast interfaces available"))
		return
	}

	bs := make([]byte, 65536)
	for {
		n, _, addr, err := pconn.ReadFrom(bs)
		if err != nil {
			if debug {
				l.Debugln(err)
			}
			r.setError(err)
			continue
		}
		if debug {
			l.Debugf("recv %d bytes from %s", n, addr)
		}

		c := make([]byte, n)
		copy(c, bs)
		select {
		case r.outbox <- recv{c, addr}:
		default:
			if debug {
				l.Debugln("dropping message")
			}
			r.Errorf("Dropping message")
		}
	}
}

func (r *multicastReader) Stop() {
	close(r.stop)
}

func (r *multicastReader) String() string {
	return fmt.Sprintf("multicastReader@%p", r)
}
