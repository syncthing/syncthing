// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package api

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"slices"
	"strings"
	"time"

	ldap "github.com/go-ldap/ldap/v3"
	"github.com/syncthing/syncthing/internal/slogutil"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/rand"
)

const (
	maxSessionLifetime  = 7 * 24 * time.Hour
	maxActiveSessions   = 25
	randomTokenLength   = 64
	maxLoginRequestSize = 1 << 10 // one kibibyte for username+password
)

func emitLoginAttempt(success bool, username string, r *http.Request, evLogger events.Logger) {
	remoteAddress, proxy := remoteAddress(r)
	evData := map[string]any{
		"success":       success,
		"username":      username,
		"remoteAddress": remoteAddress,
	}
	if proxy != "" {
		evData["proxy"] = proxy
	}
	evLogger.Log(events.LoginAttempt, evData)

	if success {
		return
	}
	l := slog.Default().With(slogutil.Address(remoteAddress), slog.String("username", username))
	if proxy != "" {
		l = l.With("proxy", proxy)
	}
	l.Warn("Bad credentials supplied during API authorization")
}

func remoteAddress(r *http.Request) (remoteAddr, proxy string) {
	remoteAddr = r.RemoteAddr
	remoteIP := osutil.IPFromString(r.RemoteAddr)

	// parse X-Forwarded-For only if the proxy connects via unix socket, localhost or a LAN IP
	var localProxy bool
	if remoteIP != nil {
		remoteAddr = remoteIP.String()
		localProxy = remoteIP.IsLoopback() || remoteIP.IsPrivate() || remoteIP.IsLinkLocalUnicast()
	} else if remoteAddr == "@" {
		localProxy = true
	}

	if !localProxy {
		return
	}

	forwardedAddr, _, _ := strings.Cut(r.Header.Get("X-Forwarded-For"), ",")
	forwardedAddr = strings.TrimSpace(forwardedAddr)
	forwardedIP := osutil.IPFromString(forwardedAddr)

	if forwardedIP != nil {
		proxy = remoteAddr
		remoteAddr = forwardedIP.String()
	}
	return
}

func antiBruteForceSleep() {
	time.Sleep(time.Duration(rand.Intn(100)+100) * time.Millisecond)
}

func unauthorized(w http.ResponseWriter, shortID string) {
	w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Basic realm="Authorization Required (%s)"`, shortID))
	http.Error(w, "Not Authorized", http.StatusUnauthorized)
}

func forbidden(w http.ResponseWriter) {
	http.Error(w, "Forbidden", http.StatusForbidden)
}

func isNoAuthPath(path string, metricsWithoutAuth bool) bool {
	// Local variable instead of module var to prevent accidental mutation
	noAuthPaths := []string{
		"/",
		"/index.html",
		"/modal.html",
		"/rest/svc/lang", // Required to load language settings on login page
	}

	if metricsWithoutAuth {
		noAuthPaths = append(noAuthPaths, "/metrics")
	}

	// Local variable instead of module var to prevent accidental mutation
	noAuthPrefixes := []string{
		// Static assets
		"/assets/",
		"/syncthing/",
		"/vendor/",
		"/theme-assets/", // This leaks information from config, but probably not sensitive

		// No-auth API endpoints
		"/rest/noauth",
	}

	return slices.Contains(noAuthPaths, path) ||
		slices.ContainsFunc(noAuthPrefixes, func(prefix string) bool {
			return strings.HasPrefix(path, prefix)
		})
}

type basicAuthAndSessionMiddleware struct {
	tokenCookieManager *tokenCookieManager
	guiCfg             config.GUIConfiguration
	ldapCfg            config.LDAPConfiguration
	next               http.Handler
	evLogger           events.Logger
}

func newBasicAuthAndSessionMiddleware(tokenCookieManager *tokenCookieManager, guiCfg config.GUIConfiguration, ldapCfg config.LDAPConfiguration, next http.Handler, evLogger events.Logger) *basicAuthAndSessionMiddleware {
	return &basicAuthAndSessionMiddleware{
		tokenCookieManager: tokenCookieManager,
		guiCfg:             guiCfg,
		ldapCfg:            ldapCfg,
		next:               next,
		evLogger:           evLogger,
	}
}

func (m *basicAuthAndSessionMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if hasValidAPIKeyHeader(r, m.guiCfg) {
		m.next.ServeHTTP(w, r)
		return
	}

	if m.tokenCookieManager.hasValidSession(r) {
		m.next.ServeHTTP(w, r)
		return
	}

	// Fall back to Basic auth if provided
	if username, ok := attemptBasicAuth(r, m.guiCfg, m.ldapCfg, m.evLogger); ok {
		m.tokenCookieManager.createSession(username, false, w, r)
		m.next.ServeHTTP(w, r)
		return
	}

	// Exception for static assets and REST calls that don't require authentication.
	if isNoAuthPath(r.URL.Path, m.guiCfg.MetricsWithoutAuth) {
		m.next.ServeHTTP(w, r)
		return
	}

	// Some browsers don't send the Authorization request header unless prompted by a 401 response.
	// This enables https://user:pass@localhost style URLs to keep working.
	if m.guiCfg.SendBasicAuthPrompt {
		unauthorized(w, m.tokenCookieManager.shortID)
		return
	}

	forbidden(w)
}

func (m *basicAuthAndSessionMiddleware) passwordAuthHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username     string
		Password     string
		StayLoggedIn bool
	}
	if err := unmarshalTo(http.MaxBytesReader(w, r.Body, maxLoginRequestSize), &req); err != nil {
		l.Debugln("Failed to parse username and password:", err)
		http.Error(w, "Failed to parse username and password.", http.StatusBadRequest)
		return
	}

	if auth(req.Username, req.Password, m.guiCfg, m.ldapCfg) {
		m.tokenCookieManager.createSession(req.Username, req.StayLoggedIn, w, r)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	emitLoginAttempt(false, req.Username, r, m.evLogger)
	antiBruteForceSleep()
	forbidden(w)
}

func attemptBasicAuth(r *http.Request, guiCfg config.GUIConfiguration, ldapCfg config.LDAPConfiguration, evLogger events.Logger) (string, bool) {
	username, password, ok := r.BasicAuth()
	if !ok {
		return "", false
	}

	slog.Debug("Sessionless HTTP request with authentication; this is expensive.")

	if auth(username, password, guiCfg, ldapCfg) {
		return username, true
	}

	usernameFromIso := string(iso88591ToUTF8([]byte(username)))
	passwordFromIso := string(iso88591ToUTF8([]byte(password)))
	if auth(usernameFromIso, passwordFromIso, guiCfg, ldapCfg) {
		return usernameFromIso, true
	}

	emitLoginAttempt(false, username, r, evLogger)
	antiBruteForceSleep()
	return "", false
}

func (m *basicAuthAndSessionMiddleware) handleLogout(w http.ResponseWriter, r *http.Request) {
	m.tokenCookieManager.destroySession(w, r)
	w.WriteHeader(http.StatusNoContent)
}

func auth(username string, password string, guiCfg config.GUIConfiguration, ldapCfg config.LDAPConfiguration) bool {
	if guiCfg.AuthMode == config.AuthModeLDAP {
		return authLDAP(username, password, ldapCfg)
	} else {
		return authStatic(username, password, guiCfg)
	}
}

func authStatic(username string, password string, guiCfg config.GUIConfiguration) bool {
	return guiCfg.CompareHashedPassword(password) == nil && username == guiCfg.User
}

func authLDAP(username string, password string, cfg config.LDAPConfiguration) bool {
	address := cfg.Address
	hostname, _, err := net.SplitHostPort(address)
	if err != nil {
		hostname = address
	}
	var connection *ldap.Conn
	if cfg.Transport == config.LDAPTransportTLS {
		connection, err = ldap.DialTLS("tcp", address, &tls.Config{
			ServerName:         hostname,
			InsecureSkipVerify: cfg.InsecureSkipVerify,
		})
	} else {
		connection, err = ldap.Dial("tcp", address)
	}

	if err != nil {
		slog.Error("Failed to dial LDAP server", slogutil.Error(err))
		return false
	}

	if cfg.Transport == config.LDAPTransportStartTLS {
		err = connection.StartTLS(&tls.Config{InsecureSkipVerify: cfg.InsecureSkipVerify})
		if err != nil {
			slog.Error("Failed to handshake start TLS With LDAP server", slogutil.Error(err))
			return false
		}
	}

	defer connection.Close()

	bindDN := formatOptionalPercentS(cfg.BindDN, escapeForLDAPDN(username))
	err = connection.Bind(bindDN, password)
	if err != nil {
		slog.Error("Failed to bind with LDAP server", slogutil.Error(err))
		return false
	}

	if cfg.SearchFilter == "" && cfg.SearchBaseDN == "" {
		// We're done here.
		return true
	}

	if cfg.SearchFilter == "" || cfg.SearchBaseDN == "" {
		slog.Error("Bad LDAP configuration: both searchFilter and searchBaseDN must be set, or neither")
		return false
	}

	// If a search filter and search base is set we do an LDAP search for
	// the user. If this matches precisely one user then we are good to go.
	// The search filter uses the same %s interpolation as the bind DN.

	searchString := formatOptionalPercentS(cfg.SearchFilter, escapeForLDAPFilter(username))
	const sizeLimit = 2  // we search for up to two users -- we only want to match one, so getting any number >1 is a failure.
	const timeLimit = 60 // Search for up to a minute...
	searchReq := ldap.NewSearchRequest(cfg.SearchBaseDN, ldap.ScopeWholeSubtree, ldap.DerefFindingBaseObj, sizeLimit, timeLimit, false, searchString, nil, nil)

	res, err := connection.Search(searchReq)
	if err != nil {
		slog.Warn("Failed LDAP search", slogutil.Error(err))
		return false
	}
	if len(res.Entries) != 1 {
		slog.Warn("Incorrect number of LDAP search results (expected one)", slog.Int("results", len(res.Entries)))
		return false
	}

	return true
}

// escapeForLDAPFilter escapes a value that will be used in a filter clause
func escapeForLDAPFilter(value string) string {
	// https://social.technet.microsoft.com/wiki/contents/articles/5392.active-directory-ldap-syntax-filters.aspx#Special_Characters
	// Backslash must always be first in the list so we don't double escape them.
	return escapeRunes(value, []rune{'\\', '*', '(', ')', 0})
}

// escapeForLDAPDN escapes a value that will be used in a bind DN
func escapeForLDAPDN(value string) string {
	// https://social.technet.microsoft.com/wiki/contents/articles/5312.active-directory-characters-to-escape.aspx
	// Backslash must always be first in the list so we don't double escape them.
	return escapeRunes(value, []rune{'\\', ',', '#', '+', '<', '>', ';', '"', '=', ' ', 0})
}

func escapeRunes(value string, runes []rune) string {
	for _, e := range runes {
		value = strings.ReplaceAll(value, string(e), fmt.Sprintf("\\%X", e))
	}
	return value
}

func formatOptionalPercentS(template string, username string) string {
	var replacements []any
	nReps := strings.Count(template, "%s") - strings.Count(template, "%%s")
	if nReps < 0 {
		nReps = 0
	}
	for range nReps {
		replacements = append(replacements, username)
	}
	return fmt.Sprintf(template, replacements...)
}

// Convert an ISO-8859-1 encoded byte string to UTF-8. Works by the
// principle that ISO-8859-1 bytes are equivalent to unicode code points,
// that a rune slice is a list of code points, and that stringifying a slice
// of runes generates UTF-8 in Go.
func iso88591ToUTF8(s []byte) []byte {
	runes := make([]rune, len(s))
	for i := range s {
		runes[i] = rune(s[i])
	}
	return []byte(string(runes))
}
