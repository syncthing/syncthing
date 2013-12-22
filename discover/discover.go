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

This is the XDR encoding of:

struct Announcement {
	unsigned int Magic;
	unsigned short Port;
	string NodeID<>;
}

(Hence NodeID is padded to a multiple of 32 bits)

The sending node's address is not encoded -- it is taken to be the source
address of the announcement. Every time such a packet is received, a local
table that maps NodeID to Address is updated. When the local node wants to
connect to another node with the address specification 'dynamic', this table is
consulted.

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
	"encoding/binary"
	"errors"
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

var (
	errBadMagic = errors.New("bad magic")
	errFormat   = errors.New("incorrect packet format")
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

type packet struct {
	magic uint32 // AnnouncementMagic or QueryMagic
	port  uint16 // unset if magic == QueryMagic
	id    string
}

// We tolerate a certain amount of errors because we might be running in
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

	buf := encodePacket(packet{AnnouncementMagic, uint16(d.ListenPort), d.MyID})
	go d.writeAnnouncements(buf, remote4, d.BroadcastIntv)
}

func (d *Discoverer) sendExtAnnouncements() {
	extIPs, err := net.LookupIP(d.extServer)
	if err != nil {
		log.Printf("discover/external: %v; no external announcements", err)
		return
	}

	buf := encodePacket(packet{AnnouncementMagic, uint16(d.ExtListenPort), d.MyID})
	for _, extIP := range extIPs {
		remote4 := &net.UDPAddr{IP: extIP, Port: AnnouncementPort}
		go d.writeAnnouncements(buf, remote4, d.ExtBroadcastIntv)
	}
}

func (d *Discoverer) writeAnnouncements(buf []byte, remote *net.UDPAddr, intv time.Duration) {
	var errCounter = 0
	var err error
	for errCounter < maxErrors {
		_, _, err = d.conn.WriteMsgUDP(buf, nil, remote)
		if err != nil {
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

		pkt, err := decodePacket(buf[:n])
		if err != nil || pkt.magic != AnnouncementMagic {
			errCounter++
			time.Sleep(time.Second)
			continue
		}

		errCounter = 0

		if pkt.id != d.MyID {
			nodeAddr := fmt.Sprintf("%s:%d", addr.IP.String(), pkt.port)
			d.registryLock.Lock()
			if d.registry[pkt.id] != nodeAddr {
				d.registry[pkt.id] = nodeAddr
			}
			d.registryLock.Unlock()
		}
	}
	log.Println("discover/read: stopping due to too many errors:", err)
}

func (d *Discoverer) Lookup(node string) (string, bool) {
	d.registryLock.Lock()
	defer d.registryLock.Unlock()
	addr, ok := d.registry[node]
	return addr, ok
}

func encodePacket(pkt packet) []byte {
	var idbs = []byte(pkt.id)
	var l = len(idbs) + pad(len(idbs)) + 4 + 4
	if pkt.magic == AnnouncementMagic {
		l += 4
	}

	var buf = make([]byte, l)
	var offset = 0

	binary.BigEndian.PutUint32(buf[offset:], pkt.magic)
	offset += 4

	if pkt.magic == AnnouncementMagic {
		binary.BigEndian.PutUint16(buf[offset:], uint16(pkt.port))
		offset += 4
	}

	binary.BigEndian.PutUint32(buf[offset:], uint32(len(idbs)))
	offset += 4
	copy(buf[offset:], idbs)

	return buf
}

func decodePacket(buf []byte) (*packet, error) {
	var p packet
	var offset int

	if len(buf) < 4 {
		// short packet
		return nil, errFormat
	}
	p.magic = binary.BigEndian.Uint32(buf[offset:])
	offset += 4

	if p.magic != AnnouncementMagic && p.magic != QueryMagic {
		return nil, errBadMagic
	}

	if p.magic == AnnouncementMagic {
		if len(buf) < offset+4 {
			// short packet
			return nil, errFormat
		}
		p.port = binary.BigEndian.Uint16(buf[offset:])
		offset += 2
		reserved := binary.BigEndian.Uint16(buf[offset:])
		if reserved != 0 {
			return nil, errFormat
		}
		offset += 2
	}

	if len(buf) < offset+4 {
		// short packet
		return nil, errFormat
	}
	l := binary.BigEndian.Uint32(buf[offset:])
	offset += 4

	if len(buf) < offset+int(l)+pad(int(l)) {
		// short packet
		return nil, errFormat
	}
	idbs := buf[offset : offset+int(l)]
	p.id = string(idbs)
	offset += int(l) + pad(int(l))
	if len(buf[offset:]) > 0 {
		// extra data
		return nil, errFormat
	}

	return &p, nil
}

func pad(l int) int {
	d := l % 4
	if d == 0 {
		return 0
	}
	return 4 - d
}
