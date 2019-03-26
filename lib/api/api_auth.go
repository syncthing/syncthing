// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package api

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/rand"
	"github.com/syncthing/syncthing/lib/sync"
	"golang.org/x/crypto/bcrypt"
	ldap "gopkg.in/ldap.v2"
)

var (
	sessions    = make(map[string]bool)
	sessionsMut = sync.NewMutex()
)

func emitLoginAttempt(success bool, username string) {
	events.Default.Log(events.LoginAttempt, map[string]interface{}{
		"success":  success,
		"username": username,
	})
}

func basicAuthAndSessionMiddleware(cookieName string, guiCfg config.GUIConfiguration, ldapCfg config.LDAPConfiguration, next http.Handler) http.Handler {
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

		hdr := r.Header.Get("Authorization")
		if !strings.HasPrefix(hdr, "Basic ") {
			error()
			return
		}

		hdr = hdr[6:]
		bs, err := base64.StdEncoding.DecodeString(hdr)
		if err != nil {
			error()
			return
		}

		fields := bytes.SplitN(bs, []byte(":"), 2)
		if len(fields) != 2 {
			error()
			return
		}

		username := string(fields[0])
		password := string(fields[1])

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
			emitLoginAttempt(false, username)
			error()
			return
		}

		sessionid := rand.String(32)
		sessionsMut.Lock()
		sessions[sessionid] = true
		sessionsMut.Unlock()
		http.SetCookie(w, &http.Cookie{
			Name:   cookieName,
			Value:  sessionid,
			MaxAge: 0,
		})

		emitLoginAttempt(true, username)
		next.ServeHTTP(w, r)
	})
}

func auth(username string, password string, guiCfg config.GUIConfiguration, ldapCfg config.LDAPConfiguration) bool {
	if guiCfg.AuthMode == config.AuthModeLDAP {
		return authLDAP(username, password, ldapCfg)
	} else {
		return authStatic(username, password, guiCfg.User, guiCfg.Password)
	}
}

func authStatic(username string, password string, configUser string, configPassword string) bool {
	configPasswordBytes := []byte(configPassword)
	passwordBytes := []byte(password)
	return bcrypt.CompareHashAndPassword(configPasswordBytes, passwordBytes) == nil && username == configUser
}

func authLDAP(username string, password string, cfg config.LDAPConfiguration) bool {
	address := cfg.Address
	var connection *ldap.Conn
	var err error
	if cfg.Transport == config.LDAPTransportTLS {
		connection, err = ldap.DialTLS("tcp", address, &tls.Config{InsecureSkipVerify: cfg.InsecureSkipVerify})
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
