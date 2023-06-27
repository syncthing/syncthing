// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package discosrv

import (
	"context"
	"crypto/tls"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/syncthing/syncthing/lib/build"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/tlsutil"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/thejerf/suture/v4"
)

const (
	addressExpiryTime          = 2 * time.Hour
	databaseStatisticsInterval = 5 * time.Minute

	// Reannounce-After is set to reannounceAfterSeconds +
	// random(reannounzeFuzzSeconds), similar for Retry-After
	reannounceAfterSeconds = 3300
	reannounzeFuzzSeconds  = 300
	errorRetryAfterSeconds = 1500
	errorRetryFuzzSeconds  = 300

	// Retry for not found is minSeconds + failures * incSeconds +
	// random(fuzz), where failures is the number of consecutive lookups
	// with no answer, up to maxSeconds. The fuzz is applied after capping
	// to maxSeconds.
	notFoundRetryMinSeconds  = 60
	notFoundRetryMaxSeconds  = 3540
	notFoundRetryIncSeconds  = 10
	notFoundRetryFuzzSeconds = 60

	// How often (in requests) we serialize the missed counter to database.
	notFoundMissesWriteInterval = 10

	httpReadTimeout    = 5 * time.Second
	httpWriteTimeout   = 5 * time.Second
	httpMaxHeaderBytes = 1 << 10

	// Size of the replication outbox channel
	replicationOutboxSize = 10000
)

// These options make the database a little more optimized for writes, at
// the expense of some memory usage and risk of losing writes in a (system)
// crash.
var levelDBOptions = &opt.Options{
	NoSync:      true,
	WriteBuffer: 32 << 20, // default 4<<20
}

var debug = false

type CLI struct {
	Dir               string `default:"./discovery.db" help:"Database directory"`
	Cert              string `default:"./cert.pem" help:"Certificate file"`
	Key               string `default:"./key.pem" help:"Key file"`
	Listen            string `default:":8443" help:"Listen address"`
	HTTP              bool   `default:"false" help:"Listen on HTTP (behind an HTTPS proxy)"`
	MetricsListen     string `help:"Metrics listen address"`
	Replicate         string `help:"Replication peers, id@address, comma separated"`
	ReplicationListen string `default:":19200" help:"Replication listen address"`
	Debug             bool   `default:"false" help:"Print debug output"`
	Version           bool   `default:"false" help:"Show version"`
}

func (cli *CLI) Run() error {
	log.SetOutput(os.Stdout)
	log.SetFlags(0)

	log.Println(build.LongVersionFor("stdiscosrv"))
	if cli.Version {
		return nil
	}

	cert, err := tls.LoadX509KeyPair(cli.Cert, cli.Key)
	if os.IsNotExist(err) {
		log.Println("Failed to load keypair. Generating one, this might take a while...")
		cert, err = tlsutil.NewCertificate(cli.Cert, cli.Key, "stdiscosrv", 20*365)
		if err != nil {
			log.Fatalln("Failed to generate X509 key pair:", err)
		}
	} else if err != nil {
		log.Fatalln("Failed to load keypair:", err)
	}
	devID := protocol.NewDeviceID(cert.Certificate[0])
	log.Println("Server device ID is", devID)

	// Parse the replication specs, if any.
	var allowedReplicationPeers []protocol.DeviceID
	var replicationDestinations []string
	parts := strings.Split(cli.Replicate, ",")
	for _, part := range parts {
		if part == "" {
			continue
		}

		fields := strings.Split(part, "@")
		switch len(fields) {
		case 2:
			// This is an id@address specification. Grab the address for the
			// destination list. Try to resolve it once to catch obvious
			// syntax errors here rather than having the sender service fail
			// repeatedly later.
			_, err := net.ResolveTCPAddr("tcp", fields[1])
			if err != nil {
				log.Fatalln("Resolving address:", err)
			}
			replicationDestinations = append(replicationDestinations, fields[1])
			fallthrough // N.B.

		case 1:
			// The first part is always a device ID.
			id, err := protocol.DeviceIDFromString(fields[0])
			if err != nil {
				log.Fatalln("Parsing device ID:", err)
			}
			if id == protocol.EmptyDeviceID {
				log.Fatalf("Missing device ID for peer in %q", part)
			}
			allowedReplicationPeers = append(allowedReplicationPeers, id)

		default:
			log.Fatalln("Unrecognized replication spec:", part)
		}
	}

	// Root of the service tree.
	main := suture.New("main", suture.Spec{
		PassThroughPanics: true,
	})

	// Start the database.
	db, err := newLevelDBStore(cli.Dir)
	if err != nil {
		log.Fatalln("Open database:", err)
	}
	main.Add(db)

	// Start any replication senders.
	var repl replicationMultiplexer
	for _, dst := range replicationDestinations {
		rs := newReplicationSender(dst, cert, allowedReplicationPeers)
		main.Add(rs)
		repl = append(repl, rs)
	}

	// If we have replication configured, start the replication listener.
	if len(allowedReplicationPeers) > 0 {
		rl := newReplicationListener(cli.ReplicationListen, cert, allowedReplicationPeers, db)
		main.Add(rl)
	}

	// Start the main API server.
	qs := newAPISrv(cli.Listen, cert, db, repl, cli.HTTP)
	main.Add(qs)

	// If we have a metrics port configured, start a metrics handler.
	if cli.MetricsListen != "" {
		go func() {
			mux := http.NewServeMux()
			mux.Handle("/metrics", promhttp.Handler())
			log.Fatal(http.ListenAndServe(cli.MetricsListen, mux))
		}()
	}

	// Engage!
	return main.Serve(context.Background())
}
