// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"compress/gzip"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type crashReceiver struct {
	dir string
	dsn string
}

func (r *crashReceiver) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// The final path component should be a SHA256 hash in hex, so 64 hex
	// characters. We don't care about case on the request but use lower
	// case internally.
	reportID := strings.ToLower(path.Base(req.URL.Path))
	if len(reportID) != 64 {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	for _, c := range reportID {
		if c >= 'a' && c <= 'f' {
			continue
		}
		if c >= '0' && c <= '9' {
			continue
		}
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	// The location of the report on disk, compressed
	fullPath := fullPathCompressed(r.dir, reportID)

	switch req.Method {
	case http.MethodGet:
		r.serveGet(fullPath, w, req)
	case http.MethodHead:
		r.serveHead(fullPath, w, req)
	case http.MethodPut:
		r.servePut(reportID, fullPath, w, req)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// serveGet responds to GET requests by serving the uncompressed report.
func (r *crashReceiver) serveGet(fullPath string, w http.ResponseWriter, _ *http.Request) {
	fd, err := os.Open(fullPath)
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	defer fd.Close()
	gr, err := gzip.NewReader(fd)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	_, _ = io.Copy(w, gr) // best effort
}

// serveHead responds to HEAD requests by checking if the named report
// already exists in the system.
func (r *crashReceiver) serveHead(fullPath string, w http.ResponseWriter, _ *http.Request) {
	if _, err := os.Lstat(fullPath); err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
	}
}

// servePut accepts and stores the given report.
func (r *crashReceiver) servePut(reportID, fullPath string, w http.ResponseWriter, req *http.Request) {
	// Ensure the destination directory exists
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		log.Println("Creating directory:", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Read at most maxRequestSize of report data.
	log.Println("Receiving report", reportID)
	lr := io.LimitReader(req.Body, maxRequestSize)
	bs, err := ioutil.ReadAll(lr)
	if err != nil {
		log.Println("Reading report:", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	err = compressAndWrite(bs, fullPath)
	if err != nil {
		log.Println("Saving crash report:", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Send the report to Sentry
	if r.dsn != "" {
		// Remote ID
		user := userIDFor(req)

		go func() {
			// There's no need for the client to have to wait for this part.
			pkt, err := parseCrashReport(reportID, bs)
			if err != nil {
				log.Println("Failed to parse crash report:", err)
				return
			}
			if err := sendReport(r.dsn, pkt, user); err != nil {
				log.Println("Failed to send crash report:", err)
			}
		}()
	}
}
