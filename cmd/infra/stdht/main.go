// Copyright (C) 2026 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/alecthomas/kong"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/p2p/transport/quicreuse"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/syncthing/syncthing/internal/db"
	"github.com/syncthing/syncthing/internal/db/sqlite"
	"github.com/syncthing/syncthing/internal/dht"
	"github.com/syncthing/syncthing/lib/build"
	"github.com/syncthing/syncthing/lib/config"
)

type CLI struct {
	Listen         []string `help:"QUIC listen multiaddresses" default:"/ip4/0.0.0.0/udp/22026/quic-v1,/ip6/::/udp/22026/quic-v1" env:"STDHT_LISTEN"`
	Advertise      []string `help:"Advertised multiaddresses (default: detected listener addresses)" env:"STDHT_ADVERTISE"`
	Bootstrap      []string `help:"Bootstrap multiaddresses including /p2p/peer-ID; empty uses Syncthing defaults" env:"STDHT_BOOTSTRAP"`
	KeyFile        string   `help:"Base64-encoded libp2p private key file" default:"identity.key" env:"STDHT_KEY_FILE"`
	Database       string   `help:"Database directory for DHT values and routing table" default:"stdht.db" env:"STDHT_DATABASE"`
	MetricsListen  string   `help:"HTTP metrics and health address; empty disables" default:"127.0.0.1:9010" env:"STDHT_METRICS_LISTEN"`
	MaxConnections int      `help:"Maximum DHT connections" default:"16000" env:"STDHT_MAX_CONNECTIONS"`
	Version        bool     `short:"v" help:"Print version and exit"`
}

func main() {
	var cli CLI
	kong.Parse(&cli)
	if cli.Version {
		fmt.Println(build.LongVersionFor("stdht"))
		return
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, cli); err != nil {
		slog.Error("DHT server failed", "error", err)
		os.Exit(1)
	}
}

func loadIdentity(path string) (crypto.PrivKey, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		s := strings.TrimSpace(string(data))
		decoded, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			return nil, fmt.Errorf("decode identity key: %w", err)
		}
		return crypto.UnmarshalPrivateKey(decoded)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	priv, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		return nil, err
	}
	data, err = crypto.MarshalPrivateKey(priv)
	if err != nil {
		return nil, err
	}

	enc := base64.StdEncoding.EncodeToString(data) + "\n"
	if err := os.WriteFile(path, []byte(enc), 0o600); err != nil {
		return nil, err
	}

	return priv, nil
}

func run(ctx context.Context, cli CLI) error {
	identity, err := loadIdentity(cli.KeyFile)
	if err != nil {
		return err
	}
	if len(cli.Bootstrap) == 0 {
		cli.Bootstrap = config.DefaultDHTBootstrapPeers
	}
	store, err := sqlite.Open(cli.Database)
	if err != nil {
		return fmt.Errorf("open DHT database: %w", err)
	}
	defer store.Close()

	var listen []netip.AddrPort
	for _, address := range cli.Listen {
		addr, err := ma.NewMultiaddr(address)
		if err != nil {
			return err
		}
		udpAddr, _, err := quicreuse.FromQuicMultiaddr(addr)
		if err != nil {
			return err
		}
		listen = append(listen, udpAddr.AddrPort())
	}
	dht, err := dht.NewSyncthingDHT(dht.Options{
		Identity:           identity,
		ListenAddresses:    listen,
		AdvertiseAddresses: cli.Advertise,
		BootstrapPeers:     cli.Bootstrap,
		Server:             true,
		MaxConnections:     cli.MaxConnections,
		ValueDatastore:     dht.NewDatastore(db.NewTyped(store, "dht-values")),
		RoutingDatastore:   dht.NewDatastore(db.NewTyped(store, "dht-routing")),
	})
	if err != nil {
		return err
	}
	defer dht.Close()

	slog.Info("Discovery DHT server started", "peer", dht.ID())

	var httpServer *http.Server
	var httpErrors <-chan error
	if cli.MetricsListen != "" {
		listener, err := net.Listen("tcp", cli.MetricsListen)
		if err != nil {
			return err
		}

		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())
		mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})

		httpServer = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second, WriteTimeout: 10 * time.Second}
		results := make(chan error, 1)
		httpErrors = results
		go func() { results <- httpServer.Serve(listener) }()
		defer httpServer.Close()
	}

	select {
	case <-ctx.Done():
		if httpServer != nil {
			shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			return httpServer.Shutdown(shutdown)
		}
		return nil
	case err := <-httpErrors:
		return err
	}
}
