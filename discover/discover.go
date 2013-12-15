/*
This is the local node discovery protocol. In principle we might be better
served by something more standardized, such as mDNS / DNS-SD. In practice, this
was much easier and quicker to get up and running.

The mode of operation is to periodically (currently once every 30 seconds)
transmit a broadcast UDP packet to the well known port number 21025. The packet
has the following format:

     0                   1                   2                   3
     0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    |                         Magic Number                          |
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    |          Port Number          |        Length of NodeID       |
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    /                                                               /
    \                   NodeID (variable length)                    \
    /                                                               /
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+

The sending node's address is not encoded -- it is taken to be the source
address of the announcement. Every time such a packet is received, a local
table that maps NodeID to Address is updated. When the local node wants to
connect to another node with the address specification 'dynamic', this table is
consulted.
*/
package discover

import (
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"time"
)

type Discoverer struct {
	MyID          string
	ListenPort    int
	BroadcastIntv time.Duration

	conn         *net.UDPConn
	registry     map[string]string
	registryLock sync.RWMutex
}

func NewDiscoverer(id string, port int) (*Discoverer, error) {
	local4 := &net.UDPAddr{IP: net.IP{0, 0, 0, 0}, Port: 21025}
	conn, err := net.ListenUDP("udp4", local4)
	if err != nil {
		return nil, err
	}

	disc := &Discoverer{
		MyID:          id,
		ListenPort:    port,
		BroadcastIntv: 30 * time.Second,
		conn:          conn,
		registry:      make(map[string]string),
	}

	go disc.sendAnnouncements()
	go disc.recvAnnouncements()

	return disc, nil
}

func (d *Discoverer) sendAnnouncements() {
	remote4 := &net.UDPAddr{IP: net.IP{255, 255, 255, 255}, Port: 21025}

	idbs := []byte(d.MyID)
	buf := make([]byte, 4+4+4+len(idbs))

	binary.BigEndian.PutUint32(buf, uint32(0x121025))
	binary.BigEndian.PutUint16(buf[4:], uint16(d.ListenPort))
	binary.BigEndian.PutUint16(buf[6:], uint16(len(idbs)))
	copy(buf[8:], idbs)

	for {
		_, _, err := d.conn.WriteMsgUDP(buf, nil, remote4)
		if err != nil {
			panic(err)
		}
		time.Sleep(d.BroadcastIntv)
	}
}

func (d *Discoverer) recvAnnouncements() {
	var buf = make([]byte, 1024)
	for {
		_, addr, err := d.conn.ReadFromUDP(buf)
		if err != nil {
			panic(err)
		}
		magic := binary.BigEndian.Uint32(buf)
		if magic != 0x121025 {
			continue
		}
		port := binary.BigEndian.Uint16(buf[4:])
		l := binary.BigEndian.Uint16(buf[6:])
		idbs := buf[8 : l+8]
		id := string(idbs)

		if id != d.MyID {
			nodeAddr := fmt.Sprintf("%s:%d", addr.IP.String(), port)
			d.registryLock.Lock()
			if d.registry[id] != nodeAddr {
				d.registry[id] = nodeAddr
			}
			d.registryLock.Unlock()
		}
	}
}

func (d *Discoverer) Lookup(node string) (string, bool) {
	d.registryLock.Lock()
	defer d.registryLock.Unlock()
	addr, ok := d.registry[node]
	return addr, ok
}
