// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"bytes"
	"encoding/base64"
	"net/http"
	"strings"
	"time"
	"fmt"
	"log"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/rand"
	"github.com/syncthing/syncthing/lib/sync"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/ldap.v2"
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

func basicAuthAndSessionMiddleware(cookieName string, cfg config.GUIConfiguration, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if cfg.IsValidAPIKey(r.Header.Get("X-API-Key")) {
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

		httpl.Debugln("Sessionless HTTP request with authentication; this is expensive.")

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

        authOk := false
        username := string(fields[0])
        password := string(fields[1])
        if cfg.IsAuthModeLdap() {
            authOk = AuthLdap(username, password, cfg.LdapServer, cfg.LdapPort, cfg.LdapBindDn)
        } else {
            authOk, username = AuthSimple(username, password, cfg.User, cfg.Password)
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

func AuthSimple(username string, password string, configUser string, configPassword string) (bool, string) {

    if username != configUser {
        username = string(iso88591ToUTF8([]byte(username)))
        if username != configUser {
            return false, ""
        }
    }

    configPasswordBytes := []byte(configPassword)
    passwordBytes := []byte(password)
    if err := bcrypt.CompareHashAndPassword(configPasswordBytes, passwordBytes); err != nil {
        passwordBytes = iso88591ToUTF8(passwordBytes)
        if err := bcrypt.CompareHashAndPassword(configPasswordBytes, passwordBytes); err != nil {
            return false, ""
        }
    }

    return true, username
}

func AuthLdap(username string, password string, server string, port int, bindDn string) bool {
    l, err := ldap.Dial("tcp", fmt.Sprintf( "%s:%d", server, port))
    if err != nil {
        log.Println(err)
        return false
    }

    defer l.Close()

    err = l.Bind(fmt.Sprintf(bindDn, username), password)
    if err != nil {
        log.Println(err)
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
