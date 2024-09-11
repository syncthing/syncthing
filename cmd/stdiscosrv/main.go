// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"context"
	"crypto/tls"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"time"

	_ "net/http/pprof"

	"github.com/alecthomas/kong"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	_ "github.com/syncthing/syncthing/lib/automaxprocs"
	"github.com/syncthing/syncthing/lib/build"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/rand"
	"github.com/syncthing/syncthing/lib/tlsutil"
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

	// Retry for not found is notFoundRetrySeenSeconds for records we have
	// seen an announcement for (but it's not active right now) and
	// notFoundRetryUnknownSeconds for records we have never seen (or not
	// seen within the last week).
	notFoundRetryUnknownMinSeconds = 60
	notFoundRetryUnknownMaxSeconds = 3600

	httpReadTimeout    = 5 * time.Second
	httpWriteTimeout   = 5 * time.Second
	httpMaxHeaderBytes = 1 << 10

	// Size of the replication outbox channel
	replicationOutboxSize = 10000
)

var debug = false

type CLI struct {
	Cert          string `group:"Listen" help:"Certificate file" default:"./cert.pem" env:"DISCOVERY_CERT_FILE"`
	Key           string `group:"Listen" help:"Key file" default:"./key.pem" env:"DISCOVERY_KEY_FILE"`
	HTTP          bool   `group:"Listen" help:"Listen on HTTP (behind an HTTPS proxy)" env:"DISCOVERY_HTTP"`
	Compression   bool   `group:"Listen" help:"Enable GZIP compression of responses" env:"DISCOVERY_COMPRESSION"`
	Listen        string `group:"Listen" help:"Listen address" default:":8443" env:"DISCOVERY_LISTEN"`
	MetricsListen string `group:"Listen" help:"Metrics listen address" env:"DISCOVERY_METRICS_LISTEN"`

	Replicate         []string `group:"Legacy replication" help:"Replication peers, id@address, comma separated" env:"DISCOVERY_REPLICATE"`
	ReplicationListen string   `group:"Legacy replication" help:"Replication listen address" default:":19200" env:"DISCOVERY_REPLICATION_LISTEN"`
	ReplicationCert   string   `group:"Legacy replication" help:"Certificate file for replication" env:"DISCOVERY_REPLICATION_CERT_FILE"`
	ReplicationKey    string   `group:"Legacy replication" help:"Key file for replication" env:"DISCOVERY_REPLICATION_KEY_FILE"`

	AMQPAddress string `group:"AMQP replication" help:"Address to AMQP broker" env:"DISCOVERY_AMQP_ADDRESS"`

	DBDir           string        `group:"Database" help:"Database directory" default:"." env:"DISCOVERY_DB_DIR"`
	DBFlushInterval time.Duration `group:"Database" help:"Interval between database flushes" default:"5m" env:"DISCOVERY_DB_FLUSH_INTERVAL"`

	DBS3Endpoint    string `name:"db-s3-endpoint" group:"Database (S3 backup)" help:"S3 endpoint for database" env:"DISCOVERY_DB_S3_ENDPOINT"`
	DBS3Region      string `name:"db-s3-region" group:"Database (S3 backup)" help:"S3 region for database" env:"DISCOVERY_DB_S3_REGION"`
	DBS3Bucket      string `name:"db-s3-bucket" group:"Database (S3 backup)" help:"S3 bucket for database" env:"DISCOVERY_DB_S3_BUCKET"`
	DBS3AccessKeyID string `name:"db-s3-access-key-id" group:"Database (S3 backup)" help:"S3 access key ID for database" env:"DISCOVERY_DB_S3_ACCESS_KEY_ID"`
	DBS3SecretKey   string `name:"db-s3-secret-key" group:"Database (S3 backup)" help:"S3 secret key for database" env:"DISCOVERY_DB_S3_SECRET_KEY"`

	Debug   bool `short:"d" help:"Print debug output" env:"DISCOVERY_DEBUG"`
	Version bool `short:"v" help:"Print version and exit"`
}

func main() {
	log.SetOutput(os.Stdout)

	var cli CLI
	kong.Parse(&cli)
	debug = cli.Debug

	log.Println(build.LongVersionFor("stdiscosrv"))
	if cli.Version {
		return
	}

	buildInfo.WithLabelValues(build.Version, runtime.Version(), build.User, build.Date.UTC().Format("2006-01-02T15:04:05Z")).Set(1)

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

	replCert := cert
	if cli.ReplicationCert != "" && cli.ReplicationKey != "" {
		replCert, err = tls.LoadX509KeyPair(cli.ReplicationCert, cli.ReplicationKey)
		if err != nil {
			log.Fatalln("Failed to load replication keypair:", err)
		}
	}
	replDevID := protocol.NewDeviceID(replCert.Certificate[0])
	log.Println("Replication device ID is", replDevID)

	// Parse the replication specs, if any.
	var allowedReplicationPeers []protocol.DeviceID
	var replicationDestinations []string
	for _, part := range cli.Replicate {
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
		Timeout:           2 * time.Minute,
	})

	// If configured, use S3 for database backups.
	var s3c *s3Copier
	if cli.DBS3Endpoint != "" {
		hostname, err := os.Hostname()
		if err != nil {
			log.Fatalf("Failed to get hostname: %v", err)
		}
		key := hostname + ".db"
		s3c = newS3Copier(cli.DBS3Endpoint, cli.DBS3Region, cli.DBS3Bucket, key, cli.DBS3AccessKeyID, cli.DBS3SecretKey)
	}

	// Start the database.
	db := newInMemoryStore(cli.DBDir, cli.DBFlushInterval, s3c)
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
		rl := newReplicationListener(cli.ReplicationListen, replCert, allowedReplicationPeers, db)
		main.Add(rl)
	}

	// If we have an AMQP broker, start that
	if cli.AMQPAddress != "" {
		clientID := rand.String(10)
		kr := newAMQPReplicator(cli.AMQPAddress, clientID, db)
		repl = append(repl, kr)
		main.Add(kr)
	}

	// Start the main API server.
	qs := newAPISrv(cli.Listen, cert, db, repl, cli.HTTP, cli.Compression)
	main.Add(qs)

	// If we have a metrics port configured, start a metrics handler.
	if cli.MetricsListen != "" {
		go func() {
			mux := http.NewServeMux()
			mux.Handle("/metrics", promhttp.Handler())
			log.Fatal(http.ListenAndServe(cli.MetricsListen, mux))
		}()
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Cancel on signal
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)
	go func() {
		sig := <-signalChan
		log.Printf("Received signal %s; shutting down", sig)
		cancel()
	}()

	// Engage!
	main.Serve(ctx)
}
