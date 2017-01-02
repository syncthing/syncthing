// Copyright (C) 2014-2015 Jakob Borg and Contributors (see the CONTRIBUTORS file).

package main

import (
	"bytes"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/golang/groupcache/lru"
	"github.com/syncthing/syncthing/lib/protocol"
	"golang.org/x/net/context"
	"golang.org/x/time/rate"
)

type querysrv struct {
	addr     string
	db       *sql.DB
	prep     map[string]*sql.Stmt
	limiter  *safeCache
	cert     tls.Certificate
	listener net.Listener
}

type announcement struct {
	Seen      time.Time `json:"seen"`
	Addresses []string  `json:"addresses"`
}

type safeCache struct {
	*lru.Cache
	mut sync.Mutex
}

func (s *safeCache) Get(key string) (val interface{}, ok bool) {
	s.mut.Lock()
	val, ok = s.Cache.Get(key)
	s.mut.Unlock()
	return
}

func (s *safeCache) Add(key string, val interface{}) {
	s.mut.Lock()
	s.Cache.Add(key, val)
	s.mut.Unlock()
}

type requestID int64

func (i requestID) String() string {
	return fmt.Sprintf("%016x", int64(i))
}

type contextKey int

const idKey contextKey = iota

func negCacheFor(lastSeen time.Time) int {
	since := time.Since(lastSeen).Seconds()
	if since >= maxDeviceAge {
		return maxNegCache
	}
	if since < 0 {
		// That's weird
		return minNegCache
	}

	// Return a value linearly scaled from minNegCache (at zero seconds ago)
	// to maxNegCache (at maxDeviceAge seconds ago).
	r := since / maxDeviceAge
	return int(minNegCache + r*(maxNegCache-minNegCache))
}

func (s *querysrv) Serve() {
	s.limiter = &safeCache{
		Cache: lru.New(lruSize),
	}

	if useHTTP {
		listener, err := net.Listen("tcp", s.addr)
		if err != nil {
			log.Println("Listen:", err)
			return
		}
		s.listener = listener
	} else {
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

		tlsListener, err := tls.Listen("tcp", s.addr, tlsCfg)
		if err != nil {
			log.Println("Listen:", err)
			return
		}
		s.listener = tlsListener
	}

	http.HandleFunc("/v2/", s.handler)
	http.HandleFunc("/ping", handlePing)

	srv := &http.Server{
		ReadTimeout:    5 * time.Second,
		WriteTimeout:   5 * time.Second,
		MaxHeaderBytes: 1 << 10,
	}

	if err := srv.Serve(s.listener); err != nil {
		log.Println("Serve:", err)
	}
}

var topCtx = context.Background()

func (s *querysrv) handler(w http.ResponseWriter, req *http.Request) {
	reqID := requestID(rand.Int63())
	ctx := context.WithValue(topCtx, idKey, reqID)

	if debug {
		log.Println(reqID, req.Method, req.URL)
	}

	t0 := time.Now()
	defer func() {
		diff := time.Since(t0)
		var comment string
		if diff > time.Second {
			comment = "(very slow request)"
		} else if diff > 100*time.Millisecond {
			comment = "(slow request)"
		}
		if comment != "" || debug {
			log.Println(reqID, req.Method, req.URL, "completed in", diff, comment)
		}
	}()

	var remoteIP net.IP
	if useHTTP {
		remoteIP = net.ParseIP(req.Header.Get("X-Forwarded-For"))
	} else {
		addr, err := net.ResolveTCPAddr("tcp", req.RemoteAddr)
		if err != nil {
			log.Println("remoteAddr:", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		remoteIP = addr.IP
	}

	if s.limit(remoteIP) {
		if debug {
			log.Println(remoteIP, "is limited")
		}
		w.Header().Set("Retry-After", "60")
		http.Error(w, "Too Many Requests", 429)
		return
	}

	switch req.Method {
	case "GET":
		s.handleGET(ctx, w, req)
	case "POST":
		s.handlePOST(ctx, remoteIP, w, req)
	default:
		globalStats.Error()
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

func (s *querysrv) handleGET(ctx context.Context, w http.ResponseWriter, req *http.Request) {
	reqID := ctx.Value(idKey).(requestID)

	deviceID, err := protocol.DeviceIDFromString(req.URL.Query().Get("device"))
	if err != nil {
		if debug {
			log.Println(reqID, "bad device param")
		}
		globalStats.Error()
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	var ann announcement

	ann.Seen, err = s.getDeviceSeen(deviceID)
	negCache := strconv.Itoa(negCacheFor(ann.Seen))
	w.Header().Set("Retry-After", negCache)
	w.Header().Set("Cache-Control", "public, max-age="+negCache)

	if err != nil {
		// The device is not in the database.
		globalStats.Query()
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	t0 := time.Now()
	ann.Addresses, err = s.getAddresses(ctx, deviceID)
	if err != nil {
		log.Println(reqID, "getAddresses:", err)
		globalStats.Error()
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if debug {
		log.Println(reqID, "getAddresses in", time.Since(t0))
	}

	globalStats.Query()

	if len(ann.Addresses) == 0 {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	globalStats.Answer()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ann)
}

func (s *querysrv) handlePOST(ctx context.Context, remoteIP net.IP, w http.ResponseWriter, req *http.Request) {
	reqID := ctx.Value(idKey).(requestID)

	rawCert := certificateBytes(req)
	if rawCert == nil {
		if debug {
			log.Println(reqID, "no certificates")
		}
		globalStats.Error()
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	var ann announcement
	if err := json.NewDecoder(req.Body).Decode(&ann); err != nil {
		if debug {
			log.Println(reqID, "decode:", err)
		}
		globalStats.Error()
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	deviceID := protocol.NewDeviceID(rawCert)

	// handleAnnounce returns *two* errors. The first indicates a problem with
	// something the client posted to us. We should return a 400 Bad Request
	// and not worry about it. The second indicates that the request was fine,
	// but something internal messed up. We should log it and respond with a
	// more apologetic 500 Internal Server Error.
	userErr, internalErr := s.handleAnnounce(ctx, remoteIP, deviceID, ann.Addresses)
	if userErr != nil {
		if debug {
			log.Println(reqID, "handleAnnounce:", userErr)
		}
		globalStats.Error()
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	if internalErr != nil {
		log.Println(reqID, "handleAnnounce:", internalErr)
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

func (s *querysrv) handleAnnounce(ctx context.Context, remote net.IP, deviceID protocol.DeviceID, addresses []string) (userErr, internalErr error) {
	reqID := ctx.Value(idKey).(requestID)

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

	for _, annAddr := range addresses {
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
		if host == "" || ip.IsUnspecified() {
			// Do not use IPv6 remote address if requested scheme is tcp4
			if uri.Scheme == "tcp4" && remote.To4() == nil {
				continue
			}

			// Do not use IPv4 remote address if requested scheme is tcp6
			if uri.Scheme == "tcp6" && remote.To4() != nil {
				continue
			}

			host = remote.String()
		}

		uri.Host = net.JoinHostPort(host, port)

		if err := s.updateAddress(ctx, tx, deviceID, uri.String()); err != nil {
			internalErr = err
			return
		}
	}

	if err := s.updateDevice(ctx, tx, deviceID); err != nil {
		internalErr = err
		return
	}

	t0 := time.Now()
	internalErr = tx.Commit()
	if debug {
		log.Println(reqID, "commit in", time.Since(t0))
	}
	return
}

func (s *querysrv) limit(remote net.IP) bool {
	key := remote.String()

	bkt, ok := s.limiter.Get(key)
	if ok {
		bkt := bkt.(*rate.Limiter)
		if !bkt.Allow() {
			// Rate limit exceeded; ignore packet
			return true
		}
	} else {
		// limitAvg is in packets per ten seconds.
		s.limiter.Add(key, rate.NewLimiter(rate.Limit(limitAvg)/10, limitBurst))
	}

	return false
}

func (s *querysrv) updateDevice(ctx context.Context, tx *sql.Tx, device protocol.DeviceID) error {
	reqID := ctx.Value(idKey).(requestID)
	t0 := time.Now()
	res, err := tx.Stmt(s.prep["updateDevice"]).Exec(device.String())
	if err != nil {
		return err
	}
	if debug {
		log.Println(reqID, "updateDevice in", time.Since(t0))
	}

	if rows, _ := res.RowsAffected(); rows == 0 {
		t0 = time.Now()
		_, err := tx.Stmt(s.prep["insertDevice"]).Exec(device.String())
		if err != nil {
			return err
		}
		if debug {
			log.Println(reqID, "insertDevice in", time.Since(t0))
		}
	}

	return nil
}

func (s *querysrv) updateAddress(ctx context.Context, tx *sql.Tx, device protocol.DeviceID, uri string) error {
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

func (s *querysrv) getAddresses(ctx context.Context, device protocol.DeviceID) ([]string, error) {
	rows, err := s.prep["selectAddress"].Query(device.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

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

func (s *querysrv) getDeviceSeen(device protocol.DeviceID) (time.Time, error) {
	row := s.prep["selectDevice"].QueryRow(device.String())
	var seen time.Time
	if err := row.Scan(&seen); err != nil {
		return time.Time{}, err
	}
	return seen.In(time.UTC), nil
}

func handlePing(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(204)
}

func certificateBytes(req *http.Request) []byte {
	if req.TLS != nil && len(req.TLS.PeerCertificates) > 0 {
		return req.TLS.PeerCertificates[0].Raw
	}

	if hdr := req.Header.Get("X-SSL-Cert"); hdr != "" {
		bs := []byte(hdr)
		// The certificate is in PEM format but with spaces for newlines. We
		// need to reinstate the newlines for the PEM decoder. But we need to
		// leave the spaces in the BEGIN and END lines - the first and last
		// space - alone.
		firstSpace := bytes.Index(bs, []byte(" "))
		lastSpace := bytes.LastIndex(bs, []byte(" "))
		for i := firstSpace + 1; i < lastSpace; i++ {
			if bs[i] == ' ' {
				bs[i] = '\n'
			}
		}
		block, _ := pem.Decode(bs)
		if block == nil {
			// Decoding failed
			return nil
		}
		return block.Bytes
	}

	return nil
}
