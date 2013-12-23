package main

import (
	"log"
	"net"
	"sync"

	"github.com/calmh/syncthing/discover"
)

type Node struct {
	IP   []byte
	Port uint16
}

var (
	nodes = make(map[string]Node)
	lock  sync.Mutex
)

func main() {
	addr, _ := net.ResolveUDPAddr("udp", ":22025")
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		panic(err)
	}

	var buf = make([]byte, 1024)
	for {
		n, addr, err := conn.ReadFromUDP(buf)
		if err != nil {
			panic(err)
		}
		pkt, err := discover.DecodePacket(buf[:n])
		if err != nil {
			log.Println("Warning:", err)
			continue
		}

		switch pkt.Magic {
		case 0x20121025:
			// Announcement
			//lock.Lock()
			ip := addr.IP.To4()
			if ip == nil {
				ip = addr.IP.To16()
			}
			node := Node{ip, uint16(pkt.Port)}
			log.Println("<-", pkt.ID, node)
			nodes[pkt.ID] = node
			//lock.Unlock()
		case 0x19760309:
			// Query
			//lock.Lock()
			node, ok := nodes[pkt.ID]
			//lock.Unlock()
			if ok {
				pkt := discover.Packet{
					Magic: 0x20121025,
					ID:    pkt.ID,
					Port:  node.Port,
					IP:    node.IP,
				}
				_, _, err = conn.WriteMsgUDP(discover.EncodePacket(pkt), nil, addr)
				if err != nil {
					log.Println("Warning:", err)
				} else {
					log.Println("->", pkt.ID, node)
				}
			}
		}

	}
}
