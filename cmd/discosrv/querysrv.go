// Copyright (C) 2014-2015 Jakob Borg and Contributors (see the CONTRIBUTORS file).

package main

import (
	"database/sql"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"net/url"
	"time"

	"github.com/golang/groupcache/lru"
	"github.com/juju/ratelimit"
	"github.com/syncthing/protocol"
	"github.com/syncthing/syncthing/lib/discover"
)

type querysrv struct {
	addr    *net.UDPAddr
	db      *sql.DB
	prep    map[string]*sql.Stmt
	limiter *lru.Cache
}

func (s *querysrv) Serve() {
	s.limiter = lru.New(lruSize)

	conn, err := net.ListenUDP("udp", s.addr)
	if err != nil {
		log.Println("Listen:", err)
		return
	}

	// Attempt to set the read and write buffers to 2^24 bytes (16 MB) or as high as
	// possible.
	for i := 24; i >= 16; i-- {
		if conn.SetReadBuffer(1<<uint(i)) == nil {
			break
		}
	}
	for i := 24; i >= 16; i-- {
		if conn.SetWriteBuffer(1<<uint(i)) == nil {
			break
		}
	}

	var buf = make([]byte, 1024)
	for {
		buf = buf[:cap(buf)]
		n, addr, err := conn.ReadFromUDP(buf)
		if err != nil {
			log.Println("Read:", err)
			return
		}

		if s.limit(addr) {
			// Rate limit in effect for source
			continue
		}

		if n < 4 {
			log.Printf("Received short packet (%d bytes)", n)
			continue
		}

		buf = buf[:n]
		magic := binary.BigEndian.Uint32(buf)

		switch magic {
		case discover.AnnouncementMagic:
			err := s.handleAnnounce(addr, buf)
			globalStats.Announce()
			if err != nil {
				log.Println("Announce:", err)
				globalStats.Error()
			}

		case discover.QueryMagic:
			err := s.handleQuery(conn, addr, buf)
			globalStats.Query()
			if err != nil {
				log.Println("Query:", err)
				globalStats.Error()
			}

		default:
			globalStats.Error()
		}
	}
}

func (s *querysrv) Stop() {
	panic("stop unimplemented")
}

func (s *querysrv) handleAnnounce(addr *net.UDPAddr, buf []byte) error {
	var pkt discover.Announce
	err := pkt.UnmarshalXDR(buf)
	if err != nil && err != io.EOF {
		return err
	}

	var id protocol.DeviceID
	copy(id[:], pkt.This.ID)

	if id == protocol.LocalDeviceID {
		return fmt.Errorf("Rejecting announce for local device ID from %v", addr)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}

	for _, annAddr := range pkt.This.Addresses {
		uri, err := url.Parse(annAddr)
		if err != nil {
			continue
		}

		host, port, err := net.SplitHostPort(uri.Host)
		if err != nil {
			continue
		}

		if len(host) == 0 {
			uri.Host = net.JoinHostPort(addr.IP.String(), port)
		}

		if err := s.updateAddress(tx, id, uri.String()); err != nil {
			tx.Rollback()
			return err
		}
	}

	_, err = tx.Stmt(s.prep["deleteRelay"]).Exec(id.String())
	if err != nil {
		tx.Rollback()
		return err
	}

	for _, relay := range pkt.This.Relays {
		uri, err := url.Parse(relay.Address)
		if err != nil {
			continue
		}

		_, err = tx.Stmt(s.prep["insertRelay"]).Exec(id.String(), uri, relay.Latency)
		if err != nil {
			tx.Rollback()
			return err
		}
	}

	if err := s.updateDevice(tx, id); err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit()
}

func (s *querysrv) handleQuery(conn *net.UDPConn, addr *net.UDPAddr, buf []byte) error {
	var pkt discover.Query
	err := pkt.UnmarshalXDR(buf)
	if err != nil {
		return err
	}

	var id protocol.DeviceID
	copy(id[:], pkt.DeviceID)

	addrs, err := s.getAddresses(id)
	if err != nil {
		return err
	}

	relays, err := s.getRelays(id)
	if err != nil {
		return err
	}

	if len(addrs) > 0 {
		ann := discover.Announce{
			Magic: discover.AnnouncementMagic,
			This: discover.Device{
				ID:        pkt.DeviceID,
				Addresses: addrs,
				Relays:    relays,
			},
		}

		tb, err := ann.MarshalXDR()
		if err != nil {
			return fmt.Errorf("QueryV2 response marshal: %v", err)
		}
		_, err = conn.WriteToUDP(tb, addr)
		if err != nil {
			return fmt.Errorf("QueryV2 response write: %v", err)
		}

		globalStats.Answer()
	}

	return nil
}

func (s *querysrv) limit(addr *net.UDPAddr) bool {
	key := addr.IP.String()

	bkt, ok := s.limiter.Get(key)
	if ok {
		bkt := bkt.(*ratelimit.Bucket)
		if bkt.TakeAvailable(1) != 1 {
			// Rate limit exceeded; ignore packet
			return true
		}
	} else {
		// One packet per ten seconds average rate, burst ten packets
		s.limiter.Add(key, ratelimit.NewBucket(10*time.Second/time.Duration(limitAvg), int64(limitBurst)))
	}

	return false
}

func (s *querysrv) updateDevice(tx *sql.Tx, device protocol.DeviceID) error {
	res, err := tx.Stmt(s.prep["updateDevice"]).Exec(device.String())
	if err != nil {
		return err
	}

	if rows, _ := res.RowsAffected(); rows == 0 {
		_, err := tx.Stmt(s.prep["insertDevice"]).Exec(device.String())
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *querysrv) updateAddress(tx *sql.Tx, device protocol.DeviceID, uri string) error {
	res, err := tx.Stmt(s.prep["updateAddress"]).Exec(device.String(), uri)
	if err != nil {
		return err
	}

	if rows, _ := res.RowsAffected(); rows == 0 {
		_, err := tx.Stmt(s.prep["insertAddress"]).Exec(device.String(), uri)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *querysrv) getAddresses(device protocol.DeviceID) ([]string, error) {
	rows, err := s.prep["selectAddress"].Query(device.String())
	if err != nil {
		return nil, err
	}

	var res []string
	for rows.Next() {
		var addr string

		err := rows.Scan(&addr)
		if err != nil {
			log.Println("Scan:", err)
			continue
		}
		res = append(res, addr)
	}

	return res, nil
}

func (s *querysrv) getRelays(device protocol.DeviceID) ([]discover.Relay, error) {
	rows, err := s.prep["selectRelay"].Query(device.String())
	if err != nil {
		return nil, err
	}

	var res []discover.Relay
	for rows.Next() {
		var addr string
		var latency int32

		err := rows.Scan(&addr, &latency)
		if err != nil {
			log.Println("Scan:", err)
			continue
		}
		res = append(res, discover.Relay{
			Address: addr,
			Latency: latency,
		})
	}

	return res, nil
}
