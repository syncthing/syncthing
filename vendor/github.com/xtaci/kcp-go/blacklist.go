package kcp

import (
	"sync"
	"time"
)

var (
	// BlacklistDuration sets a duration for which a session is blacklisted
	// once it's established. This is simillar to TIME_WAIT state in TCP, whereby
	// any connection attempt with the same session parameters is ignored for
	// some amount of time.
	//
	// This is only useful when dial attempts happen from a pre-determined port,
	// for example when you are dialing from the same connection you are listening on
	// to punch through NAT, and helps with the fact that KCP is state-less.
	// This helps better deal with scenarios where a process on one of the side (A)
	// get's restarted, and stray packets from other side (B) makes it look like
	// as if someone is trying to connect to A. Even if session dies on B,
	// new stray reply packets from A resurrect the session on B, causing the
	// session to be alive forever.
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
	m.mut.Unlock()
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
