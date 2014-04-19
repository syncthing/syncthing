package main

import (
	"encoding/binary"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"time"

	"github.com/calmh/syncthing/discover"
	"github.com/golang/groupcache/lru"
	"github.com/juju/ratelimit"
)

type Node struct {
	Addresses []Address
	Updated   time.Time
}

type Address struct {
	IP   []byte
	Port uint16
}

var (
	nodes     = make(map[string]Node)
	lock      sync.Mutex
	queries   = 0
	announces = 0
	answered  = 0
	limited   = 0
	unknowns  = 0
	debug     = false
	limiter   = lru.New(1024)
)

func main() {
	var listen string
	var timestamp bool
	var statsIntv int
	var statsFile string

	flag.StringVar(&listen, "listen", ":22025", "Listen address")
	flag.BoolVar(&debug, "debug", false, "Enable debug output")
	flag.BoolVar(&timestamp, "timestamp", true, "Timestamp the log output")
	flag.IntVar(&statsIntv, "stats-intv", 0, "Statistics output interval (s)")
	flag.StringVar(&statsFile, "stats-file", "/var/log/discosrv.stats", "Statistics file name")
	flag.Parse()

	log.SetOutput(os.Stdout)
	if !timestamp {
		log.SetFlags(0)
	}

	addr, _ := net.ResolveUDPAddr("udp", listen)
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		log.Fatal(err)
	}

	if statsIntv > 0 {
		go logStats(statsFile, statsIntv)
	}

	var buf = make([]byte, 1024)
	for {
		buf = buf[:cap(buf)]
		n, addr, err := conn.ReadFromUDP(buf)

		if limit(addr) {
			// Rate limit in effect for source
			continue
		}

		if err != nil {
			log.Fatal(err)
		}

		if n < 4 {
			log.Printf("Received short packet (%d bytes)", n)
			continue
		}

		buf = buf[:n]
		magic := binary.BigEndian.Uint32(buf)

		switch magic {
		case discover.AnnouncementMagicV2:
			handleAnnounceV2(addr, buf)

		case discover.QueryMagicV2:
			handleQueryV2(conn, addr, buf)

		default:
			lock.Lock()
			unknowns++
			lock.Unlock()
		}
	}
}

func limit(addr *net.UDPAddr) bool {
	key := addr.IP.String()

	lock.Lock()
	defer lock.Unlock()

	bkt, ok := limiter.Get(key)
	if ok {
		bkt := bkt.(*ratelimit.Bucket)
		if bkt.TakeAvailable(1) != 1 {
			// Rate limit exceeded; ignore packet
			if debug {
				log.Println("Rate limit exceeded for", key)
			}
			limited++
			return true
		}
	} else {
		if debug {
			log.Println("New limiter for", key)
		}
		// One packet per ten seconds average rate, burst ten packets
		limiter.Add(key, ratelimit.NewBucket(10*time.Second, 10))
	}

	return false
}

func handleAnnounceV2(addr *net.UDPAddr, buf []byte) {
	var pkt discover.AnnounceV2
	err := pkt.UnmarshalXDR(buf)
	if err != nil {
		log.Println("AnnounceV2 Unmarshal:", err)
		log.Println(hex.Dump(buf))
		return
	}
	if debug {
		log.Printf("<- %v %#v", addr, pkt)
	}

	lock.Lock()
	announces++
	lock.Unlock()

	ip := addr.IP.To4()
	if ip == nil {
		ip = addr.IP.To16()
	}

	var addrs []Address
	for _, addr := range pkt.Addresses {
		tip := addr.IP
		if len(tip) == 0 {
			tip = ip
		}
		addrs = append(addrs, Address{
			IP:   tip,
			Port: addr.Port,
		})
	}

	node := Node{
		Addresses: addrs,
		Updated:   time.Now(),
	}

	lock.Lock()
	nodes[pkt.NodeID] = node
	lock.Unlock()
}

func handleQueryV2(conn *net.UDPConn, addr *net.UDPAddr, buf []byte) {
	var pkt discover.QueryV2
	err := pkt.UnmarshalXDR(buf)
	if err != nil {
		log.Println("QueryV2 Unmarshal:", err)
		log.Println(hex.Dump(buf))
		return
	}
	if debug {
		log.Printf("<- %v %#v", addr, pkt)
	}

	lock.Lock()
	node, ok := nodes[pkt.NodeID]
	queries++
	lock.Unlock()

	if ok && len(node.Addresses) > 0 {
		pkt := discover.AnnounceV2{
			Magic:  discover.AnnouncementMagicV2,
			NodeID: pkt.NodeID,
		}
		for _, addr := range node.Addresses {
			pkt.Addresses = append(pkt.Addresses, discover.Address{IP: addr.IP, Port: addr.Port})
		}
		if debug {
			log.Printf("-> %v %#v", addr, pkt)
		}

		tb := pkt.MarshalXDR()
		_, _, err = conn.WriteMsgUDP(tb, nil, addr)
		if err != nil {
			log.Println("QueryV2 response write:", err)
		}

		lock.Lock()
		answered++
		lock.Unlock()
	}
}

func next(intv int) time.Time {
	d := time.Duration(intv) * time.Second
	t0 := time.Now()
	t1 := t0.Add(d).Truncate(d)
	time.Sleep(t1.Sub(t0))
	return t1
}

func logStats(file string, intv int) {
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		log.Fatal(err)
	}
	for {
		t := next(intv)

		lock.Lock()

		var deleted = 0
		for id, node := range nodes {
			if time.Since(node.Updated) > 60*time.Minute {
				delete(nodes, id)
				deleted++
			}
		}

		fmt.Fprintf(f, "%d Nr:%d Ne:%d Qt:%d Qa:%d A:%d U:%d Lq:%d Lc:%d\n",
			t.Unix(), len(nodes), deleted, queries, answered, announces, unknowns, limited, limiter.Len())
		f.Sync()

		queries = 0
		announces = 0
		answered = 0
		limited = 0
		unknowns = 0

		lock.Unlock()
	}
}
