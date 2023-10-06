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
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/rand"
	"github.com/syncthing/syncthing/lib/sync"
	"golang.org/x/exp/slices"
)

var (
	sessions    = make(map[string]bool)
	sessionsMut = sync.NewMutex()
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

func unauthorized(w http.ResponseWriter) {
	antiBruteForceSleep()
	w.Header().Set("WWW-Authenticate", "Basic realm=\"Authorization Required\"")
	http.Error(w, "Not Authorized", http.StatusUnauthorized)
}

func forbidden(w http.ResponseWriter) {
	antiBruteForceSleep()
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

func basicAuthAndSessionMiddleware(cookieName string, guiCfg config.GUIConfiguration, ldapCfg config.LDAPConfiguration, next http.Handler, evLogger events.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hasValidAPIKeyHeader(r, guiCfg) {
			next.ServeHTTP(w, r)
			return
		}

		// Exception for static assets and REST calls that don't require authentication.
		if isNoAuthPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		cookie, err := r.Cookie(cookieName)
		if err == nil && cookie != nil {
			sessionsMut.Lock()
			_, ok := sessions[cookie.Value]
			sessionsMut.Unlock()
			if ok {
				next.ServeHTTP(w, r)
				return
			}
		}

		// Fall back to Basic auth if provided
		if username, ok := attemptBasicAuth(r, guiCfg, ldapCfg, evLogger); ok {
			createSession(cookieName, username, guiCfg, evLogger, w, r)
			next.ServeHTTP(w, r)
			return
		}

		// Some browsers don't send the Authorization request header unless prompted by a 401 response.
		// This enables https://user:pass@localhost style URLs to keep working.
		if guiCfg.SendBasicAuthPrompt {
			unauthorized(w)
			return
		}

		forbidden(w)
	})
}

func passwordAuthHandler(cookieName string, guiCfg config.GUIConfiguration, ldapCfg config.LDAPConfiguration, evLogger events.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Username string
			Password string
		}
		if err := unmarshalTo(r.Body, &req); err != nil {
			l.Debugln("Failed to parse username and password:", err)
			http.Error(w, "Failed to parse username and password.", http.StatusBadRequest)
			return
		}

		if auth(req.Username, req.Password, guiCfg, ldapCfg) {
			createSession(cookieName, req.Username, guiCfg, evLogger, w, r)
			w.WriteHeader(http.StatusNoContent)
			return
		}

		emitLoginAttempt(false, req.Username, r.RemoteAddr, evLogger)
		forbidden(w)
	})
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
	return "", false
}

func createSession(cookieName string, username string, guiCfg config.GUIConfiguration, evLogger events.Logger, w http.ResponseWriter, r *http.Request) {
	sessionid := rand.String(32)
	sessionsMut.Lock()
	sessions[sessionid] = true
	sessionsMut.Unlock()

	// Best effort detection of whether the connection is HTTPS --
	// either directly to us, or as used by the client towards a reverse
	// proxy who sends us headers.
	connectionIsHTTPS := r.TLS != nil ||
		strings.ToLower(r.Header.Get("x-forwarded-proto")) == "https" ||
		strings.Contains(strings.ToLower(r.Header.Get("forwarded")), "proto=https")
	// If the connection is HTTPS, or *should* be HTTPS, set the Secure
	// bit in cookies.
	useSecureCookie := connectionIsHTTPS || guiCfg.UseTLS()

	http.SetCookie(w, &http.Cookie{
		Name:  cookieName,
		Value: sessionid,
		// In HTTP spec Max-Age <= 0 means delete immediately,
		// but in http.Cookie MaxAge = 0 means unspecified (session) and MaxAge < 0 means delete immediately
		MaxAge: 0,
		Secure: useSecureCookie,
		Path:   "/",
	})

	emitLoginAttempt(true, username, r.RemoteAddr, evLogger)
}

func handleLogout(cookieName string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(cookieName)
		if err == nil && cookie != nil {
			sessionsMut.Lock()
			delete(sessions, cookie.Value)
			sessionsMut.Unlock()
		}
		// else: If there is no session cookie, that's also a successful logout in terms of user experience.

		http.SetCookie(w, &http.Cookie{
			Name:   cookieName,
			Value:  "",
			MaxAge: -1,
			Secure: true,
			Path:   "/",
		})
		w.WriteHeader(http.StatusNoContent)
	})
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

// escapeForLDAPFilter escapes a value that will be used in a filter clause
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
