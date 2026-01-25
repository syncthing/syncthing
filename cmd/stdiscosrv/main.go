// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"time"

	"github.com/alecthomas/kong"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/thejerf/suture/v4"

	"github.com/syncthing/syncthing/internal/blob"
	"github.com/syncthing/syncthing/internal/blob/azureblob"
	"github.com/syncthing/syncthing/internal/blob/s3"
	"github.com/syncthing/syncthing/internal/slogutil"
	_ "github.com/syncthing/syncthing/lib/automaxprocs"
	"github.com/syncthing/syncthing/lib/build"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/rand"
	"github.com/syncthing/syncthing/lib/tlsutil"
)

const (
	addressExpiryTime = 2 * time.Hour

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
	Cert                string  `group:"Listen" help:"Certificate file" default:"./cert.pem" env:"DISCOVERY_CERT_FILE"`
	Key                 string  `group:"Listen" help:"Key file" default:"./key.pem" env:"DISCOVERY_KEY_FILE"`
	HTTP                bool    `group:"Listen" help:"Listen on HTTP (behind an HTTPS proxy)" env:"DISCOVERY_HTTP"`
	Compression         bool    `group:"Listen" help:"Enable GZIP compression of responses" env:"DISCOVERY_COMPRESSION"`
	Listen              string  `group:"Listen" help:"Listen address" default:":8443" env:"DISCOVERY_LISTEN"`
	MetricsListen       string  `group:"Listen" help:"Metrics listen address" env:"DISCOVERY_METRICS_LISTEN"`
	DesiredNotFoundRate float64 `group:"Listen" help:"Desired maximum rate of not-found replies (/s)" default:"1000"`

	DBDir           string        `group:"Database" help:"Database directory" default:"." env:"DISCOVERY_DB_DIR"`
	DBFlushInterval time.Duration `group:"Database" help:"Interval between database flushes" default:"5m" env:"DISCOVERY_DB_FLUSH_INTERVAL"`

	DBS3Endpoint    string `name:"db-s3-endpoint" group:"Database (S3 backup)" hidden:"true" help:"S3 endpoint for database" env:"DISCOVERY_DB_S3_ENDPOINT"`
	DBS3Region      string `name:"db-s3-region" group:"Database (S3 backup)" hidden:"true" help:"S3 region for database" env:"DISCOVERY_DB_S3_REGION"`
	DBS3Bucket      string `name:"db-s3-bucket" group:"Database (S3 backup)" hidden:"true" help:"S3 bucket for database" env:"DISCOVERY_DB_S3_BUCKET"`
	DBS3AccessKeyID string `name:"db-s3-access-key-id" group:"Database (S3 backup)" hidden:"true" help:"S3 access key ID for database" env:"DISCOVERY_DB_S3_ACCESS_KEY_ID"`
	DBS3SecretKey   string `name:"db-s3-secret-key" group:"Database (S3 backup)" hidden:"true" help:"S3 secret key for database" env:"DISCOVERY_DB_S3_SECRET_KEY"`

	DBAzureBlobAccount   string `name:"db-azure-blob-account" env:"DISCOVERY_DB_AZUREBLOB_ACCOUNT"`
	DBAzureBlobKey       string `name:"db-azure-blob-key" env:"DISCOVERY_DB_AZUREBLOB_KEY"`
	DBAzureBlobContainer string `name:"db-azure-blob-container" env:"DISCOVERY_DB_AZUREBLOB_CONTAINER"`

	AMQPAddress string `group:"AMQP replication" hidden:"true" help:"Address to AMQP broker" env:"DISCOVERY_AMQP_ADDRESS"`

	Debug   bool `short:"d" help:"Print debug output" env:"DISCOVERY_DEBUG"`
	Version bool `short:"v" help:"Print version and exit"`
}

func main() {
	var cli CLI
	kong.Parse(&cli)

	level := slog.LevelInfo
	if cli.Debug {
		level = slog.LevelDebug
	}
	slogutil.SetDefaultLevel(level)
	if cli.Version {
		fmt.Println(build.LongVersionFor("stdiscosrv"))
		return
	}
	slog.Info(build.LongVersionFor("stdiscosrv"))

	buildInfo.WithLabelValues(build.Version, runtime.Version(), build.User, build.Date.UTC().Format("2006-01-02T15:04:05Z")).Set(1)

	var cert tls.Certificate
	if !cli.HTTP {
		var err error
		cert, err = tls.LoadX509KeyPair(cli.Cert, cli.Key)
		if os.IsNotExist(err) {
			slog.Info("Failed to load keypair. Generating one, this might take a while...")
			cert, err = tlsutil.NewCertificate(cli.Cert, cli.Key, "stdiscosrv", 20*365, false)
			if err != nil {
				slog.Error("Failed to generate X509 key pair", "error", err)
				os.Exit(1)
			}
		} else if err != nil {
			slog.Error("Failed to load keypair", "error", err)
			os.Exit(1)
		}
		devID := protocol.NewDeviceID(cert.Certificate[0])
		slog.Info("Loaded certificate keypair", "deviceId", devID.String())
	}

	// Root of the service tree.
	main := suture.New("main", suture.Spec{
		PassThroughPanics: true,
		Timeout:           2 * time.Minute,
	})

	// If configured, use blob storage for database backups.
	var blobs blob.Store
	var err error
	if cli.DBS3Endpoint != "" {
		blobs, err = s3.NewSession(cli.DBS3Endpoint, cli.DBS3Region, cli.DBS3Bucket, cli.DBS3AccessKeyID, cli.DBS3SecretKey)
	} else if cli.DBAzureBlobAccount != "" {
		blobs, err = azureblob.NewBlobStore(cli.DBAzureBlobAccount, cli.DBAzureBlobKey, cli.DBAzureBlobContainer)
	}
	if err != nil {
		slog.Error("Failed to create blob store", "error", err)
		os.Exit(1)
	}

	// Start the database.
	db := newInMemoryStore(cli.DBDir, cli.DBFlushInterval, blobs)
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
	qs := newAPISrv(cli.Listen, cert, db, repl, cli.HTTP, cli.Compression, cli.DesiredNotFoundRate)
	main.Add(qs)

	// If we have a metrics port configured, start a metrics handler.
	if cli.MetricsListen != "" {
		go func() {
			mux := http.NewServeMux()
			mux.Handle("/metrics", promhttp.Handler())
			err := http.ListenAndServe(cli.MetricsListen, mux)
			slog.Error("Failed to serve", "error", err)
			os.Exit(1)
		}()
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Cancel on signal
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)
	go func() {
		sig := <-signalChan
		slog.Info("Received signal; shutting down", "signal", sig)
		cancel()
	}()

	// Engage!
	main.Serve(ctx)
}
