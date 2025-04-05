// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package beacon

import (
	"context"
	"net"
	"time"
)

func NewBroadcast(port int) Interface {
	c := newCast("broadcastBeacon")
	c.addReader(func(ctx context.Context) error {
		return readBroadcasts(ctx, c.outbox, port)
	})
	c.addWriter(func(ctx context.Context) error {
		return writeBroadcasts(ctx, c.inbox, port)
	})
	return c
}

func writeBroadcasts(ctx context.Context, inbox <-chan []byte, port int) error {
	conn, err := net.ListenUDP("udp4", nil)
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

	for {
		var bs []byte
		select {
		case bs = <-inbox:
		case <-doneCtx.Done():
			return doneCtx.Err()
		}

		intfs, err := net.Interfaces()
		if err != nil {
			l.Debugln("Failed to list interfaces:", err)
			// net.Interfaces() is broken on Android. see https://github.com/golang/go/issues/40569
			// Use the general broadcast address 255.255.255.255 instead.
		}

		var dsts []net.IP
		for _, intf := range intfs {
			if intf.Flags&net.FlagRunning == 0 || intf.Flags&net.FlagBroadcast == 0 {
				continue
			}

			addrs, err := intf.Addrs()
			if err != nil {
				l.Debugln("Failed to list interface addresses:", err)
				// Interface discovery might work while retrieving the addresses doesn't. So log the error and carry on.
				continue
			}

			for _, addr := range addrs {
				if iaddr, ok := addr.(*net.IPNet); ok && len(iaddr.IP) >= 4 && iaddr.IP.IsGlobalUnicast() && iaddr.IP.To4() != nil {
					baddr := bcast(iaddr)
					dsts = append(dsts, baddr.IP)
				}
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
			l.Debugln("couldn't send any broadcasts")
			return err
		}
	}
}

func readBroadcasts(ctx context.Context, outbox chan<- recv, port int) error {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{Port: port})
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
		case <-doneCtx.Done():
			return doneCtx.Err()
		default:
			l.Debugln("dropping message")
		}
	}
}

func bcast(ip *net.IPNet) *net.IPNet {
	bc := &net.IPNet{}
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
