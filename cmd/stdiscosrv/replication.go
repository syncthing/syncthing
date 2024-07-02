// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	io "io"
	"log"
	"net"
	"time"

	"github.com/syncthing/syncthing/lib/protocol"
)

const (
	replicationReadTimeout       = time.Minute
	replicationWriteTimeout      = 30 * time.Second
	replicationHeartbeatInterval = time.Second * 30
)

type replicator interface {
	send(key string, addrs []DatabaseAddress, seen int64)
}

// a replicationSender tries to connect to the remote address and provide
// them with a feed of replication updates.
type replicationSender struct {
	dst        string
	cert       tls.Certificate // our certificate
	allowedIDs []protocol.DeviceID
	outbox     chan ReplicationRecord
}

func newReplicationSender(dst string, cert tls.Certificate, allowedIDs []protocol.DeviceID) *replicationSender {
	return &replicationSender{
		dst:        dst,
		cert:       cert,
		allowedIDs: allowedIDs,
		outbox:     make(chan ReplicationRecord, replicationOutboxSize),
	}
}

func (s *replicationSender) Serve(ctx context.Context) error {
	// Sleep a little at startup. Peers often restart at the same time, and
	// this avoid the service failing and entering backoff state
	// unnecessarily, while also reducing the reconnect rate to something
	// reasonable by default.
	time.Sleep(2 * time.Second)

	tlsCfg := &tls.Config{
		Certificates:       []tls.Certificate{s.cert},
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: true,
	}

	// Dial the TLS connection.
	conn, err := tls.Dial("tcp", s.dst, tlsCfg)
	if err != nil {
		log.Println("Replication connect:", err)
		return err
	}
	defer func() {
		conn.SetWriteDeadline(time.Now().Add(time.Second))
		conn.Close()
	}()

	// The replication stream is not especially latency sensitive, but it is
	// quite a lot of data in small writes. Make it more efficient.
	if tcpc, ok := conn.NetConn().(*net.TCPConn); ok {
		_ = tcpc.SetNoDelay(false)
	}

	// Get the other side device ID.
	remoteID, err := deviceID(conn)
	if err != nil {
		log.Println("Replication connect:", err)
		return err
	}

	// Verify it's in the set of allowed device IDs.
	if !deviceIDIn(remoteID, s.allowedIDs) {
		log.Println("Replication connect: unexpected device ID:", remoteID)
		return err
	}

	heartBeatTicker := time.NewTicker(replicationHeartbeatInterval)
	defer heartBeatTicker.Stop()

	// Send records.
	buf := make([]byte, 1024)
	for {
		select {
		case <-heartBeatTicker.C:
			if len(s.outbox) > 0 {
				// No need to send heartbeats if there are events/prevrious
				// heartbeats to send, they will keep the connection alive.
				continue
			}
			// Empty replication message is the heartbeat:
			s.outbox <- ReplicationRecord{}

		case rec := <-s.outbox:
			// Buffer must hold record plus four bytes for size
			size := rec.Size()
			if len(buf) < size+4 {
				buf = make([]byte, size+4)
			}

			// Record comes after the four bytes size
			n, err := rec.MarshalTo(buf[4:])
			if err != nil {
				// odd to get an error here, but we haven't sent anything
				// yet so it's not fatal
				replicationSendsTotal.WithLabelValues("error").Inc()
				log.Println("Replication marshal:", err)
				continue
			}
			binary.BigEndian.PutUint32(buf, uint32(n))

			// Send
			conn.SetWriteDeadline(time.Now().Add(replicationWriteTimeout))
			if _, err := conn.Write(buf[:4+n]); err != nil {
				replicationSendsTotal.WithLabelValues("error").Inc()
				log.Println("Replication write:", err)
				// Yes, we are losing the replication event here.
				return err
			}
			replicationSendsTotal.WithLabelValues("success").Inc()

		case <-ctx.Done():
			return nil
		}
	}
}

func (s *replicationSender) String() string {
	return fmt.Sprintf("replicationSender(%q)", s.dst)
}

func (s *replicationSender) send(key string, ps []DatabaseAddress, seen int64) {
	item := ReplicationRecord{
		Key:       key,
		Addresses: ps,
		Seen:      seen,
	}

	// The send should never block. The inbox is suitably buffered for at
	// least a few seconds of stalls, which shouldn't happen in practice.
	select {
	case s.outbox <- item:
	default:
		replicationSendsTotal.WithLabelValues("drop").Inc()
	}
}

// a replicationMultiplexer sends to multiple replicators
type replicationMultiplexer []replicator

func (m replicationMultiplexer) send(key string, ps []DatabaseAddress, seen int64) {
	for _, s := range m {
		// each send is nonblocking
		s.send(key, ps, seen)
	}
}

// replicationListener accepts incoming connections and reads replication
// items from them. Incoming items are applied to the KV store.
type replicationListener struct {
	addr       string
	cert       tls.Certificate
	allowedIDs []protocol.DeviceID
	db         database
}

func newReplicationListener(addr string, cert tls.Certificate, allowedIDs []protocol.DeviceID, db database) *replicationListener {
	return &replicationListener{
		addr:       addr,
		cert:       cert,
		allowedIDs: allowedIDs,
		db:         db,
	}
}

func (l *replicationListener) Serve(ctx context.Context) error {
	tlsCfg := &tls.Config{
		Certificates:       []tls.Certificate{l.cert},
		ClientAuth:         tls.RequestClientCert,
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: true,
	}

	lst, err := tls.Listen("tcp", l.addr, tlsCfg)
	if err != nil {
		log.Println("Replication listen:", err)
		return err
	}
	defer lst.Close()

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		// Accept a connection
		conn, err := lst.Accept()
		if err != nil {
			log.Println("Replication accept:", err)
			return err
		}

		// Figure out the other side device ID
		remoteID, err := deviceID(conn.(*tls.Conn))
		if err != nil {
			log.Println("Replication accept:", err)
			conn.SetWriteDeadline(time.Now().Add(time.Second))
			conn.Close()
			continue
		}

		// Verify it is in the set of allowed device IDs
		if !deviceIDIn(remoteID, l.allowedIDs) {
			log.Println("Replication accept: unexpected device ID:", remoteID)
			conn.SetWriteDeadline(time.Now().Add(time.Second))
			conn.Close()
			continue
		}

		go l.handle(ctx, conn)
	}
}

func (l *replicationListener) String() string {
	return fmt.Sprintf("replicationListener(%q)", l.addr)
}

func (l *replicationListener) handle(ctx context.Context, conn net.Conn) {
	defer func() {
		conn.SetWriteDeadline(time.Now().Add(time.Second))
		conn.Close()
	}()

	buf := make([]byte, 1024)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		conn.SetReadDeadline(time.Now().Add(replicationReadTimeout))

		// First four bytes are the size
		if _, err := io.ReadFull(conn, buf[:4]); err != nil {
			log.Println("Replication read size:", err)
			replicationRecvsTotal.WithLabelValues("error").Inc()
			return
		}

		// Read the rest of the record
		size := int(binary.BigEndian.Uint32(buf[:4]))
		if len(buf) < size {
			buf = make([]byte, size)
		}

		if size == 0 {
			// Heartbeat, ignore
			continue
		}

		if _, err := io.ReadFull(conn, buf[:size]); err != nil {
			log.Println("Replication read record:", err)
			replicationRecvsTotal.WithLabelValues("error").Inc()
			return
		}

		// Unmarshal
		var rec ReplicationRecord
		if err := rec.Unmarshal(buf[:size]); err != nil {
			log.Println("Replication unmarshal:", err)
			replicationRecvsTotal.WithLabelValues("error").Inc()
			continue
		}

		// Store
		l.db.merge(rec.Key, rec.Addresses, rec.Seen)
		replicationRecvsTotal.WithLabelValues("success").Inc()
	}
}

func deviceID(conn *tls.Conn) (protocol.DeviceID, error) {
	// Handshake may not be complete on the server side yet, which we need
	// to get the client certificate.
	if !conn.ConnectionState().HandshakeComplete {
		if err := conn.Handshake(); err != nil {
			return protocol.DeviceID{}, err
		}
	}

	// We expect exactly one certificate.
	certs := conn.ConnectionState().PeerCertificates
	if len(certs) != 1 {
		return protocol.DeviceID{}, fmt.Errorf("unexpected number of certificates (%d != 1)", len(certs))
	}

	return protocol.NewDeviceID(certs[0].Raw), nil
}

func deviceIDIn(id protocol.DeviceID, ids []protocol.DeviceID) bool {
	for _, candidate := range ids {
		if id == candidate {
			return true
		}
	}
	return false
}
