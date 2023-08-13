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

func authFailureSleep() {
	time.Sleep(time.Duration(rand.Intn(100)+100) * time.Millisecond)
}

func unauthorized(w http.ResponseWriter) {
	authFailureSleep()
	w.Header().Set("WWW-Authenticate", "Basic realm=\"Authorization Required\"")
	http.Error(w, "Not Authorized", http.StatusUnauthorized)
}

func forbidden(w http.ResponseWriter) {
	authFailureSleep()
	http.Error(w, "Forbidden", http.StatusForbidden)
}

func equalsAny(s string, values []string) bool {
	for _, value := range values {
		if s == value {
			return true
		}
	}
	return false
}

func hasAnyPrefix(s string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}

func noAuthPaths() []string {
	return []string{
		"/",
		"/index.html",
		"/modal.html",
	}
}

func noAuthPrefixes() []string {
	return []string{
		// Static assets
		"/assets/",
		"/syncthing/",
		"/vendor/",
		"/theme-assets/", // This leaks information from config, but probably not sensitive

		// No-auth API endpoints
		"/rest/noauth",
	}
}

func authAndSessionMiddleware(cookieName string, guiCfg config.GUIConfiguration, ldapCfg config.LDAPConfiguration, next http.Handler, evLogger events.Logger) (http.Handler, http.Handler) {

	handleAuthPassthrough := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if guiCfg.IsValidAPIKey(r.Header.Get("X-API-Key")) {
			next.ServeHTTP(w, r)
			return
		}

		// Exception for static assets and REST calls that don't require authentication.
		if equalsAny(r.URL.Path, noAuthPaths()) || hasAnyPrefix(r.URL.Path, noAuthPrefixes()) {
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

	handlePasswordLogin := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct{Username string; Password string}
		if err := unmarshalTo(r.Body, &req); err != nil {
			l.Debugln("Failed to parse username and password:", err)
			http.Error(w, "Failed to parse username and password.", 400)
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

	return handleAuthPassthrough, handlePasswordLogin
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
		Name:   cookieName,
		Value:  sessionid,
		// In HTTP spec Max-Age <= 0 means delete immediately,
		// but in http.Cookie MaxAge = 0 means unspecified (session) and MaxAge < 0 means delete immediately
		MaxAge: 0,
		Secure: useSecureCookie,
		Path: "/",
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

		http.SetCookie(w, &http.Cookie{
			Name:   cookieName,
			Value:  "",
			MaxAge: -1,
			Secure: true,
			Path: "/",
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

	err = connection.Bind(fmt.Sprintf(cfg.BindDN, username), password)
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

	searchString := fmt.Sprintf(cfg.SearchFilter, username)
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
