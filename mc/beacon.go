package mc

import "net"

type recv struct {
	data []byte
	src  net.Addr
}

type dst struct {
	intf string
	conn *net.UDPConn
}

type Beacon struct {
	conn   *net.UDPConn
	port   int
	conns  []dst
	inbox  chan []byte
	outbox chan recv
}

func NewBeacon(port int) (*Beacon, error) {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{Port: port})
	if err != nil {
		return nil, err
	}
	b := &Beacon{
		conn:   conn,
		port:   port,
		inbox:  make(chan []byte),
		outbox: make(chan recv, 16),
	}

	go b.reader()
	go b.writer()

	return b, nil
}

func (b *Beacon) Send(data []byte) {
	b.inbox <- data
}

func (b *Beacon) Recv() ([]byte, net.Addr) {
	recv := <-b.outbox
	return recv.data, recv.src
}

func (b *Beacon) reader() {
	var bs = make([]byte, 65536)
	for {
		n, addr, err := b.conn.ReadFrom(bs)
		if err != nil {
			dlog.Println(err)
			return
		}
		if debug {
			dlog.Printf("recv %d bytes from %s", n, addr)
		}
		select {
		case b.outbox <- recv{bs[:n], addr}:
		default:
			if debug {
				dlog.Println("Dropping message")
			}
		}
	}
}

func (b *Beacon) writer() {
	for bs := range b.inbox {

		addrs, err := net.InterfaceAddrs()
		if err != nil {
			dlog.Println(err)
			continue
		}

		var dsts []net.IP
		for _, addr := range addrs {
			if iaddr, ok := addr.(*net.IPNet); ok && iaddr.IP.IsGlobalUnicast() {
				baddr := bcast(iaddr)
				dsts = append(dsts, baddr.IP)
			}
		}

		if len(dsts) == 0 {
			// Fall back to the general IPv4 broadcast address
			dsts = append(dsts, net.IP{0xff, 0xff, 0xff, 0xff})
		}

		for _, ip := range dsts {
			dst := &net.UDPAddr{IP: ip, Port: b.port}

			_, err := b.conn.WriteTo(bs, dst)
			if err != nil {
				dlog.Println(err)
				return
			}
			if debug {
				dlog.Printf("sent %d bytes to %s", len(bs), dst)
			}
		}
	}
}

func bcast(ip *net.IPNet) *net.IPNet {
	var bc = &net.IPNet{}
	bc.IP = make([]byte, len(ip.IP))
	copy(bc.IP, ip.IP)
	bc.Mask = ip.Mask

	offset := len(bc.IP) - len(bc.Mask)
	for i := range bc.IP {
		if i-offset > 0 {
			bc.IP[i] = ip.IP[i] | ^ip.Mask[i-offset]
		}
	}
	return bc
}
