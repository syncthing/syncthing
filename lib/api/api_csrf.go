// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package api

import (
	"bufio"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/rand"
	"github.com/syncthing/syncthing/lib/sync"
)

const maxCsrfTokens = 25

type csrfManager struct {
	// tokens is a list of valid tokens. It is sorted so that the most
	// recently used token is first in the list. New tokens are added to the front
	// of the list (as it is the most recently used at that time). The list is
	// pruned to a maximum of maxCsrfTokens, throwing away the least recently used
	// tokens.
	tokens    []string
	tokensMut sync.Mutex

	unique          string
	prefix          string
	apiKeyValidator apiKeyValidator
	next            http.Handler
	saveLocation    string
}

type apiKeyValidator interface {
	IsValidAPIKey(key string) bool
}

// Check for CSRF token on /rest/ URLs. If a correct one is not given, reject
// the request with 403. For / and /index.html, set a new CSRF cookie if none
// is currently set.
func newCsrfManager(unique string, prefix string, apiKeyValidator apiKeyValidator, next http.Handler, saveLocation string) *csrfManager {
	m := &csrfManager{
		tokensMut:       sync.NewMutex(),
		tokens:          make([]string, 0, maxCsrfTokens),
		unique:          unique,
		prefix:          prefix,
		apiKeyValidator: apiKeyValidator,
		next:            next,
		saveLocation:    saveLocation,
	}
	m.load()
	return m
}

func (m *csrfManager) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Allow requests carrying a valid API key
	if hasValidAPIKeyHeader(r, m.apiKeyValidator) {
		// Set the access-control-allow-origin header for CORS requests
		// since a valid API key has been provided
		w.Header().Add("Access-Control-Allow-Origin", "*")
		m.next.ServeHTTP(w, r)
		return
	}

	if strings.HasPrefix(r.URL.Path, "/rest/debug") {
		// Debugging functions are only available when explicitly
		// enabled, and can be accessed without a CSRF token
		m.next.ServeHTTP(w, r)
		return
	}

	if strings.HasPrefix(r.URL.Path, "/rest/noauth") { // FIXME: this duplicates some logic in basicAuthAndSessionMiddleware in api_auth.go
		// REST calls that don't require authentication also do not
		// need a CSRF token.
		m.next.ServeHTTP(w, r)
		return
	}

	// Allow requests for anything not under the protected path prefix,
	// and set a CSRF cookie if there isn't already a valid one.
	if !strings.HasPrefix(r.URL.Path, m.prefix) {
		cookie, err := r.Cookie("CSRF-Token-" + m.unique)
		if err != nil || !m.validToken(cookie.Value) {
			l.Debugln("new CSRF cookie in response to request for", r.URL)
			cookie = &http.Cookie{
				Name:  "CSRF-Token-" + m.unique,
				Value: m.newToken(),
			}
			http.SetCookie(w, cookie)
		}
		m.next.ServeHTTP(w, r)
		return
	}

	// Verify the CSRF token
	token := r.Header.Get("X-CSRF-Token-" + m.unique)
	if !m.validToken(token) {
		http.Error(w, "CSRF Error", http.StatusForbidden)
		return
	}

	m.next.ServeHTTP(w, r)
}

func (m *csrfManager) validToken(token string) bool {
	m.tokensMut.Lock()
	defer m.tokensMut.Unlock()
	for i, t := range m.tokens {
		if t == token {
			if i > 0 {
				// Move this token to the head of the list. Copy the tokens at
				// the front one step to the right and then replace the token
				// at the head.
				copy(m.tokens[1:], m.tokens[:i])
				m.tokens[0] = token
			}
			return true
		}
	}
	return false
}

func (m *csrfManager) newToken() string {
	token := rand.String(32)

	m.tokensMut.Lock()
	defer m.tokensMut.Unlock()

	if len(m.tokens) < maxCsrfTokens {
		m.tokens = append(m.tokens, "")
	}
	copy(m.tokens[1:], m.tokens)
	m.tokens[0] = token

	m.save()

	return token
}

func (m *csrfManager) save() {
	// We're ignoring errors in here. It's not super critical and there's
	// nothing relevant we can do about them anyway...

	if m.saveLocation == "" {
		return
	}

	f, err := osutil.CreateAtomic(m.saveLocation)
	if err != nil {
		return
	}

	for _, t := range m.tokens {
		fmt.Fprintln(f, t)
	}

	f.Close()
}

func (m *csrfManager) load() {
	if m.saveLocation == "" {
		return
	}

	f, err := os.Open(m.saveLocation)
	if err != nil {
		return
	}
	defer f.Close()

	s := bufio.NewScanner(f)
	for s.Scan() {
		m.tokens = append(m.tokens, s.Text())
	}
}

func hasValidAPIKeyHeader(r *http.Request, validator apiKeyValidator) bool {
	if key := r.Header.Get("X-API-Key"); validator.IsValidAPIKey(key) {
		return true
	}
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		bearerToken := auth[len("bearer "):]
		return validator.IsValidAPIKey(bearerToken)
	}
	return false
}
