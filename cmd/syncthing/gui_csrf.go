// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"bufio"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/rand"
	"github.com/syncthing/syncthing/lib/sync"
)

// csrfTokens is a list of valid tokens. It is sorted so that the most
// recently used token is first in the list. New tokens are added to the front
// of the list (as it is the most recently used at that time). The list is
// pruned to a maximum of maxCsrfTokens, throwing away the least recently used
// tokens.
var csrfTokens []string
var csrfMut = sync.NewMutex()

const maxCsrfTokens = 25

// Check for CSRF token on /rest/ URLs. If a correct one is not given, reject
// the request with 403. For / and /index.html, set a new CSRF cookie if none
// is currently set.
func csrfMiddleware(unique string, prefix string, cfg config.GUIConfiguration, next http.Handler) http.Handler {
	loadCsrfTokens()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Allow requests carrying a valid API key
		if cfg.IsValidAPIKey(r.Header.Get("X-API-Key")) {
			// Set the access-control-allow-origin header for CORS requests
			// since a valid API key has been provided
			w.Header().Add("Access-Control-Allow-Origin", "*")
			next.ServeHTTP(w, r)
			return
		}

		if strings.HasPrefix(r.URL.Path, "/rest/debug") {
			// Debugging functions are only available when explicitly
			// enabled, and can be accessed without a CSRF token
			next.ServeHTTP(w, r)
			return
		}

		// Allow requests for anything not under the protected path prefix,
		// and set a CSRF cookie if there isn't already a valid one.
		if !strings.HasPrefix(r.URL.Path, prefix) {
			cookie, err := r.Cookie("CSRF-Token-" + unique)
			if err != nil || !validCsrfToken(cookie.Value) {
				httpl.Debugln("new CSRF cookie in response to request for", r.URL)
				cookie = &http.Cookie{
					Name:  "CSRF-Token-" + unique,
					Value: newCsrfToken(),
				}
				http.SetCookie(w, cookie)
			}
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
	for i, t := range csrfTokens {
		if t == token {
			if i > 0 {
				// Move this token to the head of the list. Copy the tokens at
				// the front one step to the right and then replace the token
				// at the head.
				copy(csrfTokens[1:], csrfTokens[:i+1])
				csrfTokens[0] = token
			}
			return true
		}
	}
	return false
}

func newCsrfToken() string {
	token := rand.String(32)

	csrfMut.Lock()
	csrfTokens = append([]string{token}, csrfTokens...)
	if len(csrfTokens) > maxCsrfTokens {
		csrfTokens = csrfTokens[:maxCsrfTokens]
	}
	defer csrfMut.Unlock()

	saveCsrfTokens()

	return token
}

func saveCsrfTokens() {
	// We're ignoring errors in here. It's not super critical and there's
	// nothing relevant we can do about them anyway...

	name := locations[locCsrfTokens]
	f, err := osutil.CreateAtomic(name)
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
