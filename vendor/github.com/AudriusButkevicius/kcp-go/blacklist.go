package kcp

import (
	"sync"
	"time"
)

var (
	BlacklistDuration time.Duration
	blacklist         = blacklistMap{
		entries: make(map[sessionKey]time.Time),
	}
)

// a global map for blacklisting conversations
type blacklistMap struct {
	entries map[sessionKey]time.Time
	reapAt  time.Time
	mut     sync.Mutex
}

func (m *blacklistMap) add(address string, conv uint32) {
	if BlacklistDuration == 0 {
		return
	}
	m.mut.Lock()
	timeout := time.Now().Add(BlacklistDuration)
	m.entries[sessionKey{
		addr:   address,
		convID: conv,
	}] = timeout
	m.reap()
	m.mut.Unlock()
}

func (m *blacklistMap) has(address string, conv uint32) bool {
	if BlacklistDuration == 0 {
		return false
	}
	m.mut.Lock()
	t, ok := m.entries[sessionKey{
		addr:   address,
		convID: conv,
	}]
	m.mut.Lock()
	return ok && t.After(time.Now())
}

func (m *blacklistMap) reap() {
	now := time.Now()
	for k, t := range m.entries {
		if t.Before(now) {
			delete(m.entries, k)
		}
	}
}
