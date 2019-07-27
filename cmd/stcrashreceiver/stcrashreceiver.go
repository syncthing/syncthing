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
	"bytes"
	"compress/gzip"
	"flag"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const maxRequestSize = 1 << 20 // 1 MiB

func main() {
	dir := flag.String("dir", ".", "Directory to store reports in")
	dsn := flag.String("dsn", "", "Sentry DSN")
	listen := flag.String("listen", ":22039", "HTTP listen address")
	flag.Parse()

	cr := &crashReceiver{
		dir: *dir,
		dsn: *dsn,
	}

	log.SetOutput(os.Stdout)
	if err := http.ListenAndServe(*listen, cr); err != nil {
		log.Fatalln("HTTP serve:", err)
	}
}

type crashReceiver struct {
	dir string
	dsn string
}

func (r *crashReceiver) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// The final path component should be a SHA256 hash in hex, so 64 hex
	// characters. We don't care about case on the request but use lower
	// case internally.
	base := strings.ToLower(path.Base(req.URL.Path))
	if len(base) != 64 {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	for _, c := range base {
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
		r.serveGet(base, w, req)
	case http.MethodHead:
		r.serveHead(base, w, req)
	case http.MethodPut:
		r.servePut(base, w, req)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// serveGet responds to GET requests by serving the uncompressed report.
func (r *crashReceiver) serveGet(base string, w http.ResponseWriter, req *http.Request) {
	path := filepath.Join(r.dirFor(base), base)
	fullPath := filepath.Join(r.dir, path)
	if fd, err := os.Open(fullPath + ".gz"); err == nil {
		// There is a compressed report
		defer fd.Close()
		gr, err := gzip.NewReader(fd)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		_, _ = io.Copy(w, gr) // best effort
		return
	}
	if _, err := os.Lstat(fullPath); err == nil {
		http.ServeFile(w, req, fullPath)
		return
	}
	http.Error(w, "Not found", http.StatusNotFound)
}

// serveHead responds to HEAD requests by checking if the named report
// already exists in the system.
func (r *crashReceiver) serveHead(base string, w http.ResponseWriter, _ *http.Request) {
	path := filepath.Join(r.dirFor(base), base)
	fullPath := filepath.Join(r.dir, path)
	if _, err := os.Lstat(fullPath + ".gz"); err == nil {
		// There is a compressed report
		return
	}
	if _, err := os.Lstat(fullPath); err == nil {
		// There is an uncompressed report
		return
	}
	http.Error(w, "Not found", http.StatusNotFound)
}

// servePut accepts and stores the given report.
func (r *crashReceiver) servePut(base string, w http.ResponseWriter, req *http.Request) {
	path := filepath.Join(r.dirFor(base), base)
	fullPath := filepath.Join(r.dir, path)

	// Ensure the destination directory exists
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		log.Printf("Creating directory for report %s: %v", base, err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Read at most maxRequestSize of report data.
	log.Println("Receiving report", base)
	lr := io.LimitReader(req.Body, maxRequestSize)
	bs, err := ioutil.ReadAll(lr)
	if err != nil {
		log.Println("Reading report:", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Compress the report for storage
	buf := new(bytes.Buffer)
	gw := gzip.NewWriter(buf)
	gw.Write(bs)
	gw.Close()

	// Create an output file with the compressed report
	err = ioutil.WriteFile(fullPath+".gz", buf.Bytes(), 0644)
	if err != nil {
		log.Printf("Creating file for report %s: %v", base, err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Send the report to Sentry
	if r.dsn != "" {
		go func() {
			// There's no need for the client to have to wait for this part.
			if err := sendReport(r.dsn, base, bs); err != nil {
				log.Println("Failed to send report:", err)
			}
		}()
	}
}

// 01234567890abcdef... => 01/23
func (r *crashReceiver) dirFor(base string) string {
	return filepath.Join(base[0:2], base[2:4])
}
