package discover

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"github.com/calmh/syncthing/buffers"
	"github.com/calmh/syncthing/mc"
)

const (
	AnnouncementPort = 21025
)

type Discoverer struct {
	myID             string
	listenAddrs      []string
	localBcastIntv   time.Duration
	globalBcastIntv  time.Duration
	beacon           *mc.Beacon
	registry         map[string][]string
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

func NewDiscoverer(id string, addresses []string) (*Discoverer, error) {
	disc := &Discoverer{
		myID:            id,
		listenAddrs:     addresses,
		localBcastIntv:  30 * time.Second,
		globalBcastIntv: 1800 * time.Second,
		beacon:          mc.NewBeacon("239.21.0.25", 21025),
		registry:        make(map[string][]string),
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

func (d *Discoverer) Hint(node string, addrs []string) {
	resAddrs := resolveAddrs(addrs)
	d.registerNode(nil, Node{
		ID:        node,
		Addresses: resAddrs,
	})
}

func (d *Discoverer) All() map[string][]string {
	d.registryLock.RLock()
	nodes := make(map[string][]string, len(d.registry))
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
		Magic: AnnouncementMagicV2,
		This:  Node{d.myID, addrs},
	}
	return pkt.MarshalXDR()
}

func (d *Discoverer) sendLocalAnnouncements() {
	var addrs = resolveAddrs(d.listenAddrs)

	var pkt = AnnounceV2{
		Magic: AnnouncementMagicV2,
		This:  Node{d.myID, addrs},
	}

	for {
		pkt.Extra = nil
		d.registryLock.RLock()
		for node, addrs := range d.registry {
			if len(pkt.Extra) == 16 {
				break
			}

			anode := Node{node, resolveAddrs(addrs)}
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

	var buf []byte
	if d.extPort != 0 {
		var pkt = AnnounceV2{
			Magic: AnnouncementMagicV2,
			This:  Node{d.myID, []Address{{Port: d.extPort}}},
		}
		buf = pkt.MarshalXDR()
	} else {
		buf = d.announcementPkt()
	}
	var errCounter = 0

	for errCounter < maxErrors {
		var ok bool

		if debug {
			dlog.Printf("send announcement -> %v\n%s", remote, hex.Dump(buf))
		}

		_, err = conn.WriteTo(buf, remote)
		if err != nil {
			log.Println("discover/write: warning:", err)
			errCounter++
			ok = false
		} else {
			errCounter = 0

			// Verify that the announce server responds positively for our node ID

			time.Sleep(1 * time.Second)
			res := d.externalLookup(d.myID)
			if debug {
				dlog.Println("external lookup check:", res)
			}
			ok = len(res) > 0

		}

		d.extAnnounceOKmut.Lock()
		d.extAnnounceOK = ok
		d.extAnnounceOKmut.Unlock()

		if ok {
			time.Sleep(d.globalBcastIntv)
		} else {
			time.Sleep(60 * time.Second)
		}
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
		if err != nil && err != io.EOF {
			continue
		}

		if debug {
			dlog.Printf("parsed announcement: %#v", pkt)
		}

		var newNode bool
		if pkt.This.ID != d.myID {
			n := d.registerNode(addr, pkt.This)
			newNode = newNode || n
		}
		for _, node := range pkt.Extra {
			if node.ID != d.myID {
				n := d.registerNode(nil, node)
				newNode = newNode || n
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
			dlog.Println("no valid address for", node.ID)
		}
	}
	if debug {
		dlog.Printf("register: %s -> %#v", node.ID, addrs)
	}
	d.registryLock.Lock()
	_, seen := d.registry[node.ID]
	d.registry[node.ID] = addrs
	d.registryLock.Unlock()
	return !seen
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

	buf = buffers.Get(2048)
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
	if err != nil && err != io.EOF {
		log.Println("discover/external/decode:", err)
		return nil
	}

	if debug {
		dlog.Printf("parsed external: %#v", pkt)
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
		}
	}
	return raddrs
}
