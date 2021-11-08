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

func basicAuthAndSessionMiddleware(cookieName string, guiCfg config.GUIConfiguration, ldapCfg config.LDAPConfiguration, next http.Handler, evLogger events.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if guiCfg.IsValidAPIKey(r.Header.Get("X-API-Key")) {
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

		l.Debugln("Sessionless HTTP request with authentication; this is expensive.")

		error := func() {
			time.Sleep(time.Duration(rand.Intn(100)+100) * time.Millisecond)
			w.Header().Set("WWW-Authenticate", "Basic realm=\"Authorization Required\"")
			http.Error(w, "Not Authorized", http.StatusUnauthorized)
		}

		username, password, ok := r.BasicAuth()
		if !ok {
			error()
			return
		}

		authOk := auth(username, password, guiCfg, ldapCfg)
		if !authOk {
			usernameIso := string(iso88591ToUTF8([]byte(username)))
			passwordIso := string(iso88591ToUTF8([]byte(password)))
			authOk = auth(usernameIso, passwordIso, guiCfg, ldapCfg)
			if authOk {
				username = usernameIso
			}
		}

		if !authOk {
			emitLoginAttempt(false, username, r.RemoteAddr, evLogger)
			error()
			return
		}

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
			MaxAge: 0,
			Secure: useSecureCookie,
		})

		emitLoginAttempt(true, username, r.RemoteAddr, evLogger)
		next.ServeHTTP(w, r)
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
