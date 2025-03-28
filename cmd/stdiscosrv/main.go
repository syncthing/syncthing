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
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"time"

	"github.com/alecthomas/kong"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/thejerf/suture/v4"

	_ "github.com/syncthing/syncthing/lib/automaxprocs"
	"github.com/syncthing/syncthing/lib/build"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/rand"
	"github.com/syncthing/syncthing/lib/s3"
	"github.com/syncthing/syncthing/lib/tlsutil"
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

	DBDir           string        `group:"Database" help:"Database directory" default:"." env:"DISCOVERY_DB_DIR"`
	DBFlushInterval time.Duration `group:"Database" help:"Interval between database flushes" default:"5m" env:"DISCOVERY_DB_FLUSH_INTERVAL"`

	DBS3Endpoint    string `name:"db-s3-endpoint" group:"Database (S3 backup)" hidden:"true" help:"S3 endpoint for database" env:"DISCOVERY_DB_S3_ENDPOINT"`
	DBS3Region      string `name:"db-s3-region" group:"Database (S3 backup)" hidden:"true" help:"S3 region for database" env:"DISCOVERY_DB_S3_REGION"`
	DBS3Bucket      string `name:"db-s3-bucket" group:"Database (S3 backup)" hidden:"true" help:"S3 bucket for database" env:"DISCOVERY_DB_S3_BUCKET"`
	DBS3AccessKeyID string `name:"db-s3-access-key-id" group:"Database (S3 backup)" hidden:"true" help:"S3 access key ID for database" env:"DISCOVERY_DB_S3_ACCESS_KEY_ID"`
	DBS3SecretKey   string `name:"db-s3-secret-key" group:"Database (S3 backup)" hidden:"true" help:"S3 secret key for database" env:"DISCOVERY_DB_S3_SECRET_KEY"`

	AMQPAddress string `group:"AMQP replication" hidden:"true" help:"Address to AMQP broker" env:"DISCOVERY_AMQP_ADDRESS"`

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

	var cert tls.Certificate
	if !cli.HTTP {
		var err error
		cert, err = tls.LoadX509KeyPair(cli.Cert, cli.Key)
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
	}

	// Root of the service tree.
	main := suture.New("main", suture.Spec{
		PassThroughPanics: true,
		Timeout:           2 * time.Minute,
	})

	// If configured, use S3 for database backups.
	var s3c *s3.Session
	if cli.DBS3Endpoint != "" {
		var err error
		s3c, err = s3.NewSession(cli.DBS3Endpoint, cli.DBS3Region, cli.DBS3Bucket, cli.DBS3AccessKeyID, cli.DBS3SecretKey)
		if err != nil {
			log.Fatalf("Failed to create S3 session: %v", err)
		}
	}

	// Start the database.
	db := newInMemoryStore(cli.DBDir, cli.DBFlushInterval, s3c)
	main.Add(db)

	// If we have an AMQP broker for replication, start that
	var repl replicator
	if cli.AMQPAddress != "" {
		clientID := rand.String(10)
		kr := newAMQPReplicator(cli.AMQPAddress, clientID, db)
		main.Add(kr)
		repl = kr
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
