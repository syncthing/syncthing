// Copyright (C) 2015 Audrius Butkevicius and Contributors (see the CONTRIBUTORS file).

package main

import (
	"crypto/tls"
	"flag"
	"log"
	"net"
	"path/filepath"
	"time"

	"github.com/juju/ratelimit"
	"github.com/syncthing/relaysrv/protocol"

	syncthingprotocol "github.com/syncthing/protocol"
)

var (
	listen string
	debug  bool

	sessionAddress []byte
	sessionPort    uint16

	networkTimeout time.Duration
	pingInterval   time.Duration
	messageTimeout time.Duration

	sessionLimitBps int
	globalLimitBps  int
	sessionLimiter  *ratelimit.Bucket
	globalLimiter   *ratelimit.Bucket

	statusAddr string
)

func main() {
	var dir, extAddress string

	flag.StringVar(&listen, "listen", ":22067", "Protocol listen address")
	flag.StringVar(&dir, "keys", ".", "Directory where cert.pem and key.pem is stored")
	flag.DurationVar(&networkTimeout, "network-timeout", 2*time.Minute, "Timeout for network operations")
	flag.DurationVar(&pingInterval, "ping-interval", time.Minute, "How often pings are sent")
	flag.DurationVar(&messageTimeout, "message-timeout", time.Minute, "Maximum amount of time we wait for relevant messages to arrive")
	flag.IntVar(&sessionLimitBps, "per-session-rate", sessionLimitBps, "Per session rate limit, in bytes/s")
	flag.IntVar(&globalLimitBps, "global-rate", globalLimitBps, "Global rate limit, in bytes/s")
	flag.BoolVar(&debug, "debug", false, "Enable debug output")
	flag.StringVar(&statusAddr, "status-srv", ":22070", "Listen address for status service (blank to disable)")

	flag.Parse()

	if extAddress == "" {
		extAddress = listen
	}

	addr, err := net.ResolveTCPAddr("tcp", extAddress)
	if err != nil {
		log.Fatal(err)
	}

	sessionAddress = addr.IP[:]
	sessionPort = uint16(addr.Port)

	certFile, keyFile := filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		log.Fatalln("Failed to load X509 key pair:", err)
	}

	tlsCfg := &tls.Config{
		Certificates:           []tls.Certificate{cert},
		NextProtos:             []string{protocol.ProtocolName},
		ClientAuth:             tls.RequestClientCert,
		SessionTicketsDisabled: true,
		InsecureSkipVerify:     true,
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

	id := syncthingprotocol.NewDeviceID(cert.Certificate[0])
	if debug {
		log.Println("ID:", id)
	}

	if sessionLimitBps > 0 {
		sessionLimiter = ratelimit.NewBucketWithRate(float64(sessionLimitBps), int64(2*sessionLimitBps))
	}
	if globalLimitBps > 0 {
		globalLimiter = ratelimit.NewBucketWithRate(float64(globalLimitBps), int64(2*globalLimitBps))
	}

	if statusAddr != "" {
		go statusService(statusAddr)
	}

	listener(listen, tlsCfg)
}
