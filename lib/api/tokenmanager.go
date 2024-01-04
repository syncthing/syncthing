// Copyright (C) 2024 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package api

import (
	"time"

	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/rand"
	"github.com/syncthing/syncthing/lib/sync"
	"golang.org/x/exp/slices"
)

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
