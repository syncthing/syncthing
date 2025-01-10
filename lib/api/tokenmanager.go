// Copyright (C) 2024 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package api

import (
	"net/http"
	"slices"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/syncthing/syncthing/internal/gen/apiproto"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/rand"
	"github.com/syncthing/syncthing/lib/sync"
)

type tokenManager struct {
	key      string
	miscDB   *db.NamespacedKV
	lifetime time.Duration
	maxItems int

	timeNow func() time.Time // can be overridden for testing

	mut       sync.Mutex
	tokens    *apiproto.TokenSet
	saveTimer *time.Timer
}

func newTokenManager(key string, miscDB *db.NamespacedKV, lifetime time.Duration, maxItems int) *tokenManager {
	var tokens apiproto.TokenSet
	if bs, ok, _ := miscDB.Bytes(key); ok {
		_ = proto.Unmarshal(bs, &tokens) // best effort
	}
	if tokens.Tokens == nil {
		tokens.Tokens = make(map[string]int64)
	}
	return &tokenManager{
		key:      key,
		miscDB:   miscDB,
		lifetime: lifetime,
		maxItems: maxItems,
		timeNow:  time.Now,
		mut:      sync.NewMutex(),
		tokens:   &tokens,
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

	bs, _ := proto.Marshal(m.tokens) // can't fail
	_ = m.miscDB.PutBytes(m.key, bs) // can fail, but what are we going to do?
}

type tokenCookieManager struct {
	cookieName string
	shortID    string
	guiCfg     config.GUIConfiguration
	evLogger   events.Logger
	tokens     *tokenManager
}

func newTokenCookieManager(shortID string, guiCfg config.GUIConfiguration, evLogger events.Logger, miscDB *db.NamespacedKV) *tokenCookieManager {
	return &tokenCookieManager{
		cookieName: "sessionid-" + shortID,
		shortID:    shortID,
		guiCfg:     guiCfg,
		evLogger:   evLogger,
		tokens:     newTokenManager("sessions", miscDB, maxSessionLifetime, maxActiveSessions),
	}
}

func (m *tokenCookieManager) createSession(username string, persistent bool, w http.ResponseWriter, r *http.Request) {
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

func (m *tokenCookieManager) hasValidSession(r *http.Request) bool {
	for _, cookie := range r.Cookies() {
		// We iterate here since there may, historically, be multiple
		// cookies with the same name but different path. Any "old" ones
		// won't match an existing session and will be ignored, then
		// later removed on logout or when timing out.
		if cookie.Name == m.cookieName {
			if m.tokens.Check(cookie.Value) {
				return true
			}
		}
	}
	return false
}

func (m *tokenCookieManager) destroySession(w http.ResponseWriter, r *http.Request) {
	for _, cookie := range r.Cookies() {
		// We iterate here since there may, historically, be multiple
		// cookies with the same name but different path. We drop them
		// all.
		if cookie.Name == m.cookieName {
			m.tokens.Delete(cookie.Value)

			// Create a cookie deletion command
			http.SetCookie(w, &http.Cookie{
				Name:   m.cookieName,
				Value:  "",
				MaxAge: -1,
				Secure: cookie.Secure,
				Path:   cookie.Path,
			})
		}
	}
}
