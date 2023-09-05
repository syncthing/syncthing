// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package beacon

import (
	"context"
	"errors"
	"net"
	"time"

	"golang.org/x/net/ipv6"
)

func NewMulticast(addr string) Interface {
	c := newCast("multicastBeacon")
	c.addReader(func(ctx context.Context) error {
		return readMulticasts(ctx, c.outbox, addr)
	})
	c.addWriter(func(ctx context.Context) error {
		return writeMulticasts(ctx, c.inbox, addr)
	})
	return c
}

func writeMulticasts(ctx context.Context, inbox <-chan []byte, addr string) error {
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
	doneCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		<-doneCtx.Done()
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
		case <-doneCtx.Done():
			return doneCtx.Err()
		}

		intfs, err := net.Interfaces()
		if err != nil {
			l.Debugln(err)
			return err
		}

		success := 0
		for _, intf := range intfs {
			if intf.Flags&net.FlagRunning == 0 || intf.Flags&net.FlagMulticast == 0 {
				continue
			}

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
			case <-doneCtx.Done():
				return doneCtx.Err()
			default:
			}
		}

		if success == 0 {
			return err
		}
	}
}

func readMulticasts(ctx context.Context, outbox chan<- recv, addr string) error {
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
	doneCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		<-doneCtx.Done()
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
		case <-doneCtx.Done():
			return doneCtx.Err()
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
