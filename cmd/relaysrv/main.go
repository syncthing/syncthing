// Copyright (C) 2015 Audrius Butkevicius and Contributors (see the CONTRIBUTORS file).

package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/juju/ratelimit"
	"github.com/syncthing/syncthing/lib/relay/protocol"
	"github.com/syncthing/syncthing/lib/tlsutil"

	syncthingprotocol "github.com/syncthing/syncthing/lib/protocol"
)

var (
	listen string
	debug  bool = false

	sessionAddress []byte
	sessionPort    uint16

	networkTimeout time.Duration = 2 * time.Minute
	pingInterval   time.Duration = time.Minute
	messageTimeout time.Duration = time.Minute

	sessionLimitBps int
	globalLimitBps  int
	sessionLimiter  *ratelimit.Bucket
	globalLimiter   *ratelimit.Bucket

	statusAddr       string
	poolAddrs        string
	defaultPoolAddrs string = "https://relays.syncthing.net"
)

func main() {
	log.SetFlags(log.Lshortfile | log.LstdFlags)

	var dir, extAddress string

	flag.StringVar(&listen, "listen", ":22067", "Protocol listen address")
	flag.StringVar(&dir, "keys", ".", "Directory where cert.pem and key.pem is stored")
	flag.DurationVar(&networkTimeout, "network-timeout", networkTimeout, "Timeout for network operations between the client and the relay.\n\tIf no data is received between the client and the relay in this period of time, the connection is terminated.\n\tFurthermore, if no data is sent between either clients being relayed within this period of time, the session is also terminated.")
	flag.DurationVar(&pingInterval, "ping-interval", pingInterval, "How often pings are sent")
	flag.DurationVar(&messageTimeout, "message-timeout", messageTimeout, "Maximum amount of time we wait for relevant messages to arrive")
	flag.IntVar(&sessionLimitBps, "per-session-rate", sessionLimitBps, "Per session rate limit, in bytes/s")
	flag.IntVar(&globalLimitBps, "global-rate", globalLimitBps, "Global rate limit, in bytes/s")
	flag.BoolVar(&debug, "debug", debug, "Enable debug output")
	flag.StringVar(&statusAddr, "status-srv", ":22070", "Listen address for status service (blank to disable)")
	flag.StringVar(&poolAddrs, "pools", defaultPoolAddrs, "Comma separated list of relay pool addresses to join")

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
		log.Println("Failed to load keypair. Generating one, this might take a while...")
		cert, err = tlsutil.NewCertificate(certFile, keyFile, "relaysrv", 3072)
		if err != nil {
			log.Fatalln("Failed to generate X509 key pair:", err)
		}
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

	uri, err := url.Parse(fmt.Sprintf("relay://%s/?id=%s&pingInterval=%s&networkTimeout=%s&sessionLimitBps=%d&globalLimitBps=%d&statusAddr=%s", extAddress, id, pingInterval, networkTimeout, sessionLimitBps, globalLimitBps, statusAddr))
	if err != nil {
		log.Fatalln("Failed to construct URI", err)
	}

	log.Println("URI:", uri.String())

	if poolAddrs == defaultPoolAddrs {
		log.Println("!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!")
		log.Println("!!  Joining default relay pools, this relay will be available for public use. !!")
		log.Println(`!!      Use the -pools="" command line option to make the relay private.      !!`)
		log.Println("!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!")
	}

	pools := strings.Split(poolAddrs, ",")
	for _, pool := range pools {
		pool = strings.TrimSpace(pool)
		if len(pool) > 0 {
			go poolHandler(pool, uri)
		}
	}

	listener(listen, tlsCfg)
}
