package main

import (
	"log"
	"net"
	"sync"
	"time"

	"github.com/calmh/syncthing/discover"
)

type Node struct {
	IP      []byte
	Port    uint16
	Updated time.Time
}

var (
	nodes   = make(map[string]Node)
	lock    sync.Mutex
	queries = 0
)

func main() {
	addr, _ := net.ResolveUDPAddr("udp", ":22025")
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		panic(err)
	}

	go func() {
		for {
			time.Sleep(600 * time.Second)

			lock.Lock()

			var deleted = 0
			for id, node := range nodes {
				if time.Since(node.Updated) > 60*time.Minute {
					delete(nodes, id)
					deleted++
				}
			}
			log.Printf("Expired %d nodes; %d nodes in registry; %d queries", deleted, len(nodes), queries)
			queries = 0

			lock.Unlock()
		}
	}()

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
			lock.Lock()
			ip := addr.IP.To4()
			if ip == nil {
				ip = addr.IP.To16()
			}
			node := Node{
				IP:      ip,
				Port:    uint16(pkt.Port),
				Updated: time.Now(),
			}
			//log.Println("<-", pkt.ID, node)
			nodes[pkt.ID] = node
			lock.Unlock()
		case 0x19760309:
			// Query
			lock.Lock()
			node, ok := nodes[pkt.ID]
			queries++
			lock.Unlock()
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
				}
			}
		}
	}
}
