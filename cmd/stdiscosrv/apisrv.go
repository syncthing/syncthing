// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/syncthing/syncthing/lib/protocol"
)

// announcement is the format received from and sent to clients
type announcement struct {
	Seen      time.Time `json:"seen"`
	Addresses []string  `json:"addresses"`
}

type apiSrv struct {
	addr     string
	cert     tls.Certificate
	db       database
	listener net.Listener
	repl     replicator // optional
	useHTTP  bool

	mapsMut sync.Mutex
	misses  map[string]int32
}

type requestID int64

func (i requestID) String() string {
	return fmt.Sprintf("%016x", int64(i))
}

type contextKey int

const idKey contextKey = iota

func newAPISrv(addr string, cert tls.Certificate, db database, repl replicator, useHTTP bool) *apiSrv {
	return &apiSrv{
		addr:    addr,
		cert:    cert,
		db:      db,
		repl:    repl,
		useHTTP: useHTTP,
		misses:  make(map[string]int32),
	}
}

func (s *apiSrv) Serve() {
	if s.useHTTP {
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

	http.HandleFunc("/", s.handler)
	http.HandleFunc("/ping", handlePing)

	srv := &http.Server{
		ReadTimeout:    httpReadTimeout,
		WriteTimeout:   httpWriteTimeout,
		MaxHeaderBytes: httpMaxHeaderBytes,
	}

	if err := srv.Serve(s.listener); err != nil {
		log.Println("Serve:", err)
	}
}

var topCtx = context.Background()

func (s *apiSrv) handler(w http.ResponseWriter, req *http.Request) {
	t0 := time.Now()

	lw := NewLoggingResponseWriter(w)

	defer func() {
		diff := time.Since(t0)
		apiRequestsSeconds.WithLabelValues(req.Method).Observe(diff.Seconds())
		apiRequestsTotal.WithLabelValues(req.Method, strconv.Itoa(lw.statusCode)).Inc()
	}()

	reqID := requestID(rand.Int63())
	ctx := context.WithValue(topCtx, idKey, reqID)

	if debug {
		log.Println(reqID, req.Method, req.URL)
	}

	var remoteIP net.IP
	if s.useHTTP {
		remoteIP = net.ParseIP(req.Header.Get("X-Forwarded-For"))
	} else {
		addr, err := net.ResolveTCPAddr("tcp", req.RemoteAddr)
		if err != nil {
			log.Println("remoteAddr:", err)
			lw.Header().Set("Retry-After", errorRetryAfterString())
			http.Error(lw, "Internal Server Error", http.StatusInternalServerError)
			apiRequestsTotal.WithLabelValues("no_remote_addr").Inc()
			return
		}
		remoteIP = addr.IP
	}

	switch req.Method {
	case "GET":
		s.handleGET(ctx, lw, req)
	case "POST":
		s.handlePOST(ctx, remoteIP, lw, req)
	default:
		http.Error(lw, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

func (s *apiSrv) handleGET(ctx context.Context, w http.ResponseWriter, req *http.Request) {
	reqID := ctx.Value(idKey).(requestID)

	deviceID, err := protocol.DeviceIDFromString(req.URL.Query().Get("device"))
	if err != nil {
		if debug {
			log.Println(reqID, "bad device param")
		}
		lookupRequestsTotal.WithLabelValues("bad_request").Inc()
		w.Header().Set("Retry-After", errorRetryAfterString())
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	key := deviceID.String()
	rec, err := s.db.get(key)
	if err != nil {
		// some sort of internal error
		lookupRequestsTotal.WithLabelValues("internal_error").Inc()
		w.Header().Set("Retry-After", errorRetryAfterString())
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if len(rec.Addresses) == 0 {
		lookupRequestsTotal.WithLabelValues("not_found").Inc()

		s.mapsMut.Lock()
		misses := s.misses[key]
		if misses < rec.Misses {
			misses = rec.Misses + 1
		} else {
			misses++
		}
		s.misses[key] = misses
		s.mapsMut.Unlock()

		if misses%notFoundMissesWriteInterval == 0 {
			rec.Misses = misses
			rec.Missed = time.Now().UnixNano()
			rec.Addresses = nil
			// rec.Seen retained from get
			s.db.put(key, rec)
		}

		w.Header().Set("Retry-After", notFoundRetryAfterString(int(misses)))
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	lookupRequestsTotal.WithLabelValues("success").Inc()

	bs, _ := json.Marshal(announcement{
		Seen:      time.Unix(0, rec.Seen),
		Addresses: addressStrs(rec.Addresses),
	})
	w.Header().Set("Content-Type", "application/json")
	w.Write(bs)
}

func (s *apiSrv) handlePOST(ctx context.Context, remoteIP net.IP, w http.ResponseWriter, req *http.Request) {
	reqID := ctx.Value(idKey).(requestID)

	rawCert := certificateBytes(req)
	if rawCert == nil {
		if debug {
			log.Println(reqID, "no certificates")
		}
		announceRequestsTotal.WithLabelValues("no_certificate").Inc()
		w.Header().Set("Retry-After", errorRetryAfterString())
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	var ann announcement
	if err := json.NewDecoder(req.Body).Decode(&ann); err != nil {
		if debug {
			log.Println(reqID, "decode:", err)
		}
		announceRequestsTotal.WithLabelValues("bad_request").Inc()
		w.Header().Set("Retry-After", errorRetryAfterString())
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	deviceID := protocol.NewDeviceID(rawCert)

	addresses := fixupAddresses(remoteIP, ann.Addresses)
	if len(addresses) == 0 {
		announceRequestsTotal.WithLabelValues("bad_request").Inc()
		w.Header().Set("Retry-After", errorRetryAfterString())
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	if err := s.handleAnnounce(remoteIP, deviceID, addresses); err != nil {
		announceRequestsTotal.WithLabelValues("internal_error").Inc()
		w.Header().Set("Retry-After", errorRetryAfterString())
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	announceRequestsTotal.WithLabelValues("success").Inc()

	w.Header().Set("Reannounce-After", reannounceAfterString())
	w.WriteHeader(http.StatusNoContent)
}

func (s *apiSrv) Stop() {
	s.listener.Close()
}

func (s *apiSrv) handleAnnounce(remote net.IP, deviceID protocol.DeviceID, addresses []string) error {
	key := deviceID.String()
	now := time.Now()
	expire := now.Add(addressExpiryTime).UnixNano()

	dbAddrs := make([]DatabaseAddress, len(addresses))
	for i := range addresses {
		dbAddrs[i].Address = addresses[i]
		dbAddrs[i].Expires = expire
	}

	seen := now.UnixNano()
	if s.repl != nil {
		s.repl.send(key, dbAddrs, seen)
	}
	return s.db.merge(key, dbAddrs, seen)
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

// fixupAddresses checks the list of addresses, removing invalid ones and
// replacing unspecified IPs with the given remote IP.
func fixupAddresses(remote net.IP, addresses []string) []string {
	fixed := make([]string, 0, len(addresses))
	for _, annAddr := range addresses {
		uri, err := url.Parse(annAddr)
		if err != nil {
			continue
		}

		host, port, err := net.SplitHostPort(uri.Host)
		if err != nil {
			continue
		}

		ip := net.ParseIP(host)

		// Some classes of IP are no-go.
		if ip.IsLoopback() || ip.IsMulticast() {
			continue
		}

		if host == "" || ip.IsUnspecified() {
			// Replace the unspecified IP with the request source.

			// ... unless the request source is the loopback address or
			// multicast/unspecified (can't happen, really).
			if remote.IsLoopback() || remote.IsMulticast() || remote.IsUnspecified() {
				continue
			}

			// Do not use IPv6 remote address if requested scheme is ...4
			// (i.e., tcp4, etc.)
			if strings.HasSuffix(uri.Scheme, "4") && remote.To4() == nil {
				continue
			}

			// Do not use IPv4 remote address if requested scheme is ...6
			if strings.HasSuffix(uri.Scheme, "6") && remote.To4() != nil {
				continue
			}

			host = remote.String()
		}

		uri.Host = net.JoinHostPort(host, port)
		fixed = append(fixed, uri.String())
	}

	return fixed
}

type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func NewLoggingResponseWriter(w http.ResponseWriter) *loggingResponseWriter {
	return &loggingResponseWriter{w, http.StatusOK}
}

func (lrw *loggingResponseWriter) WriteHeader(code int) {
	lrw.statusCode = code
	lrw.ResponseWriter.WriteHeader(code)
}

func addressStrs(dbAddrs []DatabaseAddress) []string {
	res := make([]string, len(dbAddrs))
	for i, a := range dbAddrs {
		res[i] = a.Address
	}
	return res
}

func errorRetryAfterString() string {
	return strconv.Itoa(errorRetryAfterSeconds + rand.Intn(errorRetryFuzzSeconds))
}

func notFoundRetryAfterString(misses int) string {
	retryAfterS := notFoundRetryMinSeconds + notFoundRetryIncSeconds*misses
	if retryAfterS > notFoundRetryMaxSeconds {
		retryAfterS = notFoundRetryMaxSeconds
	}
	retryAfterS += rand.Intn(notFoundRetryFuzzSeconds)
	return strconv.Itoa(retryAfterS)
}

func reannounceAfterString() string {
	return strconv.Itoa(reannounceAfterSeconds + rand.Intn(reannounzeFuzzSeconds))
}
