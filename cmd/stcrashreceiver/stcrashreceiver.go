// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"io"
	"log"
	"net/http"
	"path"
	"strings"
)

type crashReceiver struct {
	store  *diskStore
	sentry *sentryService
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

	switch req.Method {
	case http.MethodGet:
		r.serveGet(reportID, w, req)
	case http.MethodHead:
		r.serveHead(reportID, w, req)
	case http.MethodPut:
		r.servePut(reportID, w, req)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// serveGet responds to GET requests by serving the uncompressed report.
func (r *crashReceiver) serveGet(reportID string, w http.ResponseWriter, _ *http.Request) {
	bs, err := r.store.Get(reportID)
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	w.Write(bs)
}

// serveHead responds to HEAD requests by checking if the named report
// already exists in the system.
func (r *crashReceiver) serveHead(reportID string, w http.ResponseWriter, _ *http.Request) {
	if !r.store.Exists(reportID) {
		http.Error(w, "Not found", http.StatusNotFound)
	}
}

// servePut accepts and stores the given report.
func (r *crashReceiver) servePut(reportID string, w http.ResponseWriter, req *http.Request) {
	// Read at most maxRequestSize of report data.
	log.Println("Receiving report", reportID)
	lr := io.LimitReader(req.Body, maxRequestSize)
	bs, err := io.ReadAll(lr)
	if err != nil {
		log.Println("Reading report:", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Store the report
	if !r.store.Put(reportID, bs) {
		log.Println("Failed to store report (queue full):", reportID)
	}

	// Send the report to Sentry
	if !r.sentry.Send(reportID, userIDFor(req), bs) {
		log.Println("Failed to send report to sentry (queue full):", reportID)
	}
}
