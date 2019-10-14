// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package beacon

import (
	"errors"
	"net"
	"time"

	"golang.org/x/net/ipv6"
)

func NewMulticast(addr string) Interface {
	c := newCast("multicastBeacon")
	c.addReader(func(stop chan struct{}) error {
		return readMulticasts(c.outbox, addr, stop)
	})
	c.addWriter(func(stop chan struct{}) error {
		return writeMulticasts(c.inbox, addr, stop)
	})
	return c
}

func writeMulticasts(inbox <-chan []byte, addr string, stop chan struct{}) error {
	gaddr, err := net.ResolveUDPAddr("udp6", addr)
	if err != nil {
		l.Debugln(err)
		return err
	}

	conn, err := net.ListenPacket("udp6", ":0")
	if err != nil {
		l.Debugln(err)
		return err
	}
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-stop:
		case <-done:
		}
		conn.Close()
	}()

	pconn := ipv6.NewPacketConn(conn)

	wcm := &ipv6.ControlMessage{
		HopLimit: 1,
	}

	for {
		var bs []byte
		select {
		case bs = <-inbox:
		case <-stop:
			return nil
		}

		intfs, err := net.Interfaces()
		if err != nil {
			l.Debugln(err)
			return err
		}

		success := 0
		for _, intf := range intfs {
			wcm.IfIndex = intf.Index
			pconn.SetWriteDeadline(time.Now().Add(time.Second))
			_, err = pconn.WriteTo(bs, wcm, gaddr)
			pconn.SetWriteDeadline(time.Time{})

			if err != nil {
				l.Debugln(err, "on write to", gaddr, intf.Name)
				continue
			}

			l.Debugf("sent %d bytes to %v on %s", len(bs), gaddr, intf.Name)

			success++

			select {
			case <-stop:
				return nil
			default:
			}
		}

		if success == 0 {
			return err
		}
	}
}

func readMulticasts(outbox chan<- recv, addr string, stop chan struct{}) error {
	gaddr, err := net.ResolveUDPAddr("udp6", addr)
	if err != nil {
		l.Debugln(err)
		return err
	}

	conn, err := net.ListenPacket("udp6", addr)
	if err != nil {
		l.Debugln(err)
		return err
	}
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-stop:
		case <-done:
		}
		conn.Close()
	}()

	intfs, err := net.Interfaces()
	if err != nil {
		l.Debugln(err)
		return err
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
		return errors.New("no multicast interfaces available")
	}

	bs := make([]byte, 65536)
	for {
		select {
		case <-stop:
			return nil
		default:
		}
		n, _, addr, err := pconn.ReadFrom(bs)
		if err != nil {
			l.Debugln(err)
			return err
		}
		l.Debugf("recv %d bytes from %s", n, addr)

		c := make([]byte, n)
		copy(c, bs)
		select {
		case outbox <- recv{c, addr}:
		default:
			l.Debugln("dropping message")
		}
	}
}
