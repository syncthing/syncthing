// Copyright (C) 2015 Audrius Butkevicius and Contributors (see the CONTRIBUTORS file).

package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/syncthing/relaysrv/protocol"

	syncthingprotocol "github.com/syncthing/protocol"
)

var (
	sessionMut = sync.Mutex{}
	sessions   = make(map[string]*session, 0)
)

type session struct {
	serverkey []byte
	clientkey []byte

	conns chan net.Conn
}

func newSession() *session {
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
		clientkey: clientkey,
		conns:     make(chan net.Conn),
	}

	if debug {
		log.Println("New session", ses)
	}

	sessionMut.Lock()
	sessions[string(ses.serverkey)] = ses
	sessions[string(ses.clientkey)] = ses
	sessionMut.Unlock()

	return ses
}

func findSession(key string) *session {
	sessionMut.Lock()
	defer sessionMut.Unlock()
	lob, ok := sessions[key]
	if !ok {
		return nil

	}
	delete(sessions, key)
	return lob
}

func (s *session) AddConnection(conn net.Conn) bool {
	if debug {
		log.Println("New connection for", s, "from", conn.RemoteAddr())
	}

	select {
	case s.conns <- conn:
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

	conns := make([]net.Conn, 0, 2)
	for {
		select {
		case conn := <-s.conns:
			conns = append(conns, conn)
			if len(conns) < 2 {
				continue
			}

			close(s.conns)

			if debug {
				log.Println("Session", s, "starting between", conns[0].RemoteAddr(), conns[1].RemoteAddr())
			}

			wg := sync.WaitGroup{}
			wg.Add(2)

			errors := make(chan error, 2)

			go func() {
				errors <- proxy(conns[0], conns[1])
				wg.Done()
			}()

			go func() {
				errors <- proxy(conns[1], conns[0])
				wg.Done()
			}()

			wg.Wait()

			if debug {
				log.Println("Session", s, "ended, outcomes:", <-errors, <-errors)
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
	sessionMut.Lock()
	delete(sessions, string(s.serverkey))
	delete(sessions, string(s.clientkey))
	sessionMut.Unlock()

	for _, conn := range conns {
		conn.Close()
	}

	if debug {
		log.Println("Session", s, "stopping")
	}
}

func (s *session) GetClientInvitationMessage(from syncthingprotocol.DeviceID) protocol.SessionInvitation {
	return protocol.SessionInvitation{
		From:         from[:],
		Key:          []byte(s.clientkey),
		Address:      sessionAddress,
		Port:         sessionPort,
		ServerSocket: false,
	}
}

func (s *session) GetServerInvitationMessage(from syncthingprotocol.DeviceID) protocol.SessionInvitation {
	return protocol.SessionInvitation{
		From:         from[:],
		Key:          []byte(s.serverkey),
		Address:      sessionAddress,
		Port:         sessionPort,
		ServerSocket: true,
	}
}

func proxy(c1, c2 net.Conn) error {
	if debug {
		log.Println("Proxy", c1.RemoteAddr(), "->", c2.RemoteAddr())
	}
	buf := make([]byte, 1024)
	for {
		c1.SetReadDeadline(time.Now().Add(networkTimeout))
		n, err := c1.Read(buf[0:])
		if err != nil {
			return err
		}

		if debug {
			log.Printf("%d bytes from %s to %s", n, c1.RemoteAddr(), c2.RemoteAddr())
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
