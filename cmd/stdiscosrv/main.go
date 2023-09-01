// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"context"
	"crypto/tls"
	"flag"
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

func main() {
	var listen string
	var dir string
	var metricsListen string
	var replicationListen string
	var replicationPeers string
	var certFile string
	var keyFile string
	var replCertFile string
	var replKeyFile string
	var useHTTP bool
	var largeDB bool

	log.SetOutput(os.Stdout)
	log.SetFlags(0)

	flag.StringVar(&certFile, "cert", "./cert.pem", "Certificate file")
	flag.StringVar(&keyFile, "key", "./key.pem", "Key file")
	flag.StringVar(&dir, "db-dir", "./discovery.db", "Database directory")
	flag.BoolVar(&debug, "debug", false, "Print debug output")
	flag.BoolVar(&useHTTP, "http", false, "Listen on HTTP (behind an HTTPS proxy)")
	flag.StringVar(&listen, "listen", ":8443", "Listen address")
	flag.StringVar(&metricsListen, "metrics-listen", "", "Metrics listen address")
	flag.StringVar(&replicationPeers, "replicate", "", "Replication peers, id@address, comma separated")
	flag.StringVar(&replicationListen, "replication-listen", ":19200", "Replication listen address")
	flag.StringVar(&replCertFile, "replication-cert", "", "Certificate file for replication")
	flag.StringVar(&replKeyFile, "replication-key", "", "Key file for replication")
	flag.BoolVar(&largeDB, "large-db", false, "Use larger database settings")
	showVersion := flag.Bool("version", false, "Show version")
	flag.Parse()

	log.Println(build.LongVersionFor("stdiscosrv"))
	if *showVersion {
		return
	}

	if largeDB {
		levelDBOptions.BlockCacheCapacity = 64 << 20
		levelDBOptions.BlockSize = 64 << 10
		levelDBOptions.CompactionTableSize = 16 << 20
		levelDBOptions.CompactionTableSizeMultiplier = 2.0
		levelDBOptions.WriteBuffer = 64 << 20
		levelDBOptions.CompactionL0Trigger = 8
	}

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if os.IsNotExist(err) {
		log.Println("Failed to load keypair. Generating one, this might take a while...")
		cert, err = tlsutil.NewCertificate(certFile, keyFile, "stdiscosrv", 20*365)
		if err != nil {
			log.Fatalln("Failed to generate X509 key pair:", err)
		}
	} else if err != nil {
		log.Fatalln("Failed to load keypair:", err)
	}
	devID := protocol.NewDeviceID(cert.Certificate[0])
	log.Println("Server device ID is", devID)

	replCert := cert
	if replCertFile != "" && replKeyFile != "" {
		replCert, err = tls.LoadX509KeyPair(replCertFile, replKeyFile)
		if err != nil {
			log.Fatalln("Failed to load replication keypair:", err)
		}
	}
	replDevID := protocol.NewDeviceID(replCert.Certificate[0])
	log.Println("Replication device ID is", replDevID)

	// Parse the replication specs, if any.
	var allowedReplicationPeers []protocol.DeviceID
	var replicationDestinations []string
	parts := strings.Split(replicationPeers, ",")
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
	db, err := newLevelDBStore(dir)
	if err != nil {
		log.Fatalln("Open database:", err)
	}
	main.Add(db)

	// Start any replication senders.
	var repl replicationMultiplexer
	for _, dst := range replicationDestinations {
		rs := newReplicationSender(dst, replCert, allowedReplicationPeers)
		main.Add(rs)
		repl = append(repl, rs)
	}

	// If we have replication configured, start the replication listener.
	if len(allowedReplicationPeers) > 0 {
		rl := newReplicationListener(replicationListen, replCert, allowedReplicationPeers, db)
		main.Add(rl)
	}

	// Start the main API server.
	qs := newAPISrv(listen, cert, db, repl, useHTTP)
	main.Add(qs)

	// If we have a metrics port configured, start a metrics handler.
	if metricsListen != "" {
		go func() {
			mux := http.NewServeMux()
			mux.Handle("/metrics", promhttp.Handler())
			log.Fatal(http.ListenAndServe(metricsListen, mux))
		}()
	}

	// Engage!
	main.Serve(context.Background())
}
