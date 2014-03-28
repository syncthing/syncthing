package discover

import (
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/calmh/syncthing/buffers"
	"github.com/calmh/syncthing/mc"
)

const (
	AnnouncementPort = 21025
)

type Discoverer struct {
	MyID                string
	ListenAddresses     []string
	BroadcastIntv       time.Duration
	ExtBroadcastIntv    time.Duration
	beacon              *mc.Beacon
	registry            map[string][]string
	registryLock        sync.RWMutex
	extServer           string
	localBroadcastTick  <-chan time.Time
	forcedBroadcastTick chan time.Time
}

var (
	ErrIncorrectMagic = errors.New("incorrect magic number")
)

// We tolerate a certain amount of errors because we might be running on
// laptops that sleep and wake, have intermittent network connectivity, etc.
// When we hit this many errors in succession, we stop.
const maxErrors = 30

func NewDiscoverer(id string, addresses []string, extServer string) (*Discoverer, error) {
	disc := &Discoverer{
		MyID:             id,
		ListenAddresses:  addresses,
		BroadcastIntv:    30 * time.Second,
		ExtBroadcastIntv: 1800 * time.Second,
		beacon:           mc.NewBeacon("239.21.0.25", 21025),
		registry:         make(map[string][]string),
		extServer:        extServer,
	}

	// Receive announcements sent to the local multicast group.

	go disc.recvAnnouncements()

	// If we got a list of addresses that we listen on, announce those
	// locally.

	if len(disc.ListenAddresses) > 0 {
		disc.localBroadcastTick = time.Tick(disc.BroadcastIntv)
		disc.forcedBroadcastTick = make(chan time.Time)
		go disc.sendLocalAnnouncements()

		// If we have an external server address, also announce to that
		// server.

		if len(disc.extServer) > 0 {
			go disc.sendExternalAnnouncements()
		}
	}

	return disc, nil
}

func (d *Discoverer) announcementPkt() []byte {
	var addrs []Address
	for _, astr := range d.ListenAddresses {
		addr, err := net.ResolveTCPAddr("tcp", astr)
		if err != nil {
			log.Printf("discover/announcement: %v: not announcing %s", err, astr)
			continue
		} else if debug {
			dlog.Printf("announcing %s: %#v", astr, addr)
		}
		if len(addr.IP) == 0 || addr.IP.IsUnspecified() {
			addrs = append(addrs, Address{Port: uint16(addr.Port)})
		} else if bs := addr.IP.To4(); bs != nil {
			addrs = append(addrs, Address{IP: bs, Port: uint16(addr.Port)})
		} else if bs := addr.IP.To16(); bs != nil {
			addrs = append(addrs, Address{IP: bs, Port: uint16(addr.Port)})
		}
	}
	var pkt = AnnounceV2{
		Magic:     AnnouncementMagicV2,
		NodeID:    d.MyID,
		Addresses: addrs,
	}
	return pkt.MarshalXDR()
}

func (d *Discoverer) sendLocalAnnouncements() {
	var buf = d.announcementPkt()

	for {
		d.beacon.Send(buf)

		select {
		case <-d.localBroadcastTick:
		case <-d.forcedBroadcastTick:
		}
	}
}

func (d *Discoverer) sendExternalAnnouncements() {
	remote, err := net.ResolveUDPAddr("udp", d.extServer)
	if err != nil {
		log.Printf("discover/external: %v; no external announcements", err)
		return
	}

	conn, err := net.ListenUDP("udp", nil)
	if err != nil {
		log.Printf("discover/external: %v; no external announcements", err)
		return
	}

	var buf = d.announcementPkt()
	var errCounter = 0

	for errCounter < maxErrors {
		if debug {
			dlog.Println("send announcement -> ", remote)
		}
		_, err = conn.WriteTo(buf, remote)
		if err != nil {
			log.Println("discover/write: warning:", err)
			errCounter++
		} else {
			errCounter = 0
		}
		time.Sleep(d.ExtBroadcastIntv)
	}
	log.Printf("discover/write: %v: stopping due to too many errors: %v", remote, err)
}

func (d *Discoverer) recvAnnouncements() {
	for {
		buf, addr := d.beacon.Recv()

		if debug {
			dlog.Printf("read announcement:\n%s", hex.Dump(buf))
		}

		var pkt AnnounceV2
		err := pkt.UnmarshalXDR(buf)
		if err != nil {
			continue
		}

		if debug {
			dlog.Printf("parsed announcement: %#v", pkt)
		}

		if pkt.NodeID != d.MyID {
			var addrs []string
			for _, a := range pkt.Addresses {
				var nodeAddr string
				if len(a.IP) > 0 {
					nodeAddr = fmt.Sprintf("%s:%d", ipStr(a.IP), a.Port)
				} else {
					ua := addr.(*net.UDPAddr)
					ua.Port = int(a.Port)
					nodeAddr = ua.String()
				}
				addrs = append(addrs, nodeAddr)
			}
			if debug {
				dlog.Printf("register: %#v", addrs)
			}
			d.registryLock.Lock()
			_, seen := d.registry[pkt.NodeID]
			if !seen {
				select {
				case d.forcedBroadcastTick <- time.Now():
				}
			}
			d.registry[pkt.NodeID] = addrs
			d.registryLock.Unlock()
		}
	}
}

func (d *Discoverer) externalLookup(node string) []string {
	extIP, err := net.ResolveUDPAddr("udp", d.extServer)
	if err != nil {
		log.Printf("discover/external: %v; no external lookup", err)
		return nil
	}

	conn, err := net.DialUDP("udp", nil, extIP)
	if err != nil {
		log.Printf("discover/external: %v; no external lookup", err)
		return nil
	}
	defer conn.Close()

	err = conn.SetDeadline(time.Now().Add(5 * time.Second))
	if err != nil {
		log.Printf("discover/external: %v; no external lookup", err)
		return nil
	}

	buf := QueryV2{QueryMagicV2, node}.MarshalXDR()
	_, err = conn.Write(buf)
	if err != nil {
		log.Printf("discover/external: %v; no external lookup", err)
		return nil
	}
	buffers.Put(buf)

	buf = buffers.Get(256)
	defer buffers.Put(buf)

	n, err := conn.Read(buf)
	if err != nil {
		if err, ok := err.(net.Error); ok && err.Timeout() {
			// Expected if the server doesn't know about requested node ID
			return nil
		}
		log.Printf("discover/external/read: %v; no external lookup", err)
		return nil
	}

	if debug {
		dlog.Printf("read external:\n%s", hex.Dump(buf[:n]))
	}

	var pkt AnnounceV2
	err = pkt.UnmarshalXDR(buf[:n])
	if err != nil {
		log.Println("discover/external/decode:", err)
		return nil
	}

	if debug {
		dlog.Printf("parsed external: %#v", pkt)
	}

	var addrs []string
	for _, a := range pkt.Addresses {
		var nodeAddr string
		if len(a.IP) > 0 {
			nodeAddr = fmt.Sprintf("%s:%d", ipStr(a.IP), a.Port)
		}
		addrs = append(addrs, nodeAddr)
	}
	return addrs
}

func (d *Discoverer) Lookup(node string) []string {
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

func ipStr(ip []byte) string {
	var f = "%d"
	var s = "."
	if len(ip) > 4 {
		f = "%x"
		s = ":"
	}
	var ss = make([]string, len(ip))
	for i := range ip {
		ss[i] = fmt.Sprintf(f, ip[i])
	}
	return strings.Join(ss, s)
}
