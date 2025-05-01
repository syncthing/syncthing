// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/syncthing/syncthing/internal/db"
)

const (
	maxCSRFTokenLifetime = time.Hour
	maxActiveCSRFTokens  = 25
)

type csrfManager struct {
	unique          string
	prefix          string
	apiKeyValidator apiKeyValidator
	next            http.Handler
	tokens          *tokenManager
}

type apiKeyValidator interface {
	IsValidAPIKey(key string) bool
}

// Check for CSRF token on /rest/ URLs. If a correct one is not given, reject
// the request with 403. For / and /index.html, set a new CSRF cookie if none
// is currently set.
func newCsrfManager(unique string, prefix string, apiKeyValidator apiKeyValidator, next http.Handler, miscDB *db.Typed) *csrfManager {
	m := &csrfManager{
		unique:          unique,
		prefix:          prefix,
		apiKeyValidator: apiKeyValidator,
		next:            next,
		tokens:          newTokenManager("csrfTokens", miscDB, maxCSRFTokenLifetime, maxActiveCSRFTokens),
	}
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

	// Allow requests for anything not under the protected path prefix,
	// and set a CSRF cookie if there isn't already a valid one.
	if !strings.HasPrefix(r.URL.Path, m.prefix) {
		cookie, err := r.Cookie("CSRF-Token-" + m.unique)
		if err != nil || !m.tokens.Check(cookie.Value) {
			l.Debugln("new CSRF cookie in response to request for", r.URL)
			cookie = &http.Cookie{
				Name:  "CSRF-Token-" + m.unique,
				Value: m.tokens.New(),
			}
			http.SetCookie(w, cookie)
		}
		m.next.ServeHTTP(w, r)
		return
	}

	if isNoAuthPath(r.URL.Path, false) {
		// REST calls that don't require authentication also do not
		// need a CSRF token.
		m.next.ServeHTTP(w, r)
		return
	}

	// Verify the CSRF token
	token := r.Header.Get("X-CSRF-Token-" + m.unique)
	if !m.tokens.Check(token) {
		http.Error(w, "CSRF Error", http.StatusForbidden)
		return
	}

	m.next.ServeHTTP(w, r)
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
