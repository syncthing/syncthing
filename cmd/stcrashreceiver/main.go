// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// Command stcrashreceiver is a trivial HTTP server that allows two things:
//
// - uploading files (crash reports) named like a SHA256 hash using a PUT request
// - checking whether such file exists using a HEAD request
//
// Typically this should be deployed behind something that manages HTTPS.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/alecthomas/kong"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/syncthing/syncthing/lib/sha256"
	"github.com/syncthing/syncthing/lib/ur"

	raven "github.com/getsentry/raven-go"
)

const maxRequestSize = 1 << 20 // 1 MiB

type cli struct {
	Dir           string `help:"Parent directory to store crash and failure reports in" env:"REPORTS_DIR" default:"."`
	DSN           string `help:"Sentry DSN" env:"SENTRY_DSN"`
	Listen        string `help:"HTTP listen address" default:":8080" env:"LISTEN_ADDRESS"`
	MaxDiskFiles  int    `help:"Maximum number of reports on disk" default:"100000" env:"MAX_DISK_FILES"`
	MaxDiskSizeMB int64  `help:"Maximum disk space to use for reports" default:"1024" env:"MAX_DISK_SIZE_MB"`
	SentryQueue   int    `help:"Maximum number of reports to queue for sending to Sentry" default:"64" env:"SENTRY_QUEUE"`
	DiskQueue     int    `help:"Maximum number of reports to queue for writing to disk" default:"64" env:"DISK_QUEUE"`
}

func main() {
	var params cli
	kong.Parse(&params)

	mux := http.NewServeMux()

	ds := &diskStore{
		dir:      filepath.Join(params.Dir, "crash_reports"),
		inbox:    make(chan diskEntry, params.DiskQueue),
		maxFiles: params.MaxDiskFiles,
		maxBytes: params.MaxDiskSizeMB << 20,
	}
	go ds.Serve(context.Background())

	ss := &sentryService{
		dsn:   params.DSN,
		inbox: make(chan sentryRequest, params.SentryQueue),
	}
	go ss.Serve(context.Background())

	cr := &crashReceiver{
		store:  ds,
		sentry: ss,
	}

	mux.Handle("/", cr)
	mux.HandleFunc("/ping", func(w http.ResponseWriter, req *http.Request) {
		w.Write([]byte("OK"))
	})
	mux.Handle("/metrics", promhttp.Handler())

	if params.DSN != "" {
		mux.HandleFunc("/newcrash/failure", handleFailureFn(params.DSN, filepath.Join(params.Dir, "failure_reports")))
	}

	log.SetOutput(os.Stdout)
	if err := http.ListenAndServe(params.Listen, mux); err != nil {
		log.Fatalln("HTTP serve:", err)
	}
}

func handleFailureFn(dsn, failureDir string) func(w http.ResponseWriter, req *http.Request) {
	return func(w http.ResponseWriter, req *http.Request) {
		result := "failure"
		defer func() {
			metricFailureReportsTotal.WithLabelValues(result).Inc()
		}()

		lr := io.LimitReader(req.Body, maxRequestSize)
		bs, err := io.ReadAll(lr)
		req.Body.Close()
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		var reports []ur.FailureReport
		err = json.Unmarshal(bs, &reports)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		if len(reports) == 0 {
			// Shouldn't happen
			log.Printf("Got zero failure reports")
			return
		}

		version, err := parseVersion(reports[0].Version)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		for _, r := range reports {
			pkt := packet(version, "failure")
			pkt.Message = r.Description
			pkt.Extra = raven.Extra{
				"count": r.Count,
			}
			for k, v := range r.Extra {
				pkt.Extra[k] = v
			}
			if r.Goroutines != "" {
				url, err := saveFailureWithGoroutines(r.FailureData, failureDir)
				if err != nil {
					log.Println("Saving failure report:", err)
					http.Error(w, "Internal server error", http.StatusInternalServerError)
					return
				}
				pkt.Extra["goroutinesURL"] = url
			}
			message := sanitizeMessageLDB(r.Description)
			pkt.Fingerprint = []string{message}

			if err := sendReport(dsn, pkt, userIDFor(req)); err != nil {
				log.Println("Failed to send failure report:", err)
			} else {
				log.Println("Sent failure report:", r.Description)
				result = "success"
			}
		}
	}
}

func saveFailureWithGoroutines(data ur.FailureData, failureDir string) (string, error) {
	bs := make([]byte, len(data.Description)+len(data.Goroutines))
	copy(bs, data.Description)
	copy(bs[len(data.Description):], data.Goroutines)
	id := fmt.Sprintf("%x", sha256.Sum256(bs))
	path := fullPathCompressed(failureDir, id)
	err := compressAndWrite(bs, path)
	if err != nil {
		return "", err
	}
	return reportServer + path, nil
}
