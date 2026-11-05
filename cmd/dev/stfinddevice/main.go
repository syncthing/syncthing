// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/alecthomas/kong"
	"github.com/syncthing/syncthing/internal/dht"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/discover"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/protocol"
)

type CLI struct {
	Server       string        `help:"Announce server (blank for default set)"`
	DHTBootstrap []string      `help:"DHT bootstrap multiaddress (blank for default set)"`
	Timeout      time.Duration `help:"Query timeout" default:"5s"`
	DeviceID     string        `arg:"" help:"Device ID to look up"`
}

func main() {
	var cli CLI
	kong.Parse(&cli)

	id, err := protocol.DeviceIDFromString(cli.DeviceID)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	servers := config.DefaultDiscoveryServers
	if cli.Server != "" {
		servers = []string{cli.Server}
	}
	bootstrapPeers := cli.DHTBootstrap
	if len(bootstrapPeers) == 0 {
		bootstrapPeers = config.DefaultP2PDiscoveryBootstrapPeers
	}
	checkServers(id, cli.Timeout, bootstrapPeers, servers...)
}

type checkResult struct {
	server    string
	addresses []string
	error
}

func checkServers(deviceID protocol.DeviceID, timeout time.Duration, bootstrapPeers []string, servers ...string) {
	t0 := time.Now()
	resc := make(chan checkResult)
	var wg sync.WaitGroup

	for _, srv := range servers {
		wg.Go(func() {
			res := checkServer(deviceID, srv, timeout)
			res.server = srv
			resc <- res
		})
	}

	wg.Go(func() {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		addresses, err := dht.Lookup(ctx, deviceID, dht.Options{
			BootstrapPeers: bootstrapPeers,
		})
		resc <- checkResult{server: "DHT", addresses: addresses, error: err}
	})

	go func() {
		wg.Wait()
		close(resc)
	}()

	for res := range resc {
		name := res.server
		if u, err := url.Parse(name); err == nil && u.Host != "" {
			name = u.Host
		}
		fmt.Printf("%s (%v):\n", name, time.Since(t0))

		if res.error != nil {
			fmt.Println("  " + res.error.Error())
		}
		for _, addr := range res.addresses {
			fmt.Println("  address:", addr)
		}
	}
}

func checkServer(deviceID protocol.DeviceID, server string, timeout time.Duration) checkResult {
	disco, err := discover.NewGlobal(server, tls.Certificate{}, nil, events.NoopLogger, nil)
	if err != nil {
		return checkResult{error: err}
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	addresses, err := disco.Lookup(ctx, deviceID)
	return checkResult{addresses: addresses, error: err}
}
