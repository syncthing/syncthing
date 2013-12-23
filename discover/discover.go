/*
This is the local node discovery protocol. In principle we might be better
served by something more standardized, such as mDNS / DNS-SD. In practice, this
was much easier and quicker to get up and running.

The mode of operation is to periodically (currently once every 30 seconds)
broadcast an Announcement packet to UDP port 21025. The packet has the
following format:

     0                   1                   2                   3
     0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    |                   Magic Number (0x20121025)                   |
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    |          Port Number          |           Reserved            |
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    |                        Length of NodeID                       |
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    /                                                               /
    \                   NodeID (variable length)                    \
    /                                                               /
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    |                          Length of IP                         |
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    /                                                               /
    \                     IP (variable length)                      \
    /                                                               /
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+

This is the XDR encoding of:

struct Announcement {
	unsigned int Magic;
	unsigned short Port;
	string NodeID<>;
}

(Hence NodeID is padded to a multiple of 32 bits)

The sending node's address is not encoded in local announcement -- the Length
of IP field is set to zero and the address is taken to be the source address of
the announcement. In announcement packets sent by a discovery server in
response to a query, the IP is present and the length is either 4 (IPv4) or 16
(IPv6).

Every time such a packet is received, a local table that maps NodeID to Address
is updated. When the local node wants to connect to another node with the
address specification 'dynamic', this table is consulted.

For external discovery, an identical packet is sent every 30 minutes to the
external discovery server. The server keeps information for up to 60 minutes.
To query the server, and UDP packet with the format below is sent.

     0                   1                   2                   3
     0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    |                   Magic Number (0x19760309)                   |
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    |                        Length of NodeID                       |
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    /                                                               /
    \                   NodeID (variable length)                    \
    /                                                               /
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+

This is the XDR encoding of:

struct Announcement {
	unsigned int Magic;
	string NodeID<>;
}

(Hence NodeID is padded to a multiple of 32 bits)

It is answered with an announcement packet for the queried node ID if the
information is available. There is no answer for queries about unknown nodes. A
reasonable timeout is recommended instead. (This, combined with server side
rate limits for packets per source IP and queries per node ID, prevents the
server from being used as an amplifier in a DDoS attack.)
*/
package discover

import (
	"fmt"
	"log"
	"net"
	"sync"
	"time"
)

const (
	AnnouncementPort  = 21025
	AnnouncementMagic = 0x20121025
	QueryMagic        = 0x19760309
)

type Discoverer struct {
	MyID             string
	ListenPort       int
	BroadcastIntv    time.Duration
	ExtListenPort    int
	ExtBroadcastIntv time.Duration

	conn         *net.UDPConn
	registry     map[string]string
	registryLock sync.RWMutex
	extServer    string
}

// We tolerate a certain amount of errors because we might be running on
// laptops that sleep and wake, have intermittent network connectivity, etc.
// When we hit this many errors in succession, we stop.
const maxErrors = 30

func NewDiscoverer(id string, port int, extPort int, extServer string) (*Discoverer, error) {
	local4 := &net.UDPAddr{IP: net.IP{0, 0, 0, 0}, Port: AnnouncementPort}
	conn, err := net.ListenUDP("udp4", local4)
	if err != nil {
		return nil, err
	}

	disc := &Discoverer{
		MyID:             id,
		ListenPort:       port,
		BroadcastIntv:    30 * time.Second,
		ExtListenPort:    extPort,
		ExtBroadcastIntv: 1800 * time.Second,

		conn:      conn,
		registry:  make(map[string]string),
		extServer: extServer,
	}

	go disc.recvAnnouncements()

	if disc.ListenPort > 0 {
		disc.sendAnnouncements()
	}
	if len(disc.extServer) > 0 && disc.ExtListenPort > 0 {
		disc.sendExtAnnouncements()
	}

	return disc, nil
}

func (d *Discoverer) sendAnnouncements() {
	remote4 := &net.UDPAddr{IP: net.IP{255, 255, 255, 255}, Port: AnnouncementPort}

	buf := EncodePacket(Packet{AnnouncementMagic, uint16(d.ListenPort), d.MyID, nil})
	go d.writeAnnouncements(buf, remote4, d.BroadcastIntv)
}

func (d *Discoverer) sendExtAnnouncements() {
	extIP, err := net.ResolveUDPAddr("udp", d.extServer+":22025")
	if err != nil {
		log.Printf("discover/external: %v; no external announcements", err)
		return
	}

	buf := EncodePacket(Packet{AnnouncementMagic, uint16(d.ExtListenPort), d.MyID, nil})
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

		pkt, err := DecodePacket(buf[:n])
		if err != nil || pkt.Magic != AnnouncementMagic {
			errCounter++
			time.Sleep(time.Second)
			continue
		}

		errCounter = 0

		if pkt.ID != d.MyID {
			nodeAddr := fmt.Sprintf("%s:%d", addr.IP.String(), pkt.Port)
			d.registryLock.Lock()
			if d.registry[pkt.ID] != nodeAddr {
				d.registry[pkt.ID] = nodeAddr
			}
			d.registryLock.Unlock()
		}
	}
	log.Println("discover/read: stopping due to too many errors:", err)
}

func (d *Discoverer) externalLookup(node string) (string, bool) {
	extIP, err := net.ResolveUDPAddr("udp", d.extServer+":22025")
	if err != nil {
		log.Printf("discover/external: %v; no external lookup", err)
		return "", false
	}

	var res = make(chan string, 1)
	conn, err := net.DialUDP("udp", nil, extIP)
	if err != nil {
		log.Printf("discover/external: %v; no external lookup", err)
		return "", false
	}

	_, err = conn.Write(EncodePacket(Packet{QueryMagic, 0, node, nil}))
	if err != nil {
		log.Printf("discover/external: %v; no external lookup", err)
		return "", false
	}

	go func() {
		var buf = make([]byte, 1024)
		n, err := conn.Read(buf)
		if err != nil {
			log.Printf("discover/external/read: %v; no external lookup", err)
			return
		}

		pkt, err := DecodePacket(buf[:n])
		if err != nil {
			log.Printf("discover/external/read: %v; no external lookup", err)
			return
		}

		if pkt.Magic != AnnouncementMagic {
			log.Printf("discover/external/read: bad magic; no external lookup", err)
			return
		}

		res <- fmt.Sprintf("%s:%d", ipStr(pkt.IP), pkt.Port)
	}()

	select {
	case r := <-res:
		return r, true
	case <-time.After(5 * time.Second):
		return "", false
	}
}

func (d *Discoverer) Lookup(node string) (string, bool) {
	d.registryLock.Lock()
	addr, ok := d.registry[node]
	d.registryLock.Unlock()

	if ok {
		return addr, true
	} else if len(d.extServer) != 0 {
		// We might want to cache this, but not permanently so it needs some intelligence
		return d.externalLookup(node)
	}
	return "", false
}
