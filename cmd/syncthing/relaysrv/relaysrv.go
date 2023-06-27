// Copyright (C) 2015 Audrius Butkevicius and Contributors.

package relaysrv

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/syncthing/syncthing/lib/build"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/relay/protocol"
	"github.com/syncthing/syncthing/lib/tlsutil"
	"golang.org/x/time/rate"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/nat"
	_ "github.com/syncthing/syncthing/lib/pmp"
	_ "github.com/syncthing/syncthing/lib/upnp"

	syncthingprotocol "github.com/syncthing/syncthing/lib/protocol"
)

const defaultPoolAddrs = "https://relays.syncthing.net/endpoint"

var (
	debug bool

	sessionAddress []byte
	sessionPort    uint16

	limitCheckTimer *time.Timer

	overLimit       atomic.Bool
	descriptorLimit int64
	sessionLimiter  *rate.Limiter
	globalLimiter   *rate.Limiter
)

// httpClient is the HTTP client we use for outbound requests. It has a
// timeout and may get further options set during initialization.
var httpClient = &http.Client{
	Timeout: 30 * time.Second,
}

type CLI struct {
	Listen          string        `default:":22067" help:"Protocol listen address"`
	Keys            string        `default:"." help:"Directory where cert.pem and key.pem is stored"`
	NetworkTimeout  time.Duration `default:"2m" help:"Timeout for network operations between the client and the relay. If no data is received between the client and the relay in this period of time, the connection is terminated. Furthermore, if no data is sent between either clients being relayed within this period of time, the session is also terminated."`
	PingInterval    time.Duration `default:"1m" help:"How often pings are sent"`
	MessageTimeout  time.Duration `default:"1m" help:"Maximum amount of time we wait for relevant messages to arrive"`
	SessionLimitBps int           `help:"Per session rate limit, in bytes/s"`
	GlobalLimitBps  int           `help:"Global rate limit, in bytes/s"`
	StatusAddr      string        `default:":22070" help:"Listen address for status service (blank to disable)"`
	Token           string        `help:"Token to restrict access to the relay (optional). Disables joining any pools."`
	Pools           []string      `default:"https://relays.syncthing.net/endpoint" help:"Comma separated list of relay pool addresses to join"`
	ProvidedBy      string        `help:"An optional help about who provides the relay"`
	ExtAddress      string        `help:"An optional address to advertise as being available on. Allows listening on an unprivileged port with port forwarding from e.g. 443, and be connected to on port 443."`
	Protocol        string        `default:"tcp" help:"Protocol used for listening. 'tcp' for IPv4 and IPv6, 'tcp4' for IPv4, 'tcp6' for IPv6"`
	NAT             bool          `default:"false" help:"Use UPnP/NAT-PMP to acquire external port mapping"`
	NATLease        time.Duration `default:"60m" help:"NAT lease length"`
	NATRenewal      time.Duration `default:"30m" help:"NAT renewal frequency"`
	NATTimeout      time.Duration `default:"10s" help:"NAT discovery timeout"`
	Pprof           bool          `default:"false" help:"Enable the built in profiling on the status server"`
	NetworkBuffer   int           `default:"65536" help:"Network buffer size (two of these per proxied connection)"`
	Debug           bool          `help:"Enable debug output"`
	Version         bool          `default:"false" help:"Show version"`
}

func (cli *CLI) Run() error {
	debug = cli.Debug
	log.SetOutput(os.Stdout)
	log.SetFlags(0)

	longVer := build.LongVersionFor("strelaysrv")
	if cli.Version {
		fmt.Println(longVer)
		return nil
	}

	if cli.ExtAddress == "" {
		cli.ExtAddress = cli.Listen
	}

	if len(cli.ProvidedBy) > 30 {
		log.Fatal("Provided-by cannot be longer than 30 characters")
	}

	addr, err := net.ResolveTCPAddr(cli.Protocol, cli.ExtAddress)
	if err != nil {
		log.Fatal(err)
	}

	laddr, err := net.ResolveTCPAddr(cli.Protocol, cli.Listen)
	if err != nil {
		log.Fatal(err)
	}

	if laddr.IP != nil && !laddr.IP.IsUnspecified() {
		// We bind to a specific address. Our outgoing HTTP requests should
		// also come from that address.
		laddr.Port = 0
		boundDialer := &net.Dialer{LocalAddr: laddr}
		httpClient.Transport = &http.Transport{
			DialContext: boundDialer.DialContext,
		}
	}

	log.Println(longVer)

	maxDescriptors, err := osutil.MaximizeOpenFileLimit()
	if maxDescriptors > 0 {
		// Assume that 20% of FD's are leaked/unaccounted for.
		descriptorLimit = int64(maxDescriptors*80) / 100
		log.Println("Connection limit", descriptorLimit)

		go monitorLimits()
	} else if err != nil && !build.IsWindows {
		log.Println("Assuming no connection limit, due to error retrieving rlimits:", err)
	}

	sessionAddress = addr.IP[:]
	sessionPort = uint16(addr.Port)

	certFile, keyFile := filepath.Join(cli.Keys, "cert.pem"), filepath.Join(cli.Keys, "key.pem")
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		log.Println("Failed to load keypair. Generating one, this might take a while...")
		cert, err = tlsutil.NewCertificate(certFile, keyFile, "strelaysrv", 20*365)
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
	if cli.Debug {
		log.Println("ID:", id)
	}

	wrapper := config.Wrap("", config.New(id), id, events.NoopLogger)
	go wrapper.Serve(context.TODO())
	wrapper.Modify(func(cfg *config.Configuration) {
		cfg.Options.NATLeaseM = int(cli.NATLease / time.Minute)
		cfg.Options.NATRenewalM = int(cli.NATRenewal / time.Minute)
		cfg.Options.NATTimeoutS = int(cli.NATTimeout / time.Second)
	})
	natSvc := nat.NewService(id, wrapper)
	mapping := mapping{natSvc.NewMapping(nat.TCP, addr.IP, addr.Port)}

	if cli.NAT {
		ctx, cancel := context.WithCancel(context.Background())
		go natSvc.Serve(ctx)
		defer cancel()
		found := make(chan struct{})
		mapping.OnChanged(func() {
			select {
			case found <- struct{}{}:
			default:
			}
		})

		// Need to wait a few extra seconds, since NAT library waits exactly natTimeout seconds on all interfaces.
		timeout := time.Duration(cli.NATTimeout+2) * time.Second
		log.Printf("Waiting %s to acquire NAT mapping", timeout)

		select {
		case <-found:
			log.Printf("Found NAT mapping: %s", mapping.ExternalAddresses())
		case <-time.After(timeout):
			log.Println("Timeout out waiting for NAT mapping.")
		}
	}

	if cli.SessionLimitBps > 0 {
		sessionLimiter = rate.NewLimiter(rate.Limit(cli.SessionLimitBps), 2*cli.SessionLimitBps)
	}
	if cli.GlobalLimitBps > 0 {
		globalLimiter = rate.NewLimiter(rate.Limit(cli.GlobalLimitBps), 2*cli.GlobalLimitBps)
	}

	if cli.StatusAddr != "" {
		go cli.statusService()
	}

	uri, err := url.Parse(fmt.Sprintf("relay://%s/", mapping.Address()))
	if err != nil {
		return fmt.Errorf("failed to construct URI: %w", err)
	}

	// Add properly encoded query string parameters to URL.
	query := make(url.Values)
	query.Set("id", id.String())
	query.Set("pingInterval", cli.PingInterval.String())
	query.Set("networkTimeout", cli.NetworkTimeout.String())
	if cli.SessionLimitBps > 0 {
		query.Set("sessionLimitBps", fmt.Sprint(cli.SessionLimitBps))
	}
	if cli.GlobalLimitBps > 0 {
		query.Set("globalLimitBps", fmt.Sprint(cli.GlobalLimitBps))
	}
	if cli.StatusAddr != "" {
		query.Set("statusAddr", cli.StatusAddr)
	}
	if cli.ProvidedBy != "" {
		query.Set("providedBy", cli.ProvidedBy)
	}
	uri.RawQuery = query.Encode()

	log.Println("URI:", uri.String())

	if cli.Token != "" {
		cli.Pools = nil
	}

	if len(cli.Pools) == 1 && cli.Pools[0] == defaultPoolAddrs {
		log.Println("!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!")
		log.Println("!!  Joining default relay pools, this relay will be available for public use. !!")
		log.Println(`!!      Use the -pools="" command line option to make the relay private.      !!`)
		log.Println("!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!")
	}

	for _, pool := range cli.Pools {
		pool = strings.TrimSpace(pool)
		if len(pool) > 0 {
			go poolHandler(pool, uri, mapping, cert)
		}
	}

	go listener(cli.Protocol, cli.Listen, tlsCfg, cli.Token, cli.MessageTimeout, cli.NetworkTimeout, cli.PingInterval, cli.NetworkBuffer)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	<-sigs

	// Gracefully close all connections, hoping that clients will be faster
	// to realize that the relay is now gone.

	sessionMut.RLock()
	for _, session := range activeSessions {
		session.CloseConns()
	}

	for _, session := range pendingSessions {
		session.CloseConns()
	}
	sessionMut.RUnlock()

	outboxesMut.RLock()
	for _, outbox := range outboxes {
		close(outbox)
	}
	outboxesMut.RUnlock()

	return nil
}

func monitorLimits() {
	limitCheckTimer = time.NewTimer(time.Minute)
	for range limitCheckTimer.C {
		if numConnections.Load()+numProxies.Load() > descriptorLimit {
			overLimit.Store(true)
			log.Println("Gone past our connection limits. Starting to refuse new/drop idle connections.")
		} else if overLimit.CompareAndSwap(true, false) {
			log.Println("Dropped below our connection limits. Accepting new connections.")
		}
		limitCheckTimer.Reset(time.Minute)
	}
}

type mapping struct {
	*nat.Mapping
}

func (m *mapping) Address() nat.Address {
	ext := m.ExternalAddresses()
	if len(ext) > 0 {
		return ext[0]
	}
	return m.Mapping.Address()
}
