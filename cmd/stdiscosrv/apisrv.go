// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	io "io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/stringutil"
)

// announcement is the format received from and sent to clients
type announcement struct {
	Seen      time.Time `json:"seen"`
	Addresses []string  `json:"addresses"`
}

type apiSrv struct {
	addr           string
	cert           tls.Certificate
	db             database
	listener       net.Listener
	repl           replicator // optional
	useHTTP        bool
	missesIncrease int

	mapsMut sync.Mutex
	misses  map[string]int32
}

type requestID int64

func (i requestID) String() string {
	return fmt.Sprintf("%016x", int64(i))
}

type contextKey int

const idKey contextKey = iota

func newAPISrv(addr string, cert tls.Certificate, db database, repl replicator, useHTTP bool, missesIncrease int) *apiSrv {
	return &apiSrv{
		addr:           addr,
		cert:           cert,
		db:             db,
		repl:           repl,
		useHTTP:        useHTTP,
		misses:         make(map[string]int32),
		missesIncrease: missesIncrease,
	}
}

func (s *apiSrv) Serve(_ context.Context) error {
	if s.useHTTP {
		listener, err := net.Listen("tcp", s.addr)
		if err != nil {
			log.Println("Listen:", err)
			return err
		}
		s.listener = listener
	} else {
		tlsCfg := &tls.Config{
			Certificates: []tls.Certificate{s.cert},
			ClientAuth:   tls.RequestClientCert,
			MinVersion:   tls.VersionTLS12,
			NextProtos:   []string{"h2", "http/1.1"},
		}

		tlsListener, err := tls.Listen("tcp", s.addr, tlsCfg)
		if err != nil {
			log.Println("Listen:", err)
			return err
		}
		s.listener = tlsListener
	}

	http.HandleFunc("/", s.handler)
	http.HandleFunc("/ping", handlePing)

	srv := &http.Server{
		ReadTimeout:    httpReadTimeout,
		WriteTimeout:   httpWriteTimeout,
		MaxHeaderBytes: httpMaxHeaderBytes,
		ErrorLog:       log.New(io.Discard, "", 0),
	}

	err := srv.Serve(s.listener)
	if err != nil {
		log.Println("Serve:", err)
	}
	return err
}

func (s *apiSrv) handler(w http.ResponseWriter, req *http.Request) {
	t0 := time.Now()

	lw := NewLoggingResponseWriter(w)

	defer func() {
		diff := time.Since(t0)
		apiRequestsSeconds.WithLabelValues(req.Method).Observe(diff.Seconds())
		apiRequestsTotal.WithLabelValues(req.Method, strconv.Itoa(lw.statusCode)).Inc()
	}()

	reqID := requestID(rand.Int63())
	req = req.WithContext(context.WithValue(req.Context(), idKey, reqID))

	if debug {
		log.Println(reqID, req.Method, req.URL, req.Proto)
	}

	remoteAddr := &net.TCPAddr{
		IP:   nil,
		Port: -1,
	}

	if s.useHTTP {
		// X-Forwarded-For can have multiple client IPs; split using the comma separator
		forwardIP, _, _ := strings.Cut(req.Header.Get("X-Forwarded-For"), ",")

		// net.ParseIP will return nil if leading/trailing whitespace exists; use strings.TrimSpace()
		remoteAddr.IP = net.ParseIP(strings.TrimSpace(forwardIP))

		if parsedPort, err := strconv.ParseInt(req.Header.Get("X-Client-Port"), 10, 0); err == nil {
			remoteAddr.Port = int(parsedPort)
		}
	} else {
		var err error
		remoteAddr, err = net.ResolveTCPAddr("tcp", req.RemoteAddr)
		if err != nil {
			log.Println("remoteAddr:", err)
			lw.Header().Set("Retry-After", errorRetryAfterString())
			http.Error(lw, "Internal Server Error", http.StatusInternalServerError)
			apiRequestsTotal.WithLabelValues("no_remote_addr").Inc()
			return
		}
	}

	switch req.Method {
	case http.MethodGet:
		s.handleGET(lw, req)
	case http.MethodPost:
		s.handlePOST(remoteAddr, lw, req)
	default:
		http.Error(lw, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

func (s *apiSrv) handleGET(w http.ResponseWriter, req *http.Request) {
	reqID := req.Context().Value(idKey).(requestID)

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
			misses = rec.Misses
		}
		misses += int32(s.missesIncrease)
		s.misses[key] = misses
		s.mapsMut.Unlock()

		if misses >= notFoundMissesWriteInterval {
			rec.Misses = misses
			rec.Missed = time.Now().UnixNano()
			rec.Addresses = nil
			// rec.Seen retained from get
			s.db.put(key, rec)
		}

		afterS := notFoundRetryAfterSeconds(int(misses))
		retryAfterHistogram.Observe(float64(afterS))
		w.Header().Set("Retry-After", strconv.Itoa(afterS))
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	lookupRequestsTotal.WithLabelValues("success").Inc()

	w.Header().Set("Content-Type", "application/json")
	var bw io.Writer = w

	// Use compression if the client asks for it
	if strings.Contains(req.Header.Get("Accept-Encoding"), "gzip") {
		w.Header().Set("Content-Encoding", "gzip")
		gw := gzip.NewWriter(bw)
		defer gw.Close()
		bw = gw
	}

	json.NewEncoder(bw).Encode(announcement{
		Seen:      time.Unix(0, rec.Seen).Truncate(time.Second),
		Addresses: addressStrs(rec.Addresses),
	})
}

func (s *apiSrv) handlePOST(remoteAddr *net.TCPAddr, w http.ResponseWriter, req *http.Request) {
	reqID := req.Context().Value(idKey).(requestID)

	rawCert, err := certificateBytes(req)
	if err != nil {
		if debug {
			log.Println(reqID, "no certificates:", err)
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

	addresses := fixupAddresses(remoteAddr, ann.Addresses)
	if len(addresses) == 0 {
		announceRequestsTotal.WithLabelValues("bad_request").Inc()
		w.Header().Set("Retry-After", errorRetryAfterString())
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	if err := s.handleAnnounce(deviceID, addresses); err != nil {
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

func (s *apiSrv) handleAnnounce(deviceID protocol.DeviceID, addresses []string) error {
	key := deviceID.String()
	now := time.Now()
	expire := now.Add(addressExpiryTime).UnixNano()

	dbAddrs := make([]DatabaseAddress, len(addresses))
	for i := range addresses {
		dbAddrs[i].Address = addresses[i]
		dbAddrs[i].Expires = expire
	}

	// The address slice must always be sorted for database merges to work
	// properly.
	sort.Sort(databaseAddressOrder(dbAddrs))

	seen := now.UnixNano()
	if s.repl != nil {
		s.repl.send(key, dbAddrs, seen)
	}
	return s.db.merge(key, dbAddrs, seen)
}

func handlePing(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(204)
}

func certificateBytes(req *http.Request) ([]byte, error) {
	if req.TLS != nil && len(req.TLS.PeerCertificates) > 0 {
		return req.TLS.PeerCertificates[0].Raw, nil
	}

	var bs []byte

	if hdr := req.Header.Get("X-SSL-Cert"); hdr != "" {
		if strings.Contains(hdr, "%") {
			// Nginx using $ssl_client_escaped_cert
			// The certificate is in PEM format with url encoding.
			// We need to decode for the PEM decoder
			hdr, err := url.QueryUnescape(hdr)
			if err != nil {
				// Decoding failed
				return nil, err
			}

			bs = []byte(hdr)
		} else {
			// Nginx using $ssl_client_cert
			// The certificate is in PEM format but with spaces for newlines. We
			// need to reinstate the newlines for the PEM decoder. But we need to
			// leave the spaces in the BEGIN and END lines - the first and last
			// space - alone.
			bs = []byte(hdr)
			firstSpace := bytes.Index(bs, []byte(" "))
			lastSpace := bytes.LastIndex(bs, []byte(" "))
			for i := firstSpace + 1; i < lastSpace; i++ {
				if bs[i] == ' ' {
					bs[i] = '\n'
				}
			}
		}
	} else if hdr := req.Header.Get("X-Tls-Client-Cert-Der-Base64"); hdr != "" {
		// Caddy {tls_client_certificate_der_base64}
		hdr, err := base64.StdEncoding.DecodeString(hdr)
		if err != nil {
			// Decoding failed
			return nil, err
		}

		bs = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: hdr})
	} else if hdr := req.Header.Get("X-Forwarded-Tls-Client-Cert"); hdr != "" {
		// Traefik 2 passtlsclientcert
		//
		// The certificate is in PEM format, maybe with URL encoding
		// (depends on Traefik version) but without newlines and start/end
		// statements. We need to decode, reinstate the newlines every 64
		// character and add statements for the PEM decoder

		if strings.Contains(hdr, "%") {
			if unesc, err := url.QueryUnescape(hdr); err == nil {
				hdr = unesc
			}
		}

		for i := 64; i < len(hdr); i += 65 {
			hdr = hdr[:i] + "\n" + hdr[i:]
		}

		hdr = "-----BEGIN CERTIFICATE-----\n" + hdr
		hdr += "\n-----END CERTIFICATE-----\n"
		bs = []byte(hdr)
	}

	if bs == nil {
		return nil, errors.New("empty certificate header")
	}

	block, _ := pem.Decode(bs)
	if block == nil {
		// Decoding failed
		return nil, errors.New("certificate decode result is empty")
	}

	return block.Bytes, nil
}

// fixupAddresses checks the list of addresses, removing invalid ones and
// replacing unspecified IPs with the given remote IP.
func fixupAddresses(remote *net.TCPAddr, addresses []string) []string {
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
			if remote != nil {
				// Replace the unspecified IP with the request source.

				// ... unless the request source is the loopback address or
				// multicast/unspecified (can't happen, really).
				if remote.IP == nil || remote.IP.IsLoopback() || remote.IP.IsMulticast() || remote.IP.IsUnspecified() {
					continue
				}

				// Do not use IPv6 remote address if requested scheme is ...4
				// (i.e., tcp4, etc.)
				if strings.HasSuffix(uri.Scheme, "4") && remote.IP.To4() == nil {
					continue
				}

				// Do not use IPv4 remote address if requested scheme is ...6
				if strings.HasSuffix(uri.Scheme, "6") && remote.IP.To4() != nil {
					continue
				}

				host = remote.IP.String()

			} else {
				// remote is nil, unable to determine host IP
				continue
			}
		}

		// If zero port was specified, use remote port.
		if port == "0" {
			if remote != nil && remote.Port > 0 {
				// use remote port
				port = strconv.Itoa(remote.Port)
			} else {
				// unable to determine remote port
				continue
			}
		}

		uri.Host = net.JoinHostPort(host, port)
		fixed = append(fixed, uri.String())
	}

	// Remove duplicate addresses
	fixed = stringutil.UniqueTrimmedStrings(fixed)

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

func notFoundRetryAfterSeconds(misses int) int {
	retryAfterS := notFoundRetryMinSeconds + notFoundRetryIncSeconds*misses
	if retryAfterS > notFoundRetryMaxSeconds {
		retryAfterS = notFoundRetryMaxSeconds
	}
	retryAfterS += rand.Intn(notFoundRetryFuzzSeconds)
	return retryAfterS
}

func reannounceAfterString() string {
	return strconv.Itoa(reannounceAfterSeconds + rand.Intn(reannounzeFuzzSeconds))
}
