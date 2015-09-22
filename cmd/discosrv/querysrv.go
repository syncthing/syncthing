// Copyright (C) 2014-2015 Jakob Borg and Contributors (see the CONTRIBUTORS file).

package main

import (
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/golang/groupcache/lru"
	"github.com/juju/ratelimit"
	"github.com/syncthing/syncthing/lib/protocol"
)

type querysrv struct {
	addr     string
	db       *sql.DB
	prep     map[string]*sql.Stmt
	limiter  *lru.Cache
	cert     tls.Certificate
	listener net.Listener
}

type announcement struct {
	Direct []string   `json:"direct"`
	Relays []annRelay `json:"relays"`
}

type annRelay struct {
	URL     string `json:"url"`
	Latency int    `json:"latency"`
}

func (s *querysrv) Serve() {
	s.limiter = lru.New(lruSize)

	tlsCfg := &tls.Config{
		Certificates:           []tls.Certificate{s.cert},
		ClientAuth:             tls.RequestClientCert,
		SessionTicketsDisabled: true,
		MinVersion:             tls.VersionTLS12,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
			tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
		},
	}

	http.HandleFunc("/", s.handler)
	http.HandleFunc("/ping", handlePing)

	tlsListener, err := tls.Listen("tcp", s.addr, tlsCfg)
	if err != nil {
		log.Println("Listen:", err)
		return
	}

	s.listener = tlsListener

	srv := &http.Server{
		ReadTimeout:    30 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 2 << 10,
	}

	if err := srv.Serve(tlsListener); err != nil {
		log.Println("Serve:", err)
	}
}

func (s *querysrv) handler(w http.ResponseWriter, req *http.Request) {
	if debug {
		log.Println(req.Method, req.URL)
	}

	remoteAddr, err := net.ResolveTCPAddr("tcp", req.RemoteAddr)
	if err != nil {
		log.Println("remoteAddr:", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if s.limit(remoteAddr.IP) {
		if debug {
			log.Println(remoteAddr.IP, "is limited")
		}
		w.Header().Set("Retry-After", "60")
		http.Error(w, "Too Many Requests", 429)
		return
	}

	switch req.Method {
	case "GET":
		s.handleGET(w, req)
	case "POST":
		s.handlePOST(w, req)
	default:
		globalStats.Error()
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

func (s *querysrv) handleGET(w http.ResponseWriter, req *http.Request) {
	if req.TLS == nil {
		if debug {
			log.Println(req.Method, req.URL, "not TLS")
		}
		globalStats.Error()
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	deviceID, err := protocol.DeviceIDFromString(req.URL.Query().Get("device"))
	if err != nil {
		if debug {
			log.Println(req.Method, req.URL, "bad device param")
		}
		globalStats.Error()
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	var ann announcement

	ann.Direct, err = s.getAddresses(deviceID)
	if err != nil {
		log.Println("getAddresses:", err)
		globalStats.Error()
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	ann.Relays, err = s.getRelays(deviceID)
	if err != nil {
		log.Println("getRelays:", err)
		globalStats.Error()
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if len(ann.Direct)+len(ann.Relays) == 0 {
		globalStats.Error()
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	globalStats.Query()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ann)
}

func (s *querysrv) handlePOST(w http.ResponseWriter, req *http.Request) {
	if req.TLS == nil {
		if debug {
			log.Println(req.Method, req.URL, "not TLS")
		}
		globalStats.Error()
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	if len(req.TLS.PeerCertificates) == 0 {
		if debug {
			log.Println(req.Method, req.URL, "no certificates")
		}
		globalStats.Error()
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	var ann announcement
	if err := json.NewDecoder(req.Body).Decode(&ann); err != nil {
		if debug {
			log.Println(req.Method, req.URL, err)
		}
		globalStats.Error()
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	remoteAddr, err := net.ResolveTCPAddr("tcp", req.RemoteAddr)
	if err != nil {
		log.Println("remoteAddr:", err)
		globalStats.Error()
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	deviceID := protocol.NewDeviceID(req.TLS.PeerCertificates[0].Raw)

	// handleAnnounce returns *two* errors. The first indicates a problem with
	// something the client posted to us. We should return a 400 Bad Request
	// and not worry about it. The second indicates that the request was fine,
	// but something internal fucked up. We should log it and respond with a
	// more apologetic 500 Internal Server Error.
	userErr, internalErr := s.handleAnnounce(remoteAddr.IP, deviceID, ann.Direct, ann.Relays)
	if userErr != nil {
		if debug {
			log.Println(req.Method, req.URL, userErr)
		}
		globalStats.Error()
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	if internalErr != nil {
		log.Println("handleAnnounce:", internalErr)
		globalStats.Error()
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	globalStats.Announce()

	// TODO: Slowly increase this for stable clients
	w.Header().Set("Reannounce-After", "1800")

	// We could return the lookup result here, but it's kind of unnecessarily
	// expensive to go query the database again so we let the client decide to
	// do a lookup if they really care.
	w.WriteHeader(http.StatusNoContent)
}

func (s *querysrv) Stop() {
	s.listener.Close()
}

func (s *querysrv) handleAnnounce(remote net.IP, deviceID protocol.DeviceID, direct []string, relays []annRelay) (userErr, internalErr error) {
	tx, err := s.db.Begin()
	if err != nil {
		internalErr = err
		return
	}

	defer func() {
		// Since we return from a bunch of different places, we handle
		// rollback in the defer.
		if internalErr != nil || userErr != nil {
			tx.Rollback()
		}
	}()

	for _, annAddr := range direct {
		uri, err := url.Parse(annAddr)
		if err != nil {
			userErr = err
			return
		}

		host, port, err := net.SplitHostPort(uri.Host)
		if err != nil {
			userErr = err
			return
		}

		ip := net.ParseIP(host)
		if len(ip) == 0 || ip.IsUnspecified() {
			uri.Host = net.JoinHostPort(remote.String(), port)
		}

		if err := s.updateAddress(tx, deviceID, uri.String()); err != nil {
			internalErr = err
			return
		}
	}

	_, err = tx.Stmt(s.prep["deleteRelay"]).Exec(deviceID.String())
	if err != nil {
		internalErr = err
		return
	}

	for _, relay := range relays {
		uri, err := url.Parse(relay.URL)
		if err != nil {
			userErr = err
			return
		}

		_, err = tx.Stmt(s.prep["insertRelay"]).Exec(deviceID.String(), uri.String(), relay.Latency)
		if err != nil {
			internalErr = err
			return
		}
	}

	if err := s.updateDevice(tx, deviceID); err != nil {
		internalErr = err
		return
	}

	internalErr = tx.Commit()
	return
}

func (s *querysrv) limit(remote net.IP) bool {
	key := remote.String()

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

func (s *querysrv) getRelays(device protocol.DeviceID) ([]annRelay, error) {
	rows, err := s.prep["selectRelay"].Query(device.String())
	if err != nil {
		return nil, err
	}

	var res []annRelay
	for rows.Next() {
		var rel annRelay

		err := rows.Scan(&rel.URL, &rel.Latency)
		if err != nil {
			return nil, err
		}
		res = append(res, rel)
	}

	return res, nil
}

func handlePing(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(204)
}
