package pfilter

import (
	"net"
	"sort"
	"sync"
	"sync/atomic"
)

// Filter object receives all data sent out on the Outgoing callback,
// and is expected to decide if it wants to receive the packet or not via
// the Receive callback
type Filter interface {
	Outgoing([]byte, net.Addr)
	ClaimIncoming([]byte, net.Addr) bool
}

// NewPacketFilter creates a packet filter object wrapping the given packet
// connection.
func NewPacketFilter(conn net.PacketConn) *PacketFilter {
	d := &PacketFilter{
		PacketConn: conn,
	}
	return d
}

// PacketFilter embeds a net.PacketConn to perform the filtering.
type PacketFilter struct {
	net.PacketConn

	conns []*FilteredConn
	mut   sync.Mutex

	dropped  uint64
	overflow uint64
}

// NewConn returns a new net.PacketConn object which filters packets based
// on the provided filter. If filter is nil, the connection will receive all
// packets. Priority decides which connection gets the ability to claim the packet.
func (d *PacketFilter) NewConn(priority int, filter Filter) net.PacketConn {
	conn := &FilteredConn{
		priority:   priority,
		source:     d,
		recvBuffer: make(chan packet, 256),
		filter:     filter,
		closed:     make(chan struct{}),
	}
	d.mut.Lock()
	d.conns = append(d.conns, conn)
	sort.Sort(filteredConnList(d.conns))
	d.mut.Unlock()
	return conn
}

func (d *PacketFilter) removeConn(r *FilteredConn) {
	d.mut.Lock()
	for i, conn := range d.conns {
		if conn == r {
			copy(d.conns[i:], d.conns[i+1:])
			d.conns[len(d.conns)-1] = nil
			d.conns = d.conns[:len(d.conns)-1]
			break
		}
	}
	d.mut.Unlock()
}

// NumberOfConns returns the number of currently active virtual connections
func (d *PacketFilter) NumberOfConns() int {
	d.mut.Lock()
	n := len(d.conns)
	d.mut.Unlock()
	return n
}

// Dropped returns number of packets dropped due to nobody claiming them.
func (d *PacketFilter) Dropped() uint64 {
	return atomic.LoadUint64(&d.dropped)
}

// Overflow returns number of packets were dropped due to receive buffers being
// full.
func (d *PacketFilter) Overflow() uint64 {
	return atomic.LoadUint64(&d.overflow)
}

// Start starts the packet filter.
func (d *PacketFilter) Start() {
	go d.loop()
}

func (d *PacketFilter) loop() {
	var buf []byte
next:
	for {
		buf = bufPool.Get().([]byte)
		n, addr, err := d.ReadFrom(buf)
		pkt := packet{
			n:    n,
			addr: addr,
			err:  err,
			buf:  buf[:n],
		}

		d.mut.Lock()
		conns := d.conns
		d.mut.Unlock()

		if err != nil {
			for _, conn := range conns {
				select {
				case conn.recvBuffer <- pkt:
				default:
					atomic.AddUint64(&d.overflow, 1)
				}
			}
			return
		}

		for _, conn := range conns {
			if conn.filter == nil || conn.filter.ClaimIncoming(pkt.buf, pkt.addr) {
				select {
				case conn.recvBuffer <- pkt:
				default:
					atomic.AddUint64(&d.overflow, 1)
				}
				goto next
			}
		}

		atomic.AddUint64(&d.dropped, 1)
	}
}
