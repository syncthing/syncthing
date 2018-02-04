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
		entries: make(map[sessionKey]int64),
	}
)

// a global map for blacklisting conversations
type blacklistMap struct {
	entries map[sessionKey]int64
	mut     sync.RWMutex
}

func (m *blacklistMap) add(address string, conv uint32) {
	if BlacklistDuration == 0 {
		return
	}
	timeout := time.Now().Add(BlacklistDuration).UnixNano()
	m.mut.Lock()
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
	m.mut.RLock()
	t, ok := m.entries[sessionKey{
		addr:   address,
		convID: conv,
	}]
	m.mut.RUnlock()
	return ok && t > time.Now().UnixNano()
}

func (m *blacklistMap) reap() {
	now := time.Now().UnixNano()
	for k, t := range m.entries {
		if t < now {
			delete(m.entries, k)
		}
	}
}
