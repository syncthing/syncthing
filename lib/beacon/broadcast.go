// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package beacon

import (
	"net"
	"time"
)

func NewBroadcast(port int) Interface {
	c := newCast("broadcastBeacon")
	c.addReader(func(stop chan struct{}) error {
		return readBroadcasts(c.outbox, port, stop)
	})
	c.addWriter(func(stop chan struct{}) error {
		return writeBroadcasts(c.inbox, port, stop)
	})
	return c
}

func writeBroadcasts(inbox <-chan []byte, port int, stop chan struct{}) error {
	conn, err := net.ListenUDP("udp4", nil)
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

	for {
		var bs []byte
		select {
		case bs = <-inbox:
		case <-stop:
			return nil
		}

		addrs, err := net.InterfaceAddrs()
		if err != nil {
			l.Debugln(err)
			return err
		}

		var dsts []net.IP
		for _, addr := range addrs {
			if iaddr, ok := addr.(*net.IPNet); ok && len(iaddr.IP) >= 4 && iaddr.IP.IsGlobalUnicast() && iaddr.IP.To4() != nil {
				baddr := bcast(iaddr)
				dsts = append(dsts, baddr.IP)
			}
		}

		if len(dsts) == 0 {
			// Fall back to the general IPv4 broadcast address
			dsts = append(dsts, net.IP{0xff, 0xff, 0xff, 0xff})
		}

		l.Debugln("addresses:", dsts)

		success := 0
		for _, ip := range dsts {
			dst := &net.UDPAddr{IP: ip, Port: port}

			conn.SetWriteDeadline(time.Now().Add(time.Second))
			_, err = conn.WriteTo(bs, dst)
			conn.SetWriteDeadline(time.Time{})

			if nerr, ok := err.(net.Error); ok && nerr.Timeout() {
				// Write timeouts should not happen. We treat it as a fatal
				// error on the socket.
				l.Debugln(err)
				return err
			}

			if err != nil {
				// Some other error that we don't expect. Debug and continue.
				l.Debugln(err)
				continue
			}

			l.Debugf("sent %d bytes to %s", len(bs), dst)
			success++
		}

		if success == 0 {
			l.Debugln("couldn't send any braodcasts")
			return err
		}
	}
}

func readBroadcasts(outbox chan<- recv, port int, stop chan struct{}) error {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{Port: port})
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

	bs := make([]byte, 65536)
	for {
		n, addr, err := conn.ReadFrom(bs)
		if err != nil {
			l.Debugln(err)
			return err
		}

		l.Debugf("recv %d bytes from %s", n, addr)

		c := make([]byte, n)
		copy(c, bs)
		select {
		case outbox <- recv{c, addr}:
		case <-stop:
			return nil
		default:
			l.Debugln("dropping message")
		}
	}
}

func bcast(ip *net.IPNet) *net.IPNet {
	var bc = &net.IPNet{}
	bc.IP = make([]byte, len(ip.IP))
	copy(bc.IP, ip.IP)
	bc.Mask = ip.Mask

	offset := len(bc.IP) - len(bc.Mask)
	for i := range bc.IP {
		if i-offset >= 0 {
			bc.IP[i] = ip.IP[i] | ^ip.Mask[i-offset]
		}
	}
	return bc
}
