// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"bytes"
	"compress/gzip"
	"io"
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
	fullPath := filepath.Join(r.dir, r.dirFor(reportID), reportID) + ".gz"

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
	bs, err := io.ReadAll(lr)
	if err != nil {
		log.Println("Reading report:", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Compress the report for storage
	buf := new(bytes.Buffer)
	gw := gzip.NewWriter(buf)
	_, _ = gw.Write(bs) // can't fail
	gw.Close()

	// Create an output file with the compressed report
	err = os.WriteFile(fullPath, buf.Bytes(), 0644)
	if err != nil {
		log.Println("Saving report:", err)
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

// 01234567890abcdef... => 01/23
func (r *crashReceiver) dirFor(base string) string {
	return filepath.Join(base[0:2], base[2:4])
}
