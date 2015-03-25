// Copyright (C) 2014-2015 Jakob Borg and Contributors (see the CONTRIBUTORS file).

package main

import (
	"database/sql"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"time"

	"github.com/golang/groupcache/lru"
	"github.com/juju/ratelimit"
	"github.com/syncthing/protocol"
	"github.com/syncthing/syncthing/internal/discover"
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
			err := s.handleAnnounceV2(addr, buf)
			globalStats.Announce()
			if err != nil {
				log.Println("Announce:", err)
				globalStats.Error()
			}

		case discover.QueryMagic:
			err := s.handleQueryV2(conn, addr, buf)
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

func (s *querysrv) handleAnnounceV2(addr *net.UDPAddr, buf []byte) error {
	var pkt discover.Announce
	err := pkt.UnmarshalXDR(buf)
	if err != nil && err != io.EOF {
		return err
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

	if id == protocol.LocalDeviceID {
		return fmt.Errorf("Rejecting announce for local device ID from %v", addr)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}

	for _, annAddr := range pkt.This.Addresses {
		tip := annAddr.IP
		if len(tip) == 0 {
			tip = addr.IP
		}
		if err := s.updateAddress(tx, id, tip, annAddr.Port); err != nil {
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

func (s *querysrv) handleQueryV2(conn *net.UDPConn, addr *net.UDPAddr, buf []byte) error {
	var pkt discover.Query
	err := pkt.UnmarshalXDR(buf)
	if err != nil {
		return err
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

	addrs, err := s.getAddresses(id)
	if err != nil {
		return err
	}

	if len(addrs) > 0 {
		ann := discover.Announce{
			Magic: discover.AnnouncementMagic,
			This: discover.Device{
				ID:        pkt.DeviceID,
				Addresses: addrs,
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

func (s *querysrv) updateAddress(tx *sql.Tx, device protocol.DeviceID, ip net.IP, port uint16) error {
	res, err := tx.Stmt(s.prep["updateAddress"]).Exec(device.String(), ip.String(), port)
	if err != nil {
		return err
	}

	if rows, _ := res.RowsAffected(); rows == 0 {
		_, err := tx.Stmt(s.prep["insertAddress"]).Exec(device.String(), ip.String(), port)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *querysrv) getAddresses(device protocol.DeviceID) ([]discover.Address, error) {
	rows, err := s.prep["selectAddress"].Query(device.String())
	if err != nil {
		return nil, err
	}

	var res []discover.Address
	for rows.Next() {
		var addr string
		var port int
		err := rows.Scan(&addr, &port)
		if err != nil {
			log.Println("Scan:", err)
			continue
		}
		ip := net.ParseIP(addr)
		bs := ip.To4()
		if bs == nil {
			bs = ip.To16()
		}
		res = append(res, discover.Address{IP: []byte(bs), Port: uint16(port)})
	}

	return res, nil
}
