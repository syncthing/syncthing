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
)

const (
	AnnouncementPort = 21025
	Debug            = false
)

type Discoverer struct {
	MyID             string
	ListenPort       int
	BroadcastIntv    time.Duration
	ExtBroadcastIntv time.Duration

	conn         *net.UDPConn
	registry     map[string][]string
	registryLock sync.RWMutex
	extServer    string
}

var (
	ErrIncorrectMagic = errors.New("Incorrect magic number")
)

// We tolerate a certain amount of errors because we might be running on
// laptops that sleep and wake, have intermittent network connectivity, etc.
// When we hit this many errors in succession, we stop.
const maxErrors = 30

func NewDiscoverer(id string, port int, extServer string) (*Discoverer, error) {
	local4 := &net.UDPAddr{IP: net.IP{0, 0, 0, 0}, Port: AnnouncementPort}
	conn, err := net.ListenUDP("udp4", local4)
	if err != nil {
		return nil, err
	}

	disc := &Discoverer{
		MyID:             id,
		ListenPort:       port,
		BroadcastIntv:    30 * time.Second,
		ExtBroadcastIntv: 1800 * time.Second,

		conn:      conn,
		registry:  make(map[string][]string),
		extServer: extServer,
	}

	go disc.recvAnnouncements()

	if disc.ListenPort > 0 {
		disc.sendAnnouncements()
	}
	if len(disc.extServer) > 0 {
		disc.sendExtAnnouncements()
	}

	return disc, nil
}

func (d *Discoverer) sendAnnouncements() {
	remote4 := &net.UDPAddr{IP: net.IP{255, 255, 255, 255}, Port: AnnouncementPort}

	pkt := AnnounceV2{AnnouncementMagicV2, d.MyID, []Address{{nil, 22000}}}
	buf := pkt.MarshalXDR()
	go d.writeAnnouncements(buf, remote4, d.BroadcastIntv)
}

func (d *Discoverer) sendExtAnnouncements() {
	extIP, err := net.ResolveUDPAddr("udp", d.extServer)
	if err != nil {
		log.Printf("discover/external: %v; no external announcements", err)
		return
	}

	pkt := AnnounceV2{AnnouncementMagicV2, d.MyID, []Address{{nil, 22000}}}
	buf := pkt.MarshalXDR()
	go d.writeAnnouncements(buf, extIP, d.ExtBroadcastIntv)
}

func (d *Discoverer) writeAnnouncements(buf []byte, remote *net.UDPAddr, intv time.Duration) {
	var errCounter = 0
	var err error
	for errCounter < maxErrors {
		_, _, err = d.conn.WriteMsgUDP(buf, nil, remote)
		if err != nil {
			log.Println("discover/write: warning:", err)
			errCounter++
		} else {
			errCounter = 0
		}
		time.Sleep(intv)
	}
	log.Println("discover/write: %v: stopping due to too many errors:", remote, err)
}

func (d *Discoverer) recvAnnouncements() {
	var buf = make([]byte, 1024)
	var errCounter = 0
	var err error
	for errCounter < maxErrors {
		n, addr, err := d.conn.ReadFromUDP(buf)
		if err != nil {
			errCounter++
			time.Sleep(time.Second)
			continue
		}

		if Debug {
			log.Printf("read announcement:\n%s", hex.Dump(buf[:n]))
		}

		var pkt AnnounceV2
		err = pkt.UnmarshalXDR(buf[:n])
		if err != nil {
			errCounter++
			time.Sleep(time.Second)
			continue
		}

		if Debug {
			log.Printf("read announcement: %#v", pkt)
		}

		errCounter = 0

		if pkt.NodeID != d.MyID {
			var addrs []string
			for _, a := range pkt.Addresses {
				var nodeAddr string
				if len(a.IP) > 0 {
					nodeAddr = fmt.Sprintf("%s:%d", ipStr(a.IP), a.Port)
				} else {
					nodeAddr = fmt.Sprintf("%s:%d", addr.IP.String(), a.Port)
				}
				addrs = append(addrs, nodeAddr)
			}
			if Debug {
				log.Printf("register: %#v", addrs)
			}
			d.registryLock.Lock()
			d.registry[pkt.NodeID] = addrs
			d.registryLock.Unlock()
		}
	}
	log.Println("discover/read: stopping due to too many errors:", err)
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

	if Debug {
		log.Printf("read external:\n%s", hex.Dump(buf[:n]))
	}

	var pkt AnnounceV2
	err = pkt.UnmarshalXDR(buf[:n])
	if err != nil {
		log.Println("discover/external/decode:", err)
		return nil
	}

	if Debug {
		log.Printf("read external: %#v", pkt)
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
