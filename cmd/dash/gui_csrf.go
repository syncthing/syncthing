// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package main

import (
	"bufio"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/sync"
)

var csrfTokens []string
var csrfMut = sync.NewMutex()

// Check for CSRF token on /rest/ URLs. If a correct one is not given, reject
// the request with 403. For / and /index.html, set a new CSRF cookie if none
// is currently set.
func csrfMiddleware(unique, prefix, apiKey string, next http.Handler) http.Handler {
	loadCsrfTokens()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Allow requests carrying a valid API key
		if apiKey != "" && r.Header.Get("X-API-Key") == apiKey {
			next.ServeHTTP(w, r)
			return
		}

		// Allow requests for the front page, and set a CSRF cookie if there isn't already a valid one.
		if !strings.HasPrefix(r.URL.Path, prefix) {
			cookie, err := r.Cookie("CSRF-Token-" + unique)
			if err != nil || !validCsrfToken(cookie.Value) {
				cookie = &http.Cookie{
					Name:  "CSRF-Token-" + unique,
					Value: newCsrfToken(),
				}
				http.SetCookie(w, cookie)
			}
			next.ServeHTTP(w, r)
			return
		}

		if r.Method == "GET" {
			// Allow GET requests unconditionally
			next.ServeHTTP(w, r)
			return
		}

		// Verify the CSRF token
		token := r.Header.Get("X-CSRF-Token-" + unique)
		if !validCsrfToken(token) {
			http.Error(w, "CSRF Error", 403)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func validCsrfToken(token string) bool {
	csrfMut.Lock()
	defer csrfMut.Unlock()
	for _, t := range csrfTokens {
		if t == token {
			return true
		}
	}
	return false
}

func newCsrfToken() string {
	token := randomString(32)

	csrfMut.Lock()
	csrfTokens = append(csrfTokens, token)
	if len(csrfTokens) > 10 {
		csrfTokens = csrfTokens[len(csrfTokens)-10:]
	}
	defer csrfMut.Unlock()

	saveCsrfTokens()

	return token
}

func saveCsrfTokens() {
	// We're ignoring errors in here. It's not super critical and there's
	// nothing relevant we can do about them anyway...

	name := locations[locCsrfTokens]
	f, err := osutil.CreateAtomic(name, 0600)
	if err != nil {
		return
	}

	for _, t := range csrfTokens {
		fmt.Fprintln(f, t)
	}

	f.Close()
}

func loadCsrfTokens() {
	f, err := os.Open(locations[locCsrfTokens])
	if err != nil {
		return
	}
	defer f.Close()

	s := bufio.NewScanner(f)
	for s.Scan() {
		csrfTokens = append(csrfTokens, s.Text())
	}
}
