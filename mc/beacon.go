package mc

import (
	"log"
	"net"
)

type recv struct {
	data []byte
	src  net.Addr
}

type Beacon struct {
	group  string
	port   int
	conns  []*net.UDPConn
	inbox  chan []byte
	outbox chan recv
}

func NewBeacon(group string, port int) *Beacon {
	b := &Beacon{
		group:  group,
		port:   port,
		inbox:  make(chan []byte),
		outbox: make(chan recv),
	}
	go b.run()
	return b
}

func (b *Beacon) Send(data []byte) {
	b.inbox <- data
}

func (b *Beacon) Recv() ([]byte, net.Addr) {
	recv := <-b.outbox
	return recv.data, recv.src
}

func (b *Beacon) run() {
	group := &net.UDPAddr{IP: net.ParseIP(b.group), Port: b.port}

	intfs, err := net.Interfaces()
	if err != nil {
		log.Fatal(err)
	}
	if debug {
		dlog.Printf("trying %d interfaces", len(intfs))
	}

	for _, intf := range intfs {
		intf := intf

		if debug {
			dlog.Printf("trying interface %q", intf.Name)
		}
		conn, err := net.ListenMulticastUDP("udp4", &intf, group)
		if err != nil {
			if debug {
				dlog.Printf("failed to listen for multicast group on %q: %v", intf.Name, err)
			}
		} else {
			b.conns = append(b.conns, conn)
			if debug {
				dlog.Printf("listening for multicast group on %q", intf.Name)
			}
		}
	}

	for _, conn := range b.conns {
		conn := conn
		go func() {
			for {
				var bs = make([]byte, 1500)
				n, addr, err := conn.ReadFrom(bs)
				if err != nil {
					dlog.Println(err)
					return
				}
				if debug {
					dlog.Printf("recv %d bytes from %s on %v", n, addr, conn)
				}
				b.outbox <- recv{bs[:n], addr}
			}
		}()
	}

	go func() {
		for bs := range b.inbox {
			for _, conn := range b.conns {
				_, err := conn.WriteTo(bs, group)
				if err != nil {
					dlog.Println(err)
					return
				}
				if debug {
					dlog.Printf("sent %d bytes to %s on %v", len(bs), group, conn)
				}
			}
		}
	}()
}
