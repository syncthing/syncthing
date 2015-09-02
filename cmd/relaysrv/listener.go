// Copyright (C) 2015 Audrius Butkevicius and Contributors (see the CONTRIBUTORS file).

package main

import (
	"crypto/tls"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	syncthingprotocol "github.com/syncthing/protocol"
	"github.com/syncthing/syncthing/lib/tlsutil"

	"github.com/syncthing/relaysrv/protocol"
)

var (
	outboxesMut    = sync.RWMutex{}
	outboxes       = make(map[syncthingprotocol.DeviceID]chan interface{})
	numConnections int64
)

func listener(addr string, config *tls.Config) {
	tcpListener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalln(err)
	}

	listener := tlsutil.DowngradingListener{tcpListener, nil}

	for {
		conn, isTLS, err := listener.AcceptNoWrap()
		if err != nil {
			if debug {
				log.Println(err)
			}
			continue
		}

		setTCPOptions(conn)

		if debug {
			log.Println("Listener accepted connection from", conn.RemoteAddr(), "tls", isTLS)
		}

		if isTLS {
			go protocolConnectionHandler(conn, config)
		} else {
			go sessionConnectionHandler(conn)
		}

	}
}

func protocolConnectionHandler(tcpConn net.Conn, config *tls.Config) {
	conn := tls.Server(tcpConn, config)
	err := conn.Handshake()
	if err != nil {
		if debug {
			log.Println("Protocol connection TLS handshake:", conn.RemoteAddr(), err)
		}
		conn.Close()
		return
	}

	state := conn.ConnectionState()
	if (!state.NegotiatedProtocolIsMutual || state.NegotiatedProtocol != protocol.ProtocolName) && debug {
		log.Println("Protocol negotiation error")
	}

	certs := state.PeerCertificates
	if len(certs) != 1 {
		if debug {
			log.Println("Certificate list error")
		}
		conn.Close()
		return
	}

	id := syncthingprotocol.NewDeviceID(certs[0].Raw)

	messages := make(chan interface{})
	errors := make(chan error, 1)
	outbox := make(chan interface{})

	// Read messages from the connection and send them on the messages
	// channel. When there is an error, send it on the error channel and
	// return. Applies also when the connection gets closed, so the pattern
	// below is to close the connection on error, then wait for the error
	// signal from messageReader to exit.
	go messageReader(conn, messages, errors)

	pingTicker := time.NewTicker(pingInterval)
	timeoutTicker := time.NewTimer(networkTimeout)
	joined := false

	for {
		select {
		case message := <-messages:
			timeoutTicker.Reset(networkTimeout)
			if debug {
				log.Printf("Message %T from %s", message, id)
			}

			switch msg := message.(type) {
			case protocol.JoinRelayRequest:
				outboxesMut.RLock()
				_, ok := outboxes[id]
				outboxesMut.RUnlock()
				if ok {
					protocol.WriteMessage(conn, protocol.ResponseAlreadyConnected)
					if debug {
						log.Println("Already have a peer with the same ID", id, conn.RemoteAddr())
					}
					conn.Close()
					continue
				}

				outboxesMut.Lock()
				outboxes[id] = outbox
				outboxesMut.Unlock()
				joined = true

				protocol.WriteMessage(conn, protocol.ResponseSuccess)

			case protocol.ConnectRequest:
				requestedPeer := syncthingprotocol.DeviceIDFromBytes(msg.ID)
				outboxesMut.RLock()
				peerOutbox, ok := outboxes[requestedPeer]
				outboxesMut.RUnlock()
				if !ok {
					if debug {
						log.Println(id, "is looking for", requestedPeer, "which does not exist")
					}
					protocol.WriteMessage(conn, protocol.ResponseNotFound)
					conn.Close()
					continue
				}

				ses := newSession(sessionLimiter, globalLimiter)

				go ses.Serve()

				clientInvitation := ses.GetClientInvitationMessage(requestedPeer)
				serverInvitation := ses.GetServerInvitationMessage(id)

				if err := protocol.WriteMessage(conn, clientInvitation); err != nil {
					if debug {
						log.Printf("Error sending invitation from %s to client: %s", id, err)
					}
					conn.Close()
					continue
				}

				peerOutbox <- serverInvitation

				if debug {
					log.Println("Sent invitation from", id, "to", requestedPeer)
				}
				conn.Close()

			case protocol.Pong:
				// Nothing

			default:
				if debug {
					log.Printf("Unknown message %s: %T", id, message)
				}
				protocol.WriteMessage(conn, protocol.ResponseUnexpectedMessage)
				conn.Close()
			}

		case err := <-errors:
			if debug {
				log.Printf("Closing connection %s: %s", id, err)
			}
			close(outbox)

			// Potentially closing a second time.
			conn.Close()

			// Only delete the outbox if the client is joined, as it might be
			// a lookup request coming from the same client.
			if joined {
				outboxesMut.Lock()
				delete(outboxes, id)
				outboxesMut.Unlock()
			}
			return

		case <-pingTicker.C:
			if !joined {
				if debug {
					log.Println(id, "didn't join within", pingInterval)
				}
				conn.Close()
				continue
			}

			if err := protocol.WriteMessage(conn, protocol.Ping{}); err != nil {
				if debug {
					log.Println(id, err)
				}
				conn.Close()
			}

		case <-timeoutTicker.C:
			// We should receive a error from the reader loop, which will cause
			// us to quit this loop.
			if debug {
				log.Printf("%s timed out", id)
			}
			conn.Close()

		case msg := <-outbox:
			if debug {
				log.Printf("Sending message %T to %s", msg, id)
			}
			if err := protocol.WriteMessage(conn, msg); err != nil {
				if debug {
					log.Println(id, err)
				}
				conn.Close()
			}
		}
	}
}

func sessionConnectionHandler(conn net.Conn) {
	if err := conn.SetDeadline(time.Now().Add(messageTimeout)); err != nil {
		if debug {
			log.Println("Weird error setting deadline:", err, "on", conn.RemoteAddr())
		}
		return
	}

	message, err := protocol.ReadMessage(conn)
	if err != nil {
		return
	}

	switch msg := message.(type) {
	case protocol.JoinSessionRequest:
		ses := findSession(string(msg.Key))
		if debug {
			log.Println(conn.RemoteAddr(), "session lookup", ses)
		}

		if ses == nil {
			protocol.WriteMessage(conn, protocol.ResponseNotFound)
			conn.Close()
			return
		}

		if !ses.AddConnection(conn) {
			if debug {
				log.Println("Failed to add", conn.RemoteAddr(), "to session", ses)
			}
			protocol.WriteMessage(conn, protocol.ResponseAlreadyConnected)
			conn.Close()
			return
		}

		if err := protocol.WriteMessage(conn, protocol.ResponseSuccess); err != nil {
			if debug {
				log.Println("Failed to send session join response to ", conn.RemoteAddr(), "for", ses)
			}
			return
		}

		if err := conn.SetDeadline(time.Time{}); err != nil {
			if debug {
				log.Println("Weird error setting deadline:", err, "on", conn.RemoteAddr())
			}
			conn.Close()
			return
		}

	default:
		if debug {
			log.Println("Unexpected message from", conn.RemoteAddr(), message)
		}
		protocol.WriteMessage(conn, protocol.ResponseUnexpectedMessage)
		conn.Close()
	}
}

func messageReader(conn net.Conn, messages chan<- interface{}, errors chan<- error) {
	atomic.AddInt64(&numConnections, 1)
	defer atomic.AddInt64(&numConnections, -1)

	for {
		msg, err := protocol.ReadMessage(conn)
		if err != nil {
			errors <- err
			return
		}
		messages <- msg
	}
}
