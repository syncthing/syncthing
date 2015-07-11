// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package beacon

import (
	"fmt"
	"net"
	"time"

	"github.com/thejerf/suture"
)

type Broadcast struct {
	*suture.Supervisor
	port   int
	inbox  chan []byte
	outbox chan recv
}

func NewBroadcast(port int) *Broadcast {
	b := &Broadcast{
		Supervisor: suture.New("broadcastBeacon", suture.Spec{
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
		port:   port,
		inbox:  make(chan []byte),
		outbox: make(chan recv, 16),
	}

	b.Add(&broadcastReader{
		port:   port,
		outbox: b.outbox,
	})
	b.Add(&broadcastWriter{
		port:  port,
		inbox: b.inbox,
	})

	return b
}

func (b *Broadcast) Send(data []byte) {
	b.inbox <- data
}

func (b *Broadcast) Recv() ([]byte, net.Addr) {
	recv := <-b.outbox
	return recv.data, recv.src
}

type broadcastWriter struct {
	port   int
	inbox  chan []byte
	conn   *net.UDPConn
	failed bool // Have we already logged a failure reason?
}

func (w *broadcastWriter) Serve() {
	if debug {
		l.Debugln(w, "starting")
		defer l.Debugln(w, "stopping")
	}

	var err error
	w.conn, err = net.ListenUDP("udp4", nil)
	if err != nil {
		if !w.failed {
			l.Warnln("Local discovery over IPv4 unavailable:", err)
			w.failed = true
		}
		return
	}
	defer w.conn.Close()

	w.failed = false

	for bs := range w.inbox {
		addrs, err := net.InterfaceAddrs()
		if err != nil {
			if debug {
				l.Debugln("Local discovery (broadcast writer):", err)
			}
			continue
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

		if debug {
			l.Debugln("addresses:", dsts)
		}

		for _, ip := range dsts {
			dst := &net.UDPAddr{IP: ip, Port: w.port}

			w.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			_, err := w.conn.WriteTo(bs, dst)
			if err, ok := err.(net.Error); ok && err.Timeout() {
				// Write timeouts should not happen. We treat it as a fatal
				// error on the socket.
				l.Infoln("Local discovery (broadcast writer):", err)
				w.failed = true
				return
			} else if err, ok := err.(net.Error); ok && err.Temporary() {
				// A transient error. Lets hope for better luck in the future.
				if debug {
					l.Debugln(err)
				}
				continue
			} else if err != nil {
				// Some other error that we don't expect. Bail and retry.
				l.Infoln("Local discovery (broadcast writer):", err)
				w.failed = true
				return
			} else if debug {
				l.Debugf("sent %d bytes to %s", len(bs), dst)
			}
		}
	}
}

func (w *broadcastWriter) Stop() {
	w.conn.Close()
}

func (w *broadcastWriter) String() string {
	return fmt.Sprintf("broadcastWriter@%p", w)
}

type broadcastReader struct {
	port   int
	outbox chan recv
	conn   *net.UDPConn
	failed bool
}

func (r *broadcastReader) Serve() {
	if debug {
		l.Debugln(r, "starting")
		defer l.Debugln(r, "stopping")
	}

	var err error
	r.conn, err = net.ListenUDP("udp4", &net.UDPAddr{Port: r.port})
	if err != nil {
		if !r.failed {
			l.Warnln("Local discovery over IPv4 unavailable:", err)
			r.failed = true
		}
		return
	}
	defer r.conn.Close()

	bs := make([]byte, 65536)
	for {
		n, addr, err := r.conn.ReadFrom(bs)
		if err != nil {
			if !r.failed {
				l.Infoln("Local discovery (broadcast reader):", err)
				r.failed = true
			}
			return
		}

		r.failed = false

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
		}
	}

}

func (r *broadcastReader) Stop() {
	r.conn.Close()
}

func (r *broadcastReader) String() string {
	return fmt.Sprintf("broadcastReader@%p", r)
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
