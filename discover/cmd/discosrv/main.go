// Copyright (C) 2014 Jakob Borg and other contributors. All rights reserved.
// Use of this source code is governed by an MIT-style license that can be
// found in the LICENSE file.

package main

import (
	"encoding/binary"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"sync"
	"time"

	"github.com/calmh/syncthing/discover"
	"github.com/golang/groupcache/lru"
	"github.com/juju/ratelimit"
)

type node struct {
	addresses []address
	updated   time.Time
}

type address struct {
	ip   []byte
	port uint16
}

var (
	nodes     = make(map[string]node)
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
	if err != nil && err != io.EOF {
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

	var addrs []address
	for _, addr := range pkt.This.Addresses {
		tip := addr.IP
		if len(tip) == 0 {
			tip = ip
		}
		addrs = append(addrs, address{
			ip:   tip,
			port: addr.Port,
		})
	}

	node := node{
		addresses: addrs,
		updated:   time.Now(),
	}

	lock.Lock()
	nodes[pkt.This.ID] = node
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

	if ok && len(node.addresses) > 0 {
		ann := discover.AnnounceV2{
			Magic: discover.AnnouncementMagicV2,
			This: discover.Node{
				ID: pkt.NodeID,
			},
		}
		for _, addr := range node.addresses {
			ann.This.Addresses = append(ann.This.Addresses, discover.Address{IP: addr.ip, Port: addr.port})
		}
		if debug {
			log.Printf("-> %v %#v", addr, pkt)
		}

		tb := ann.MarshalXDR()
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
			if time.Since(node.updated) > 60*time.Minute {
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
