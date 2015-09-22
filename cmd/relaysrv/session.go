// Copyright (C) 2015 Audrius Butkevicius and Contributors (see the CONTRIBUTORS file).

package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/juju/ratelimit"
	"github.com/syncthing/syncthing/lib/relay/protocol"

	syncthingprotocol "github.com/syncthing/syncthing/lib/protocol"
)

var (
	sessionMut      = sync.RWMutex{}
	activeSessions  = make([]*session, 0)
	pendingSessions = make(map[string]*session, 0)
	numProxies      int64
	bytesProxied    int64
)

func newSession(serverid, clientid syncthingprotocol.DeviceID, sessionRateLimit, globalRateLimit *ratelimit.Bucket) *session {
	serverkey := make([]byte, 32)
	_, err := rand.Read(serverkey)
	if err != nil {
		return nil
	}

	clientkey := make([]byte, 32)
	_, err = rand.Read(clientkey)
	if err != nil {
		return nil
	}

	ses := &session{
		serverkey: serverkey,
		serverid:  serverid,
		clientkey: clientkey,
		clientid:  clientid,
		rateLimit: makeRateLimitFunc(sessionRateLimit, globalRateLimit),
		connsChan: make(chan net.Conn),
		conns:     make([]net.Conn, 0, 2),
	}

	if debug {
		log.Println("New session", ses)
	}

	sessionMut.Lock()
	pendingSessions[string(ses.serverkey)] = ses
	pendingSessions[string(ses.clientkey)] = ses
	sessionMut.Unlock()

	return ses
}

func findSession(key string) *session {
	sessionMut.Lock()
	defer sessionMut.Unlock()
	ses, ok := pendingSessions[key]
	if !ok {
		return nil

	}
	delete(pendingSessions, key)
	return ses
}

func dropSessions(id syncthingprotocol.DeviceID) {
	sessionMut.RLock()
	for _, session := range activeSessions {
		if session.HasParticipant(id) {
			if debug {
				log.Println("Dropping session", session, "involving", id)
			}
			session.CloseConns()
		}
	}
	sessionMut.RUnlock()
}

type session struct {
	mut sync.Mutex

	serverkey []byte
	serverid  syncthingprotocol.DeviceID

	clientkey []byte
	clientid  syncthingprotocol.DeviceID

	rateLimit func(bytes int64)

	connsChan chan net.Conn
	conns     []net.Conn
}

func (s *session) AddConnection(conn net.Conn) bool {
	if debug {
		log.Println("New connection for", s, "from", conn.RemoteAddr())
	}

	select {
	case s.connsChan <- conn:
		return true
	default:
	}
	return false
}

func (s *session) Serve() {
	timedout := time.After(messageTimeout)

	if debug {
		log.Println("Session", s, "serving")
	}

	for {
		select {
		case conn := <-s.connsChan:
			s.mut.Lock()
			s.conns = append(s.conns, conn)
			s.mut.Unlock()
			// We're the only ones mutating% s.conns, hence we are free to read it.
			if len(s.conns) < 2 {
				continue
			}

			close(s.connsChan)

			if debug {
				log.Println("Session", s, "starting between", s.conns[0].RemoteAddr(), "and", s.conns[1].RemoteAddr())
			}

			wg := sync.WaitGroup{}
			wg.Add(2)

			var err0 error
			go func() {
				err0 = s.proxy(s.conns[0], s.conns[1])
				wg.Done()
			}()

			var err1 error
			go func() {
				err1 = s.proxy(s.conns[1], s.conns[0])
				wg.Done()
			}()

			sessionMut.Lock()
			activeSessions = append(activeSessions, s)
			sessionMut.Unlock()

			wg.Wait()

			if debug {
				log.Println("Session", s, "ended, outcomes:", err0, "and", err1)
			}
			goto done

		case <-timedout:
			if debug {
				log.Println("Session", s, "timed out")
			}
			goto done
		}
	}
done:
	// We can end up here in 3 cases:
	// 1. Timeout joining, in which case there are potentially entries in pendingSessions
	// 2. General session end/timeout, in which case there are entries in activeSessions
	// 3. Protocol handler calls dropSession as one of it's clients disconnects.

	sessionMut.Lock()
	delete(pendingSessions, string(s.serverkey))
	delete(pendingSessions, string(s.clientkey))

	for i, session := range activeSessions {
		if session == s {
			l := len(activeSessions) - 1
			activeSessions[i] = activeSessions[l]
			activeSessions[l] = nil
			activeSessions = activeSessions[:l]
		}
	}
	sessionMut.Unlock()

	// If we are here because of case 2 or 3, we are potentially closing some or
	// all connections a second time.
	s.CloseConns()

	if debug {
		log.Println("Session", s, "stopping")
	}
}

func (s *session) GetClientInvitationMessage() protocol.SessionInvitation {
	return protocol.SessionInvitation{
		From:         s.serverid[:],
		Key:          []byte(s.clientkey),
		Address:      sessionAddress,
		Port:         sessionPort,
		ServerSocket: false,
	}
}

func (s *session) GetServerInvitationMessage() protocol.SessionInvitation {
	return protocol.SessionInvitation{
		From:         s.clientid[:],
		Key:          []byte(s.serverkey),
		Address:      sessionAddress,
		Port:         sessionPort,
		ServerSocket: true,
	}
}

func (s *session) HasParticipant(id syncthingprotocol.DeviceID) bool {
	return s.clientid == id || s.serverid == id
}

func (s *session) CloseConns() {
	s.mut.Lock()
	for _, conn := range s.conns {
		conn.Close()
	}
	s.mut.Unlock()
}

func (s *session) proxy(c1, c2 net.Conn) error {
	if debug {
		log.Println("Proxy", c1.RemoteAddr(), "->", c2.RemoteAddr())
	}

	atomic.AddInt64(&numProxies, 1)
	defer atomic.AddInt64(&numProxies, -1)

	buf := make([]byte, 65536)
	for {
		c1.SetReadDeadline(time.Now().Add(networkTimeout))
		n, err := c1.Read(buf)
		if err != nil {
			return err
		}

		atomic.AddInt64(&bytesProxied, int64(n))

		if debug {
			log.Printf("%d bytes from %s to %s", n, c1.RemoteAddr(), c2.RemoteAddr())
		}

		if s.rateLimit != nil {
			s.rateLimit(int64(n))
		}

		c2.SetWriteDeadline(time.Now().Add(networkTimeout))
		_, err = c2.Write(buf[:n])
		if err != nil {
			return err
		}
	}
}

func (s *session) String() string {
	return fmt.Sprintf("<%s/%s>", hex.EncodeToString(s.clientkey)[:5], hex.EncodeToString(s.serverkey)[:5])
}

func makeRateLimitFunc(sessionRateLimit, globalRateLimit *ratelimit.Bucket) func(int64) {
	// This may be a case of super duper premature optimization... We build an
	// optimized function to do the rate limiting here based on what we need
	// to do and then use it in the loop.

	if sessionRateLimit == nil && globalRateLimit == nil {
		// No limiting needed. We could equally well return a func(int64){} and
		// not do a nil check were we use it, but I think the nil check there
		// makes it clear that there will be no limiting if none is
		// configured...
		return nil
	}

	if sessionRateLimit == nil {
		// We only have a global limiter
		return func(bytes int64) {
			globalRateLimit.Wait(bytes)
		}
	}

	if globalRateLimit == nil {
		// We only have a session limiter
		return func(bytes int64) {
			sessionRateLimit.Wait(bytes)
		}
	}

	// We have both. Queue the bytes on both the global and session specific
	// rate limiters. Wait for both in parallell, so that the actual send
	// happens when both conditions are satisfied. In practice this just means
	// wait the longer of the two times.
	return func(bytes int64) {
		t0 := sessionRateLimit.Take(bytes)
		t1 := globalRateLimit.Take(bytes)
		if t0 > t1 {
			time.Sleep(t0)
		} else {
			time.Sleep(t1)
		}
	}
}
