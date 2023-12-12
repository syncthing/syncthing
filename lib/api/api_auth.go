// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package api

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	ldap "github.com/go-ldap/ldap/v3"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/rand"
	"github.com/syncthing/syncthing/lib/sync"
	"golang.org/x/exp/slices"
)

const (
	maxSessionLifetime = 7 * 24 * time.Hour
	maxActiveSessions  = 25
	randomTokenLength  = 64
)

func emitLoginAttempt(success bool, username, address string, evLogger events.Logger) {
	evLogger.Log(events.LoginAttempt, map[string]interface{}{
		"success":       success,
		"username":      username,
		"remoteAddress": address,
	})
	if !success {
		l.Infof("Wrong credentials supplied during API authorization from %s", address)
	}
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

func isNoAuthPath(path string) bool {
	// Local variable instead of module var to prevent accidental mutation
	noAuthPaths := []string{
		"/",
		"/index.html",
		"/modal.html",
		"/rest/svc/lang", // Required to load language settings on login page
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
	cookieName string
	shortID    string
	guiCfg     config.GUIConfiguration
	ldapCfg    config.LDAPConfiguration
	next       http.Handler
	evLogger   events.Logger
	tokens     *tokenManager
}

func newBasicAuthAndSessionMiddleware(cookieName, shortID string, guiCfg config.GUIConfiguration, ldapCfg config.LDAPConfiguration, next http.Handler, evLogger events.Logger, miscDB *db.NamespacedKV) *basicAuthAndSessionMiddleware {
	return &basicAuthAndSessionMiddleware{
		cookieName: cookieName,
		shortID:    shortID,
		guiCfg:     guiCfg,
		ldapCfg:    ldapCfg,
		next:       next,
		evLogger:   evLogger,
		tokens:     newTokenManager("sessions", miscDB, maxSessionLifetime, maxActiveSessions),
	}
}

func (m *basicAuthAndSessionMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if hasValidAPIKeyHeader(r, m.guiCfg) {
		m.next.ServeHTTP(w, r)
		return
	}

	for _, cookie := range r.Cookies() {
		// We iterate here since there may, historically, be multiple
		// cookies with the same name but different path. Any "old" ones
		// won't match an existing session and will be ignored, then
		// later removed on logout or when timing out.
		if cookie.Name == m.cookieName {
			if m.tokens.Check(cookie.Value) {
				m.next.ServeHTTP(w, r)
				return
			}
		}
	}

	// Fall back to Basic auth if provided
	if username, ok := attemptBasicAuth(r, m.guiCfg, m.ldapCfg, m.evLogger); ok {
		m.createSession(username, false, w, r)
		m.next.ServeHTTP(w, r)
		return
	}

	// Exception for static assets and REST calls that don't require authentication.
	if isNoAuthPath(r.URL.Path) {
		m.next.ServeHTTP(w, r)
		return
	}

	// Some browsers don't send the Authorization request header unless prompted by a 401 response.
	// This enables https://user:pass@localhost style URLs to keep working.
	if m.guiCfg.SendBasicAuthPrompt {
		unauthorized(w, m.shortID)
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
	if err := unmarshalTo(r.Body, &req); err != nil {
		l.Debugln("Failed to parse username and password:", err)
		http.Error(w, "Failed to parse username and password.", http.StatusBadRequest)
		return
	}

	if auth(req.Username, req.Password, m.guiCfg, m.ldapCfg) {
		m.createSession(req.Username, req.StayLoggedIn, w, r)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	emitLoginAttempt(false, req.Username, r.RemoteAddr, m.evLogger)
	antiBruteForceSleep()
	forbidden(w)
}

func attemptBasicAuth(r *http.Request, guiCfg config.GUIConfiguration, ldapCfg config.LDAPConfiguration, evLogger events.Logger) (string, bool) {
	username, password, ok := r.BasicAuth()
	if !ok {
		return "", false
	}

	l.Debugln("Sessionless HTTP request with authentication; this is expensive.")

	if auth(username, password, guiCfg, ldapCfg) {
		return username, true
	}

	usernameFromIso := string(iso88591ToUTF8([]byte(username)))
	passwordFromIso := string(iso88591ToUTF8([]byte(password)))
	if auth(usernameFromIso, passwordFromIso, guiCfg, ldapCfg) {
		return usernameFromIso, true
	}

	emitLoginAttempt(false, username, r.RemoteAddr, evLogger)
	antiBruteForceSleep()
	return "", false
}

func (m *basicAuthAndSessionMiddleware) createSession(username string, persistent bool, w http.ResponseWriter, r *http.Request) {
	sessionid := m.tokens.New()

	// Best effort detection of whether the connection is HTTPS --
	// either directly to us, or as used by the client towards a reverse
	// proxy who sends us headers.
	connectionIsHTTPS := r.TLS != nil ||
		strings.ToLower(r.Header.Get("x-forwarded-proto")) == "https" ||
		strings.Contains(strings.ToLower(r.Header.Get("forwarded")), "proto=https")
	// If the connection is HTTPS, or *should* be HTTPS, set the Secure
	// bit in cookies.
	useSecureCookie := connectionIsHTTPS || m.guiCfg.UseTLS()

	maxAge := 0
	if persistent {
		maxAge = int(maxSessionLifetime.Seconds())
	}
	http.SetCookie(w, &http.Cookie{
		Name:  m.cookieName,
		Value: sessionid,
		// In HTTP spec Max-Age <= 0 means delete immediately,
		// but in http.Cookie MaxAge = 0 means unspecified (session) and MaxAge < 0 means delete immediately
		MaxAge: maxAge,
		Secure: useSecureCookie,
		Path:   "/",
	})

	emitLoginAttempt(true, username, r.RemoteAddr, m.evLogger)
}

func (m *basicAuthAndSessionMiddleware) handleLogout(w http.ResponseWriter, r *http.Request) {
	for _, cookie := range r.Cookies() {
		// We iterate here since there may, historically, be multiple
		// cookies with the same name but different path. We drop them
		// all.
		if cookie.Name == m.cookieName {
			m.tokens.Delete(cookie.Value)

			// Delete the cookie
			http.SetCookie(w, &http.Cookie{
				Name:   m.cookieName,
				Value:  "",
				MaxAge: -1,
				Secure: cookie.Secure,
				Path:   cookie.Path,
			})
		}
	}

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
		l.Warnln("LDAP Dial:", err)
		return false
	}

	if cfg.Transport == config.LDAPTransportStartTLS {
		err = connection.StartTLS(&tls.Config{InsecureSkipVerify: cfg.InsecureSkipVerify})
		if err != nil {
			l.Warnln("LDAP Start TLS:", err)
			return false
		}
	}

	defer connection.Close()

	bindDN := formatOptionalPercentS(cfg.BindDN, escapeForLDAPDN(username))
	err = connection.Bind(bindDN, password)
	if err != nil {
		l.Warnln("LDAP Bind:", err)
		return false
	}

	if cfg.SearchFilter == "" && cfg.SearchBaseDN == "" {
		// We're done here.
		return true
	}

	if cfg.SearchFilter == "" || cfg.SearchBaseDN == "" {
		l.Warnln("LDAP configuration: both searchFilter and searchBaseDN must be set, or neither.")
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
		l.Warnln("LDAP Search:", err)
		return false
	}
	if len(res.Entries) != 1 {
		l.Infof("Wrong number of LDAP search results, %d != 1", len(res.Entries))
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
	for i := 0; i < nReps; i++ {
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

type tokenManager struct {
	key      string
	miscDB   *db.NamespacedKV
	lifetime time.Duration
	maxItems int

	timeNow func() time.Time // can be overridden for testing

	mut       sync.Mutex
	tokens    *TokenSet
	saveTimer *time.Timer
}

func newTokenManager(key string, miscDB *db.NamespacedKV, lifetime time.Duration, maxItems int) *tokenManager {
	tokens := &TokenSet{
		Tokens: make(map[string]int64),
	}
	if bs, ok, _ := miscDB.Bytes(key); ok {
		_ = tokens.Unmarshal(bs) // best effort
	}
	return &tokenManager{
		key:      key,
		miscDB:   miscDB,
		lifetime: lifetime,
		maxItems: maxItems,
		timeNow:  time.Now,
		mut:      sync.NewMutex(),
		tokens:   tokens,
	}
}

// Check returns true if the token is valid, and updates the token's expiry
// time. The token is removed if it is expired.
func (m *tokenManager) Check(token string) bool {
	m.mut.Lock()
	defer m.mut.Unlock()

	expires, ok := m.tokens.Tokens[token]
	if ok {
		if expires < m.timeNow().UnixNano() {
			// The token is expired.
			m.saveLocked() // removes expired tokens
			return false
		}

		// Give the token further life.
		m.tokens.Tokens[token] = m.timeNow().Add(m.lifetime).UnixNano()
		m.saveLocked()
	}
	return ok
}

// New creates a new token and returns it.
func (m *tokenManager) New() string {
	token := rand.String(randomTokenLength)

	m.mut.Lock()
	defer m.mut.Unlock()

	m.tokens.Tokens[token] = m.timeNow().Add(m.lifetime).UnixNano()
	m.saveLocked()

	return token
}

// Delete removes a token.
func (m *tokenManager) Delete(token string) {
	m.mut.Lock()
	defer m.mut.Unlock()

	delete(m.tokens.Tokens, token)
	m.saveLocked()
}

func (m *tokenManager) saveLocked() {
	// Remove expired tokens.
	now := m.timeNow().UnixNano()
	for token, expiry := range m.tokens.Tokens {
		if expiry < now {
			delete(m.tokens.Tokens, token)
		}
	}

	// If we have a limit on the number of tokens, remove the oldest ones.
	if m.maxItems > 0 && len(m.tokens.Tokens) > m.maxItems {
		// Sort the tokens by expiry time, oldest first.
		type tokenExpiry struct {
			token  string
			expiry int64
		}
		var tokens []tokenExpiry
		for token, expiry := range m.tokens.Tokens {
			tokens = append(tokens, tokenExpiry{token, expiry})
		}
		slices.SortFunc(tokens, func(i, j tokenExpiry) int {
			return int(i.expiry - j.expiry)
		})
		// Remove the oldest tokens.
		for _, token := range tokens[:len(tokens)-m.maxItems] {
			delete(m.tokens.Tokens, token.token)
		}
	}

	// Postpone saving until one second of inactivity.
	if m.saveTimer == nil {
		m.saveTimer = time.AfterFunc(time.Second, m.scheduledSave)
	} else {
		m.saveTimer.Reset(time.Second)
	}
}

func (m *tokenManager) scheduledSave() {
	m.mut.Lock()
	defer m.mut.Unlock()

	m.saveTimer = nil

	bs, _ := m.tokens.Marshal()      // can't fail
	_ = m.miscDB.PutBytes(m.key, bs) // can fail, but what are we going to do?
}
