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
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/syncthing/syncthing/lib/sha256"
	"github.com/syncthing/syncthing/lib/ur"

	raven "github.com/getsentry/raven-go"
)

const maxRequestSize = 1 << 20 // 1 MiB

func main() {
	dir := flag.String("dir", ".", "Directory to store reports in")
	dsn := flag.String("dsn", "", "Sentry DSN")
	listen := flag.String("listen", ":22039", "HTTP listen address")
	flag.Parse()

	mux := http.NewServeMux()

	cr := &crashReceiver{
		dir: *dir,
		dsn: *dsn,
	}
	mux.Handle("/", cr)

	if *dsn != "" {
		mux.HandleFunc("/newcrash/failure", handleFailureFn(*dsn))
	}

	log.SetOutput(os.Stdout)
	if err := http.ListenAndServe(*listen, mux); err != nil {
		log.Fatalln("HTTP serve:", err)
	}
}

func handleFailureFn(dsn string) func(w http.ResponseWriter, req *http.Request) {
	return func(w http.ResponseWriter, req *http.Request) {
		lr := io.LimitReader(req.Body, maxRequestSize)
		bs, err := ioutil.ReadAll(lr)
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
			pkt := packet(version)
			pkt.Message = r.Description
			pkt.Extra = raven.Extra{
				"count": r.Count,
			}
			pkt.Fingerprint = []string{r.Description}

			if err := sendReport(dsn, pkt, userIDFor(req)); err != nil {
				log.Println("Failed to send failure report:", err)
			} else {
				log.Println("Sent failure report:", r.Description)
			}
		}
	}
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
