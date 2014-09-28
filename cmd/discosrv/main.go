// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/golang/groupcache/lru"
	"github.com/juju/ratelimit"
	"github.com/syncthing/syncthing/internal/discover"
	"github.com/syncthing/syncthing/internal/protocol"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

const cacheLimitSeconds = 3600

var (
	lock       sync.Mutex
	queries    = 0
	announces  = 0
	answered   = 0
	limited    = 0
	unknowns   = 0
	debug      = false
	lruSize    = 1024
	limitAvg   = 1
	limitBurst = 10
	limiter    *lru.Cache
)

func main() {
	var listen string
	var timestamp bool
	var statsIntv int
	var statsFile string
	var unknownFile string
	var dbDir string

	flag.StringVar(&listen, "listen", ":22026", "Listen address")
	flag.BoolVar(&debug, "debug", false, "Enable debug output")
	flag.BoolVar(&timestamp, "timestamp", true, "Timestamp the log output")
	flag.IntVar(&statsIntv, "stats-intv", 0, "Statistics output interval (s)")
	flag.StringVar(&statsFile, "stats-file", "/var/discosrv/stats", "Statistics file name")
	flag.StringVar(&unknownFile, "unknown-file", "", "Unknown packet log file name")
	flag.IntVar(&lruSize, "limit-cache", lruSize, "Limiter cache entries")
	flag.IntVar(&limitAvg, "limit-avg", limitAvg, "Allowed average package rate, per 10 s")
	flag.IntVar(&limitBurst, "limit-burst", limitBurst, "Allowed burst size, packets")
	flag.StringVar(&dbDir, "db-dir", "/var/discosrv/db", "Database directory")
	flag.Parse()

	limiter = lru.New(lruSize)

	log.SetOutput(os.Stdout)
	if !timestamp {
		log.SetFlags(0)
	}

	addr, _ := net.ResolveUDPAddr("udp", listen)
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		log.Fatal(err)
	}

	parentDir := filepath.Dir(dbDir)
	if _, err := os.Stat(parentDir); err != nil && os.IsNotExist(err) {
		err = os.MkdirAll(parentDir, 0755)
		if err != nil {
			log.Fatal(err)
		}
	}

	db, err := leveldb.OpenFile(dbDir, &opt.Options{CachedOpenFiles: 32})
	if err != nil {
		log.Fatal(err)
	}

	statsLog, err := os.OpenFile(statsFile, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		log.Fatal(err)
	}

	var unknownLog io.Writer
	if unknownFile != "" {
		unknownLog, err = os.OpenFile(unknownFile, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
		if err != nil {
			log.Fatal(err)
		}
	}

	if statsIntv > 0 {
		go logStats(statsLog, statsIntv)
	}

	go clean(statsLog, db)

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
		case discover.AnnouncementMagic:
			err := handleAnnounceV2(db, addr, buf)
			if err != nil && unknownLog != nil {
				fmt.Fprintf(unknownLog, "AE %d %v %x\n", time.Now().Unix(), addr, buf)
			}

		case discover.QueryMagic:
			err := handleQueryV2(db, conn, addr, buf)
			if err != nil && unknownLog != nil {
				fmt.Fprintf(unknownLog, "QE %d %v %x\n", time.Now().Unix(), addr, buf)
			}

		default:
			lock.Lock()
			unknowns++
			lock.Unlock()
			if unknownLog != nil {
				fmt.Fprintf(unknownLog, "UN %d %v %x\n", time.Now().Unix(), addr, buf)
			}
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
		limiter.Add(key, ratelimit.NewBucket(10*time.Second/time.Duration(limitAvg), int64(limitBurst)))
	}

	return false
}

func handleAnnounceV2(db *leveldb.DB, addr *net.UDPAddr, buf []byte) error {
	var pkt discover.Announce
	err := pkt.UnmarshalXDR(buf)
	if err != nil && err != io.EOF {
		return err
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
	now := time.Now().Unix()
	for _, addr := range pkt.This.Addresses {
		tip := addr.IP
		if len(tip) == 0 {
			tip = ip
		}
		addrs = append(addrs, address{
			ip:   tip,
			port: addr.Port,
			seen: now,
		})
	}

	var id protocol.DeviceID
	if len(pkt.This.ID) == 32 {
		// Raw node ID
		copy(id[:], pkt.This.ID)
	} else {
		err = id.UnmarshalText(pkt.This.ID)
		if err != nil {
			return err
		}
	}

	update(db, id, addrs)
	return nil
}

func handleQueryV2(db *leveldb.DB, conn *net.UDPConn, addr *net.UDPAddr, buf []byte) error {
	var pkt discover.Query
	err := pkt.UnmarshalXDR(buf)
	if err != nil {
		return err
	}
	if debug {
		log.Printf("<- %v %#v", addr, pkt)
	}

	var id protocol.DeviceID
	if len(pkt.DeviceID) == 32 {
		// Raw node ID
		copy(id[:], pkt.DeviceID)
	} else {
		err = id.UnmarshalText(pkt.DeviceID)
		if err != nil {
			return err
		}
	}

	lock.Lock()
	queries++
	lock.Unlock()

	addrs := get(db, id)

	now := time.Now().Unix()
	if len(addrs) > 0 {
		ann := discover.Announce{
			Magic: discover.AnnouncementMagic,
			This: discover.Device{
				ID: pkt.DeviceID,
			},
		}
		for _, addr := range addrs {
			if now-addr.seen > cacheLimitSeconds {
				continue
			}
			ann.This.Addresses = append(ann.This.Addresses, discover.Address{IP: addr.ip, Port: addr.port})
		}
		if debug {
			log.Printf("-> %v %#v", addr, pkt)
		}

		if len(ann.This.Addresses) == 0 {
			return nil
		}

		tb := ann.MarshalXDR()
		_, _, err = conn.WriteMsgUDP(tb, nil, addr)
		if err != nil {
			log.Println("QueryV2 response write:", err)
			return nil
		}

		lock.Lock()
		answered++
		lock.Unlock()
	}
	return nil
}

func next(intv int) time.Time {
	d := time.Duration(intv) * time.Second
	t0 := time.Now()
	t1 := t0.Add(d).Truncate(d)
	time.Sleep(t1.Sub(t0))
	return t1
}

func logStats(statsLog io.Writer, intv int) {
	for {
		t := next(intv)

		lock.Lock()

		fmt.Fprintf(statsLog, "%d Queries:%d Answered:%d Announces:%d Unknown:%d Limited:%d\n",
			t.Unix(), queries, answered, announces, unknowns, limited)

		queries = 0
		announces = 0
		answered = 0
		limited = 0
		unknowns = 0

		lock.Unlock()
	}
}

func get(db *leveldb.DB, id protocol.DeviceID) []address {
	var addrs addressList
	val, err := db.Get(id[:], nil)
	if err == nil {
		addrs.UnmarshalXDR(val)
	}
	return addrs.addresses
}

func update(db *leveldb.DB, id protocol.DeviceID, addrs []address) {
	var newAddrs addressList

	val, err := db.Get(id[:], nil)
	if err == nil {
		newAddrs.UnmarshalXDR(val)
	}

nextAddr:
	for _, newAddr := range addrs {
		for i, exAddr := range newAddrs.addresses {
			if bytes.Compare(newAddr.ip, exAddr.ip) == 0 {
				newAddrs.addresses[i] = newAddr
				continue nextAddr
			}
		}
		newAddrs.addresses = append(newAddrs.addresses, newAddr)
	}

	db.Put(id[:], newAddrs.MarshalXDR(), nil)
}

func clean(statsLog io.Writer, db *leveldb.DB) {
	for {
		now := next(cacheLimitSeconds)
		nowSecs := now.Unix()

		var kept, deleted int64
		iter := db.NewIterator(nil, nil)
		for iter.Next() {
			var addrs addressList
			addrs.UnmarshalXDR(iter.Value())

			// Remove expired addresses
			newAddrs := addrs.addresses
			for i := 0; i < len(newAddrs); i++ {
				if nowSecs-newAddrs[i].seen > cacheLimitSeconds {
					newAddrs[i] = newAddrs[len(newAddrs)-1]
					newAddrs = newAddrs[:len(newAddrs)-1]
				}
			}

			// Delete empty records
			if len(newAddrs) == 0 {
				db.Delete(iter.Key(), nil)
				deleted++
				continue
			}

			// Update changed records
			if len(newAddrs) != len(addrs.addresses) {
				addrs.addresses = newAddrs
				db.Put(iter.Key(), addrs.MarshalXDR(), nil)
			}
			kept++
		}
		iter.Release()

		fmt.Fprintf(statsLog, "%d Kept:%d Deleted:%d Took:%0.04fs\n", nowSecs, kept, deleted, time.Since(now).Seconds())
	}
}
