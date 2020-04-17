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
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/syncthing/syncthing/lib/sha256"
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
	bs, err := ioutil.ReadAll(lr)
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
	err = ioutil.WriteFile(fullPath, buf.Bytes(), 0644)
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
			if err := sendReport(r.dsn, reportID, bs, user); err != nil {
				log.Println("Failed to send report:", err)
			}
		}()
	}
}

// 01234567890abcdef... => 01/23
func (r *crashReceiver) dirFor(base string) string {
	return filepath.Join(base[0:2], base[2:4])
}

// userIDFor returns a string we can use as the user ID for the purpose of
// counting affected users. It's the truncated hash of a salt, the user
// remote IP, and the current month.
func userIDFor(req *http.Request) string {
	addr := req.RemoteAddr
	if fwd := req.Header.Get("x-forwarded-for"); fwd != "" {
		addr = fwd
	}
	if host, _, err := net.SplitHostPort(addr); err == nil {
		addr = host
	}
	now := time.Now().Format("200601")
	salt := "stcrashreporter"
	hash := sha256.Sum256([]byte(salt + addr + now))
	return fmt.Sprintf("%x", hash[:8])
}
