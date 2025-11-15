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
	"log/slog"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/syncthing/syncthing/internal/gen/discosrv"
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
	compression    bool
	gzipWriters    sync.Pool
	seenTracker    *retryAfterTracker
	notSeenTracker *retryAfterTracker
}

type replicator interface {
	send(key *protocol.DeviceID, addrs []*discosrv.DatabaseAddress, seen int64)
}

type requestID int64

func (i requestID) String() string {
	return fmt.Sprintf("%016x", int64(i))
}

type contextKey int

const idKey contextKey = iota

func newAPISrv(addr string, cert tls.Certificate, db database, repl replicator, useHTTP, compression bool, desiredNotFoundRate float64) *apiSrv {
	return &apiSrv{
		addr:        addr,
		cert:        cert,
		db:          db,
		repl:        repl,
		useHTTP:     useHTTP,
		compression: compression,
		seenTracker: &retryAfterTracker{
			name:         "seenTracker",
			bucketStarts: time.Now(),
			desiredRate:  desiredNotFoundRate / 2,
			currentDelay: notFoundRetryUnknownMinSeconds,
		},
		notSeenTracker: &retryAfterTracker{
			name:         "notSeenTracker",
			bucketStarts: time.Now(),
			desiredRate:  desiredNotFoundRate / 2,
			currentDelay: notFoundRetryUnknownMaxSeconds / 2,
		},
	}
}

func (s *apiSrv) Serve(ctx context.Context) error {
	if s.useHTTP {
		listener, err := net.Listen("tcp", s.addr)
		if err != nil {
			slog.ErrorContext(ctx, "Failed to listen", "error", err)
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
			slog.ErrorContext(ctx, "Failed to listen", "error", err)
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
	}
	if !debug {
		srv.ErrorLog = log.New(io.Discard, "", 0)
	}

	go func() {
		<-ctx.Done()
		srv.Shutdown(context.Background())
	}()

	err := srv.Serve(s.listener)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to serve", "error", err)
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

	slog.Debug("Handling request", "id", reqID, "method", req.Method, "url", req.URL, "proto", req.Proto)

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
			slog.Warn("Failed to resolve remote address", "address", req.RemoteAddr, "error", err)
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
		slog.Debug("Request with bad device param", "id", reqID, "error", err)
		lookupRequestsTotal.WithLabelValues("bad_request").Inc()
		w.Header().Set("Retry-After", errorRetryAfterString())
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	rec, err := s.db.get(&deviceID)
	if err != nil {
		// some sort of internal error
		lookupRequestsTotal.WithLabelValues("internal_error").Inc()
		w.Header().Set("Retry-After", errorRetryAfterString())
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if len(rec.Addresses) == 0 {
		var afterS int
		if rec.Seen == 0 {
			afterS = s.notSeenTracker.retryAfterS()
			lookupRequestsTotal.WithLabelValues("not_found_ever").Inc()
		} else {
			afterS = s.seenTracker.retryAfterS()
			lookupRequestsTotal.WithLabelValues("not_found_recent").Inc()
		}
		w.Header().Set("Retry-After", strconv.Itoa(afterS))
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	lookupRequestsTotal.WithLabelValues("success").Inc()

	w.Header().Set("Content-Type", "application/json")
	var bw io.Writer = w

	// Use compression if the client asks for it
	if s.compression && strings.Contains(req.Header.Get("Accept-Encoding"), "gzip") {
		gw, ok := s.gzipWriters.Get().(*gzip.Writer)
		if ok {
			gw.Reset(w)
		} else {
			gw = gzip.NewWriter(w)
		}
		w.Header().Set("Content-Encoding", "gzip")
		defer gw.Close()
		defer s.gzipWriters.Put(gw)
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
		slog.Debug("Request without certificates", "id", reqID, "error", err)
		announceRequestsTotal.WithLabelValues("no_certificate").Inc()
		w.Header().Set("Retry-After", errorRetryAfterString())
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	var ann announcement
	if err := json.NewDecoder(req.Body).Decode(&ann); err != nil {
		slog.Debug("Failed to decode request", "id", reqID, "error", err)
		announceRequestsTotal.WithLabelValues("bad_request").Inc()
		w.Header().Set("Retry-After", errorRetryAfterString())
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	deviceID := protocol.NewDeviceID(rawCert)

	addresses := fixupAddresses(remoteAddr, ann.Addresses)
	if len(addresses) == 0 {
		slog.Debug("Request without addresses", "id", reqID, "error", err)
		announceRequestsTotal.WithLabelValues("bad_request").Inc()
		w.Header().Set("Retry-After", errorRetryAfterString())
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	if err := s.handleAnnounce(deviceID, addresses); err != nil {
		slog.Debug("Failed to handle request", "id", reqID, "error", err)
		announceRequestsTotal.WithLabelValues("internal_error").Inc()
		w.Header().Set("Retry-After", errorRetryAfterString())
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	announceRequestsTotal.WithLabelValues("success").Inc()

	w.Header().Set("Reannounce-After", reannounceAfterString())
	w.WriteHeader(http.StatusNoContent)
	slog.Debug("Device announced", "id", reqID, "device", deviceID, "addresses", addresses)
}

func (s *apiSrv) Stop() {
	s.listener.Close()
}

func (s *apiSrv) handleAnnounce(deviceID protocol.DeviceID, addresses []string) error {
	now := time.Now()
	expire := now.Add(addressExpiryTime).UnixNano()

	// The address slice must always be sorted for database merges to work
	// properly.
	slices.Sort(addresses)
	addresses = slices.Compact(addresses)

	dbAddrs := make([]*discosrv.DatabaseAddress, len(addresses))
	for i := range addresses {
		dbAddrs[i] = &discosrv.DatabaseAddress{
			Address: addresses[i],
			Expires: expire,
		}
	}

	seen := now.UnixNano()
	if s.repl != nil {
		s.repl.send(&deviceID, dbAddrs, seen)
	}
	return s.db.merge(&deviceID, dbAddrs, seen)
}

func handlePing(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

func certificateBytes(req *http.Request) ([]byte, error) {
	if req.TLS != nil && len(req.TLS.PeerCertificates) > 0 {
		return req.TLS.PeerCertificates[0].Raw, nil
	}

	var bs []byte

	if hdr := req.Header.Get("X-Ssl-Cert"); hdr != "" {
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
	} else if cert := req.Header.Get("X-Forwarded-Tls-Client-Cert"); cert != "" {
		// Traefik 2 passtlsclientcert
		//
		// The certificate is in PEM format, maybe with URL encoding
		// (depends on Traefik version) but without newlines and start/end
		// statements. We need to decode, reinstate the newlines every 64
		// character and add statements for the PEM decoder

		if strings.Contains(cert, "%") {
			if unesc, err := url.QueryUnescape(cert); err == nil {
				cert = unesc
			}
		}

		const (
			header = "-----BEGIN CERTIFICATE-----"
			footer = "-----END CERTIFICATE-----"
		)

		var b bytes.Buffer
		b.Grow(len(header) + 1 + len(cert) + len(cert)/64 + 1 + len(footer) + 1)

		b.WriteString(header)
		b.WriteByte('\n')

		for i := 0; i < len(cert); i += 64 {
			end := i + 64
			if end > len(cert) {
				end = len(cert)
			}
			b.WriteString(cert[i:end])
			b.WriteByte('\n')
		}

		b.WriteString(footer)
		b.WriteByte('\n')

		bs = b.Bytes()
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

func addressStrs(dbAddrs []*discosrv.DatabaseAddress) []string {
	res := make([]string, len(dbAddrs))
	for i, a := range dbAddrs {
		res[i] = a.Address
	}
	return res
}

func errorRetryAfterString() string {
	return strconv.Itoa(errorRetryAfterSeconds + rand.Intn(errorRetryFuzzSeconds))
}

func reannounceAfterString() string {
	return strconv.Itoa(reannounceAfterSeconds + rand.Intn(reannounzeFuzzSeconds))
}

type retryAfterTracker struct {
	name        string
	desiredRate float64 // requests per second

	mut          sync.Mutex
	lastCount    int       // requests in the last bucket
	curCount     int       // requests in the current bucket
	bucketStarts time.Time // start of the current bucket
	currentDelay int       // current delay in seconds
}

func (t *retryAfterTracker) retryAfterS() int {
	now := time.Now()
	t.mut.Lock()
	if durS := now.Sub(t.bucketStarts).Seconds(); durS > float64(t.currentDelay) {
		t.bucketStarts = now
		t.lastCount = t.curCount
		lastRate := float64(t.lastCount) / durS

		switch {
		case t.currentDelay > notFoundRetryUnknownMinSeconds &&
			lastRate < 0.75*t.desiredRate:
			t.currentDelay = max(8*t.currentDelay/10, notFoundRetryUnknownMinSeconds)
		case t.currentDelay < notFoundRetryUnknownMaxSeconds &&
			lastRate > 1.25*t.desiredRate:
			t.currentDelay = min(3*t.currentDelay/2, notFoundRetryUnknownMaxSeconds)
		}

		t.curCount = 0
	}
	if t.curCount == 0 {
		retryAfterLevel.WithLabelValues(t.name).Set(float64(t.currentDelay))
	}
	t.curCount++
	t.mut.Unlock()
	return t.currentDelay + rand.Intn(t.currentDelay/4)
}
