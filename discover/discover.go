// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package discover

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/syncthing/syncthing/beacon"
	"github.com/syncthing/syncthing/events"
	"github.com/syncthing/syncthing/protocol"
)

type Discoverer struct {
	myID             protocol.NodeID
	listenAddrs      []string
	localBcastIntv   time.Duration
	globalBcastIntv  time.Duration
	beacon           *beacon.Beacon
	registry         map[protocol.NodeID][]string
	registryLock     sync.RWMutex
	extServer        string
	extPort          uint16
	localBcastTick   <-chan time.Time
	forcedBcastTick  chan time.Time
	extAnnounceOK    bool
	extAnnounceOKmut sync.Mutex
}

var (
	ErrIncorrectMagic = errors.New("incorrect magic number")
)

// We tolerate a certain amount of errors because we might be running on
// laptops that sleep and wake, have intermittent network connectivity, etc.
// When we hit this many errors in succession, we stop.
const maxErrors = 30

func NewDiscoverer(id protocol.NodeID, addresses []string, localPort int) (*Discoverer, error) {
	b, err := beacon.New(localPort)
	if err != nil {
		return nil, err
	}
	disc := &Discoverer{
		myID:            id,
		listenAddrs:     addresses,
		localBcastIntv:  30 * time.Second,
		globalBcastIntv: 1800 * time.Second,
		beacon:          b,
		registry:        make(map[protocol.NodeID][]string),
	}

	go disc.recvAnnouncements()

	return disc, nil
}

func (d *Discoverer) StartLocal() {
	d.localBcastTick = time.Tick(d.localBcastIntv)
	d.forcedBcastTick = make(chan time.Time)
	go d.sendLocalAnnouncements()
}

func (d *Discoverer) StartGlobal(server string, extPort uint16) {
	d.extServer = server
	d.extPort = extPort
	go d.sendExternalAnnouncements()
}

func (d *Discoverer) ExtAnnounceOK() bool {
	d.extAnnounceOKmut.Lock()
	defer d.extAnnounceOKmut.Unlock()
	return d.extAnnounceOK
}

func (d *Discoverer) Lookup(node protocol.NodeID) []string {
	d.registryLock.Lock()
	addr, ok := d.registry[node]
	d.registryLock.Unlock()

	if ok {
		return addr
	} else if len(d.extServer) != 0 {
		// We might want to cache this, but not permanently so it needs some intelligence
		return d.externalLookup(node)
	}
	return nil
}

func (d *Discoverer) Hint(node string, addrs []string) {
	resAddrs := resolveAddrs(addrs)
	var id protocol.NodeID
	id.UnmarshalText([]byte(node))
	d.registerNode(nil, Node{
		Addresses: resAddrs,
		ID:        id[:],
	})
}

func (d *Discoverer) All() map[protocol.NodeID][]string {
	d.registryLock.RLock()
	nodes := make(map[protocol.NodeID][]string, len(d.registry))
	for node, addrs := range d.registry {
		addrsCopy := make([]string, len(addrs))
		copy(addrsCopy, addrs)
		nodes[node] = addrsCopy
	}
	d.registryLock.RUnlock()
	return nodes
}

func (d *Discoverer) announcementPkt() []byte {
	var addrs []Address
	for _, astr := range d.listenAddrs {
		addr, err := net.ResolveTCPAddr("tcp", astr)
		if err != nil {
			l.Warnln("%v: not announcing %s", err, astr)
			continue
		} else if debug {
			l.Debugf("discover: announcing %s: %#v", astr, addr)
		}
		if len(addr.IP) == 0 || addr.IP.IsUnspecified() {
			addrs = append(addrs, Address{Port: uint16(addr.Port)})
		} else if bs := addr.IP.To4(); bs != nil {
			addrs = append(addrs, Address{IP: bs, Port: uint16(addr.Port)})
		} else if bs := addr.IP.To16(); bs != nil {
			addrs = append(addrs, Address{IP: bs, Port: uint16(addr.Port)})
		}
	}
	var pkt = Announce{
		Magic: AnnouncementMagic,
		This:  Node{d.myID[:], addrs},
	}
	return pkt.MarshalXDR()
}

func (d *Discoverer) sendLocalAnnouncements() {
	var addrs = resolveAddrs(d.listenAddrs)

	var pkt = Announce{
		Magic: AnnouncementMagic,
		This:  Node{d.myID[:], addrs},
	}

	for {
		pkt.Extra = nil
		d.registryLock.RLock()
		for node, addrs := range d.registry {
			if len(pkt.Extra) == 16 {
				break
			}

			anode := Node{node[:], resolveAddrs(addrs)}
			pkt.Extra = append(pkt.Extra, anode)
		}
		d.registryLock.RUnlock()

		d.beacon.Send(pkt.MarshalXDR())

		select {
		case <-d.localBcastTick:
		case <-d.forcedBcastTick:
		}
	}
}

func (d *Discoverer) sendExternalAnnouncements() {
	// this should go in the Discoverer struct
	errorRetryIntv := 60 * time.Second

	remote, err := net.ResolveUDPAddr("udp", d.extServer)
	for err != nil {
		l.Warnf("Global discovery: %v; trying again in %v", err, errorRetryIntv)
		time.Sleep(errorRetryIntv)
		remote, err = net.ResolveUDPAddr("udp", d.extServer)
	}

	conn, err := net.ListenUDP("udp", nil)
	for err != nil {
		l.Warnf("Global discovery: %v; trying again in %v", err, errorRetryIntv)
		time.Sleep(errorRetryIntv)
		conn, err = net.ListenUDP("udp", nil)
	}

	var buf []byte
	if d.extPort != 0 {
		var pkt = Announce{
			Magic: AnnouncementMagic,
			This:  Node{d.myID[:], []Address{{Port: d.extPort}}},
		}
		buf = pkt.MarshalXDR()
	} else {
		buf = d.announcementPkt()
	}

	for {
		var ok bool

		if debug {
			l.Debugf("discover: send announcement -> %v\n%s", remote, hex.Dump(buf))
		}

		_, err := conn.WriteTo(buf, remote)
		if err != nil {
			if debug {
				l.Debugln("discover: warning:", err)
			}
			ok = false
		} else {
			// Verify that the announce server responds positively for our node ID

			time.Sleep(1 * time.Second)
			res := d.externalLookup(d.myID)
			if debug {
				l.Debugln("discover: external lookup check:", res)
			}
			ok = len(res) > 0
		}

		d.extAnnounceOKmut.Lock()
		d.extAnnounceOK = ok
		d.extAnnounceOKmut.Unlock()

		if ok {
			time.Sleep(d.globalBcastIntv)
		} else {
			time.Sleep(errorRetryIntv)
		}
	}
}

func (d *Discoverer) recvAnnouncements() {
	for {
		buf, addr := d.beacon.Recv()

		if debug {
			l.Debugf("discover: read announcement:\n%s", hex.Dump(buf))
		}

		var pkt Announce
		err := pkt.UnmarshalXDR(buf)
		if err != nil && err != io.EOF {
			continue
		}

		if debug {
			l.Debugf("discover: parsed announcement: %#v", pkt)
		}

		var newNode bool
		if bytes.Compare(pkt.This.ID, d.myID[:]) != 0 {
			newNode = d.registerNode(addr, pkt.This)
			for _, node := range pkt.Extra {
				if bytes.Compare(node.ID, d.myID[:]) != 0 {
					if d.registerNode(nil, node) {
						newNode = true
					}
				}
			}
		}

		if newNode {
			select {
			case d.forcedBcastTick <- time.Now():
			}
		}
	}
}

func (d *Discoverer) registerNode(addr net.Addr, node Node) bool {
	var addrs []string
	for _, a := range node.Addresses {
		var nodeAddr string
		if len(a.IP) > 0 {
			nodeAddr = fmt.Sprintf("%s:%d", net.IP(a.IP), a.Port)
			addrs = append(addrs, nodeAddr)
		} else if addr != nil {
			ua := addr.(*net.UDPAddr)
			ua.Port = int(a.Port)
			nodeAddr = ua.String()
			addrs = append(addrs, nodeAddr)
		}
	}
	if len(addrs) == 0 {
		if debug {
			l.Debugln("discover: no valid address for", node.ID)
		}
	}
	if debug {
		l.Debugf("discover: register: %s -> %#v", node.ID, addrs)
	}
	var id protocol.NodeID
	copy(id[:], node.ID)
	d.registryLock.Lock()
	_, seen := d.registry[id]
	d.registry[id] = addrs
	d.registryLock.Unlock()

	if !seen {
		events.Default.Log(events.NodeDiscovered, map[string]interface{}{
			"node":  id.String(),
			"addrs": addrs,
		})
	}
	return !seen
}

func (d *Discoverer) externalLookup(node protocol.NodeID) []string {
	extIP, err := net.ResolveUDPAddr("udp", d.extServer)
	if err != nil {
		if debug {
			l.Debugf("discover: %v; no external lookup", err)
		}
		return nil
	}

	conn, err := net.DialUDP("udp", nil, extIP)
	if err != nil {
		if debug {
			l.Debugf("discover: %v; no external lookup", err)
		}
		return nil
	}
	defer conn.Close()

	err = conn.SetDeadline(time.Now().Add(5 * time.Second))
	if err != nil {
		if debug {
			l.Debugf("discover: %v; no external lookup", err)
		}
		return nil
	}

	buf := Query{QueryMagic, node[:]}.MarshalXDR()
	_, err = conn.Write(buf)
	if err != nil {
		if debug {
			l.Debugf("discover: %v; no external lookup", err)
		}
		return nil
	}

	buf = make([]byte, 2048)
	n, err := conn.Read(buf)
	if err != nil {
		if err, ok := err.(net.Error); ok && err.Timeout() {
			// Expected if the server doesn't know about requested node ID
			return nil
		}
		if debug {
			l.Debugf("discover: %v; no external lookup", err)
		}
		return nil
	}

	if debug {
		l.Debugf("discover: read external:\n%s", hex.Dump(buf[:n]))
	}

	var pkt Announce
	err = pkt.UnmarshalXDR(buf[:n])
	if err != nil && err != io.EOF {
		if debug {
			l.Debugln("discover:", err)
		}
		return nil
	}

	if debug {
		l.Debugf("discover: parsed external: %#v", pkt)
	}

	var addrs []string
	for _, a := range pkt.This.Addresses {
		nodeAddr := fmt.Sprintf("%s:%d", net.IP(a.IP), a.Port)
		addrs = append(addrs, nodeAddr)
	}
	return addrs
}

func addrToAddr(addr *net.TCPAddr) Address {
	if len(addr.IP) == 0 || addr.IP.IsUnspecified() {
		return Address{Port: uint16(addr.Port)}
	} else if bs := addr.IP.To4(); bs != nil {
		return Address{IP: bs, Port: uint16(addr.Port)}
	} else if bs := addr.IP.To16(); bs != nil {
		return Address{IP: bs, Port: uint16(addr.Port)}
	}
	return Address{}
}

func resolveAddrs(addrs []string) []Address {
	var raddrs []Address
	for _, addrStr := range addrs {
		addrRes, err := net.ResolveTCPAddr("tcp", addrStr)
		if err != nil {
			continue
		}
		addr := addrToAddr(addrRes)
		if len(addr.IP) > 0 {
			raddrs = append(raddrs, addr)
		} else {
			raddrs = append(raddrs, Address{Port: addr.Port})
		}
	}
	return raddrs
}
