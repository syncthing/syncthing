// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package discover

import (
	"encoding/hex"
	"io"
	"net"
	"net/url"
	"strconv"
	"time"

	"github.com/syncthing/protocol"
	"github.com/syncthing/syncthing/lib/sync"
)

func init() {
	for _, proto := range []string{"udp", "udp4", "udp6"} {
		Register(proto, func(uri *url.URL, announcer Announcer) (Client, error) {
			c := &UDPClient{
				announcer: announcer,
				wg:        sync.NewWaitGroup(),
				mut:       sync.NewRWMutex(),
			}
			err := c.Start(uri)
			if err != nil {
				return nil, err
			}
			return c, nil
		})
	}
}

type UDPClient struct {
	url *url.URL

	stop          chan struct{}
	wg            sync.WaitGroup
	listenAddress *net.UDPAddr

	globalBroadcastInterval time.Duration
	errorRetryInterval      time.Duration
	announcer               Announcer

	status bool
	mut    sync.RWMutex
}

func (d *UDPClient) Start(uri *url.URL) error {
	d.url = uri
	d.stop = make(chan struct{})

	params := uri.Query()
	// The address must not have a port, as otherwise both announce and lookup
	// sockets would try to bind to the same port.
	addr, err := net.ResolveUDPAddr(d.url.Scheme, params.Get("listenaddress")+":0")
	if err != nil {
		return err
	}
	d.listenAddress = addr

	broadcastSeconds, err := strconv.ParseUint(params.Get("broadcast"), 0, 0)
	if err != nil {
		d.globalBroadcastInterval = DefaultGlobalBroadcastInterval
	} else {
		d.globalBroadcastInterval = time.Duration(broadcastSeconds) * time.Second
	}

	retrySeconds, err := strconv.ParseUint(params.Get("retry"), 0, 0)
	if err != nil {
		d.errorRetryInterval = DefaultErrorRetryInternval
	} else {
		d.errorRetryInterval = time.Duration(retrySeconds) * time.Second
	}

	d.wg.Add(1)
	go d.broadcast()
	return nil
}

func (d *UDPClient) broadcast() {
	defer d.wg.Done()

	conn, err := net.ListenUDP(d.url.Scheme, d.listenAddress)
	for err != nil {
		if debug {
			l.Debugf("discover %s: broadcast listen: %v; trying again in %v", d.url, err, d.errorRetryInterval)
		}
		select {
		case <-d.stop:
			return
		case <-time.After(d.errorRetryInterval):
		}
		conn, err = net.ListenUDP(d.url.Scheme, d.listenAddress)
	}
	defer conn.Close()

	remote, err := net.ResolveUDPAddr(d.url.Scheme, d.url.Host)
	for err != nil {
		if debug {
			l.Debugf("discover %s: broadcast resolve: %v; trying again in %v", d.url, err, d.errorRetryInterval)
		}
		select {
		case <-d.stop:
			return
		case <-time.After(d.errorRetryInterval):
		}
		remote, err = net.ResolveUDPAddr(d.url.Scheme, d.url.Host)
	}

	timer := time.NewTimer(0)
	for {
		select {
		case <-d.stop:
			return

		case <-timer.C:
			var ok bool

			if debug {
				l.Debugf("discover %s: broadcast: Sending self announcement to %v", d.url, remote)
			}

			ann := d.announcer.Announcement()
			pkt, err := ann.MarshalXDR()
			if err != nil {
				timer.Reset(d.errorRetryInterval)
				continue
			}

			_, err = conn.WriteTo(pkt, remote)
			if err != nil {
				if debug {
					l.Debugf("discover %s: broadcast: Failed to send self announcement: %s", d.url, err)
				}
				ok = false
			} else {
				// Verify that the announce server responds positively for our device ID

				time.Sleep(1 * time.Second)

				pkt, err := d.Lookup(protocol.DeviceIDFromBytes(ann.This.ID))
				if err != nil && debug {
					l.Debugf("discover %s: broadcast: Self-lookup failed: %v", d.url, err)
				} else if debug {
					l.Debugf("discover %s: broadcast: Self-lookup returned: %v", d.url, pkt.This.Addresses)
				}
				ok = len(pkt.This.Addresses) > 0
			}

			d.mut.Lock()
			d.status = ok
			d.mut.Unlock()

			if ok {
				timer.Reset(d.globalBroadcastInterval)
			} else {
				timer.Reset(d.errorRetryInterval)
			}
		}
	}
}

func (d *UDPClient) Lookup(device protocol.DeviceID) (Announce, error) {
	extIP, err := net.ResolveUDPAddr(d.url.Scheme, d.url.Host)
	if err != nil {
		if debug {
			l.Debugf("discover %s: Lookup(%s): %s", d.url, device, err)
		}
		return Announce{}, err
	}

	conn, err := net.DialUDP(d.url.Scheme, d.listenAddress, extIP)
	if err != nil {
		if debug {
			l.Debugf("discover %s: Lookup(%s): %s", d.url, device, err)
		}
		return Announce{}, err
	}
	defer conn.Close()

	err = conn.SetDeadline(time.Now().Add(5 * time.Second))
	if err != nil {
		if debug {
			l.Debugf("discover %s: Lookup(%s): %s", d.url, device, err)
		}
		return Announce{}, err
	}

	buf := Query{QueryMagic, device[:]}.MustMarshalXDR()
	_, err = conn.Write(buf)
	if err != nil {
		if debug {
			l.Debugf("discover %s: Lookup(%s): %s", d.url, device, err)
		}
		return Announce{}, err
	}

	buf = make([]byte, 2048)
	n, err := conn.Read(buf)
	if err != nil {
		if err, ok := err.(net.Error); ok && err.Timeout() {
			// Expected if the server doesn't know about requested device ID
			return Announce{}, err
		}
		if debug {
			l.Debugf("discover %s: Lookup(%s): %s", d.url, device, err)
		}
		return Announce{}, err
	}

	var pkt Announce
	err = pkt.UnmarshalXDR(buf[:n])
	if err != nil && err != io.EOF {
		if debug {
			l.Debugf("discover %s: Lookup(%s): %s\n%s", d.url, device, err, hex.Dump(buf[:n]))
		}
		return Announce{}, err
	}

	if debug {
		l.Debugf("discover %s: Lookup(%s) result: %v relays: %v", d.url, device, pkt.This.Addresses, pkt.This.Relays)
	}
	return pkt, nil
}

func (d *UDPClient) Stop() {
	if d.stop != nil {
		close(d.stop)
		d.wg.Wait()
	}
}

func (d *UDPClient) StatusOK() bool {
	d.mut.RLock()
	defer d.mut.RUnlock()
	return d.status
}

func (d *UDPClient) Address() string {
	return d.url.String()
}
