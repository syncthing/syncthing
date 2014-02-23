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

	localBroadcastTick  <-chan time.Time
	forcedBroadcastTick chan time.Time
}

var (
	ErrIncorrectMagic = errors.New("Incorrect magic number")
)

// We tolerate a certain amount of errors because we might be running on
// laptops that sleep and wake, have intermittent network connectivity, etc.
// When we hit this many errors in succession, we stop.
const maxErrors = 30

func NewDiscoverer(id string, port int, extServer string) (*Discoverer, error) {
	local := &net.UDPAddr{IP: nil, Port: AnnouncementPort}
	conn, err := net.ListenUDP("udp", local)
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
		disc.localBroadcastTick = time.Tick(disc.BroadcastIntv)
		disc.forcedBroadcastTick = make(chan time.Time)
		go disc.sendAnnouncements()
	}
	if len(disc.extServer) > 0 {
		go disc.sendExtAnnouncements()
	}

	return disc, nil
}

func (d *Discoverer) sendAnnouncements() {
	var pkt = AnnounceV2{AnnouncementMagicV2, d.MyID, []Address{{nil, 22000}}}
	var buf = pkt.MarshalXDR()
	var errCounter = 0
	var err error

	for errCounter < maxErrors {
		for _, ipStr := range allBroadcasts() {
			var addrStr = ipStr + ":21025"

			remote, err := net.ResolveUDPAddr("udp4", addrStr)
			if err != nil {
				log.Printf("discover/external: %v; no external announcements", err)
				return
			}

			if Debug {
				log.Println("send announcement -> ", remote)
			}
			_, _, err = d.conn.WriteMsgUDP(buf, nil, remote)
			if err != nil {
				log.Println("discover/write: warning:", err)
				errCounter++
			} else {
				errCounter = 0
			}
		}

		select {
		case <-d.localBroadcastTick:
		case <-d.forcedBroadcastTick:
		}
	}
	log.Println("discover/write: local: stopping due to too many errors:", err)
}

func (d *Discoverer) sendExtAnnouncements() {
	remote, err := net.ResolveUDPAddr("udp", d.extServer)
	if err != nil {
		log.Printf("discover/external: %v; no external announcements", err)
		return
	}

	var pkt = AnnounceV2{AnnouncementMagicV2, d.MyID, []Address{{nil, 22000}}}
	var buf = pkt.MarshalXDR()
	var errCounter = 0

	for errCounter < maxErrors {
		if Debug {
			log.Println("send announcement -> ", remote)
		}
		_, _, err = d.conn.WriteMsgUDP(buf, nil, remote)
		if err != nil {
			log.Println("discover/write: warning:", err)
			errCounter++
		} else {
			errCounter = 0
		}
		time.Sleep(d.ExtBroadcastIntv)
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

func allBroadcasts() []string {
	var bcasts = make(map[string]bool)
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}

	for _, addr := range addrs {
		switch {
		case strings.HasPrefix(addr.String(), "127."):
			// Ignore v4 localhost

		case strings.Contains(addr.String(), ":"):
			// Ignore all v6, because we need link local multicast there which I haven't implemented

		default:
			if in, ok := addr.(*net.IPNet); ok {
				il := len(in.IP) - 1
				ml := len(in.Mask) - 1
				for i := range in.Mask {
					in.IP[il-i] = in.IP[il-i] | ^in.Mask[ml-i]
				}
				parts := strings.Split(in.String(), "/")
				bcasts[parts[0]] = true
			}
		}
	}

	var l []string
	for ip := range bcasts {
		l = append(l, ip)
	}
	return l
}
