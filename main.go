// Copyright (C) 2015 Audrius Butkevicius and Contributors (see the CONTRIBUTORS file).

package main

import (
	"crypto/tls"
	"flag"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	syncthingprotocol "github.com/syncthing/protocol"
	"github.com/syncthing/relaysrv/protocol"
)

var (
	listenProtocol string
	listenSession  string
	debug          bool

	sessionAddress []byte
	sessionPort    uint16

	networkTimeout time.Duration
	pingInterval   time.Duration
	messageTimeout time.Duration

	pingMessage message

	mut    = sync.RWMutex{}
	outbox = make(map[syncthingprotocol.DeviceID]chan message)
)

func main() {
	var dir, extAddress string

	pingPayload := protocol.Ping{}.MustMarshalXDR()
	pingMessage = message{
		header: protocol.Header{
			Magic:         protocol.Magic,
			MessageType:   protocol.MessageTypePing,
			MessageLength: int32(len(pingPayload)),
		},
		payload: pingPayload,
	}

	flag.StringVar(&listenProtocol, "protocol-listen", ":22067", "Protocol listen address")
	flag.StringVar(&listenSession, "session-listen", ":22068", "Session listen address")
	flag.StringVar(&extAddress, "external-address", "", "External address to advertise, defaults no IP and session-listen port, causing clients to use the remote IP from the protocol connection")
	flag.StringVar(&dir, "keys", ".", "Directory where cert.pem and key.pem is stored")
	flag.DurationVar(&networkTimeout, "network-timeout", 2*time.Minute, "Timeout for network operations")
	flag.DurationVar(&pingInterval, "ping-interval", time.Minute, "How often pings are sent")
	flag.DurationVar(&messageTimeout, "message-timeout", time.Minute, "Maximum amount of time we wait for relevant messages to arrive")

	flag.BoolVar(&debug, "debug", false, "Enable debug output")
	flag.Parse()

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

	log.SetOutput(os.Stdout)

	go sessionListener(listenSession)

	protocolListener(listenProtocol, tlsCfg)
}
