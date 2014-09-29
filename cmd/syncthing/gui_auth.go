// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

package main

import (
	"bytes"
	"encoding/base64"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	"code.google.com/p/go.crypto/bcrypt"
	"github.com/syncthing/syncthing/internal/config"
)

var (
	sessions    = make(map[string]bool)
	sessionsMut sync.Mutex
)

func basicAuthAndSessionMiddleware(cfg config.GUIConfiguration, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if cfg.APIKey != "" && r.Header.Get("X-API-Key") == cfg.APIKey {
			next.ServeHTTP(w, r)
			return
		}

		cookie, err := r.Cookie("sessionid")
		if err == nil && cookie != nil {
			sessionsMut.Lock()
			_, ok := sessions[cookie.Value]
			sessionsMut.Unlock()
			if ok {
				next.ServeHTTP(w, r)
				return
			}
		}

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

		if string(fields[0]) != cfg.User {
			error()
			return
		}

		if err := bcrypt.CompareHashAndPassword([]byte(cfg.Password), fields[1]); err != nil {
			error()
			return
		}

		sessionid := randomString(32)
		sessionsMut.Lock()
		sessions[sessionid] = true
		sessionsMut.Unlock()
		http.SetCookie(w, &http.Cookie{
			Name:   "sessionid",
			Value:  sessionid,
			MaxAge: 0,
		})

		next.ServeHTTP(w, r)
	})
}
