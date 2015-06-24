// Copyright (C) 2015 Audrius Butkevicius and Contributors (see the CONTRIBUTORS file).

package main

import (
	"crypto/rand"
	"net"
	"sync"
	"time"

	"github.com/syncthing/relaysrv/protocol"
)

var (
	sessionmut = sync.Mutex{}
	sessions   = make(map[string]*session, 0)
)

type session struct {
	serverkey string
	clientkey string

	mut   sync.RWMutex
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

	return &session{
		serverkey: string(serverkey),
		clientkey: string(clientkey),
		conns:     make(chan net.Conn),
	}
}

func findSession(key string) *session {
	sessionmut.Lock()
	defer sessionmut.Unlock()
	lob, ok := sessions[key]
	if !ok {
		return nil

	}
	delete(sessions, key)
	return lob
}

func (l *session) AddConnection(conn net.Conn) {
	select {
	case l.conns <- conn:
	default:
	}
}

func (l *session) Serve() {

	timedout := time.After(messageTimeout)

	sessionmut.Lock()
	sessions[l.serverkey] = l
	sessions[l.clientkey] = l
	sessionmut.Unlock()

	conns := make([]net.Conn, 0, 2)
	for {
		select {
		case conn := <-l.conns:
			conns = append(conns, conn)
			if len(conns) < 2 {
				continue
			}

			close(l.conns)

			wg := sync.WaitGroup{}

			wg.Add(2)

			go proxy(conns[0], conns[1], wg)
			go proxy(conns[1], conns[0], wg)

			wg.Wait()

			break
		case <-timedout:
			sessionmut.Lock()
			delete(sessions, l.serverkey)
			delete(sessions, l.clientkey)
			sessionmut.Unlock()

			for _, conn := range conns {
				conn.Close()
			}

			break
		}
	}
}

func (l *session) GetClientInvitationMessage() (message, error) {
	invitation := protocol.SessionInvitation{
		Key:          []byte(l.clientkey),
		Address:      nil,
		Port:         123,
		ServerSocket: false,
	}
	data, err := invitation.MarshalXDR()
	if err != nil {
		return message{}, err
	}

	return message{
		header: protocol.Header{
			Magic:         protocol.Magic,
			MessageType:   protocol.MessageTypeSessionInvitation,
			MessageLength: int32(len(data)),
		},
		payload: data,
	}, nil
}

func (l *session) GetServerInvitationMessage() (message, error) {
	invitation := protocol.SessionInvitation{
		Key:          []byte(l.serverkey),
		Address:      nil,
		Port:         123,
		ServerSocket: true,
	}
	data, err := invitation.MarshalXDR()
	if err != nil {
		return message{}, err
	}

	return message{
		header: protocol.Header{
			Magic:         protocol.Magic,
			MessageType:   protocol.MessageTypeSessionInvitation,
			MessageLength: int32(len(data)),
		},
		payload: data,
	}, nil
}

func proxy(c1, c2 net.Conn, wg sync.WaitGroup) {
	for {
		buf := make([]byte, 1024)
		c1.SetReadDeadline(time.Now().Add(networkTimeout))
		n, err := c1.Read(buf)
		if err != nil {
			break
		}

		c2.SetWriteDeadline(time.Now().Add(networkTimeout))
		_, err = c2.Write(buf[:n])
		if err != nil {
			break
		}
	}
	c1.Close()
	c2.Close()
	wg.Done()
}
