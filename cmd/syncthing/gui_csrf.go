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
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/syncthing/syncthing/internal/osutil"
)

var csrfTokens []string
var csrfMut sync.Mutex

// Check for CSRF token on /rest/ URLs. If a correct one is not given, reject
// the request with 403. For / and /index.html, set a new CSRF cookie if none
// is currently set.
func csrfMiddleware(prefix, apiKey string, next http.Handler) http.Handler {
	loadCsrfTokens()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Allow requests carrying a valid API key
		if apiKey != "" && r.Header.Get("X-API-Key") == apiKey {
			next.ServeHTTP(w, r)
			return
		}

		// Allow requests for the front page, and set a CSRF cookie if there isn't already a valid one.
		if !strings.HasPrefix(r.URL.Path, prefix) {
			cookie, err := r.Cookie("CSRF-Token")
			if err != nil || !validCsrfToken(cookie.Value) {
				cookie = &http.Cookie{
					Name:  "CSRF-Token",
					Value: newCsrfToken(),
				}
				http.SetCookie(w, cookie)
			}
			next.ServeHTTP(w, r)
			return
		}

		if r.Method == "GET" {
			// Allow GET requests unconditionally
			next.ServeHTTP(w, r)
			return
		}

		// Verify the CSRF token
		token := r.Header.Get("X-CSRF-Token")
		if !validCsrfToken(token) {
			http.Error(w, "CSRF Error", 403)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func validCsrfToken(token string) bool {
	csrfMut.Lock()
	defer csrfMut.Unlock()
	for _, t := range csrfTokens {
		if t == token {
			return true
		}
	}
	return false
}

func newCsrfToken() string {
	token := randomString(30)

	csrfMut.Lock()
	csrfTokens = append(csrfTokens, token)
	if len(csrfTokens) > 10 {
		csrfTokens = csrfTokens[len(csrfTokens)-10:]
	}
	defer csrfMut.Unlock()

	saveCsrfTokens()

	return token
}

func saveCsrfTokens() {
	name := filepath.Join(confDir, "csrftokens.txt")
	tmp := fmt.Sprintf("%s.tmp.%d", name, time.Now().UnixNano())

	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer os.Remove(tmp)

	for _, t := range csrfTokens {
		_, err := fmt.Fprintln(f, t)
		if err != nil {
			return
		}
	}

	err = f.Close()
	if err != nil {
		return
	}

	osutil.Rename(tmp, name)
}

func loadCsrfTokens() {
	name := filepath.Join(confDir, "csrftokens.txt")
	f, err := os.Open(name)
	if err != nil {
		return
	}
	defer f.Close()

	s := bufio.NewScanner(f)
	for s.Scan() {
		csrfTokens = append(csrfTokens, s.Text())
	}
}

func randomString(len int) string {
	bs := make([]byte, len)
	_, err := rand.Reader.Read(bs)
	if err != nil {
		l.Fatalln(err)
	}

	return base64.StdEncoding.EncodeToString(bs)
}
