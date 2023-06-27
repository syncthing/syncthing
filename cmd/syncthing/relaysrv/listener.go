// Copyright (C) 2015 Audrius Butkevicius and Contributors.

package relaysrv

import (
	"crypto/tls"
	"encoding/hex"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	syncthingprotocol "github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/tlsutil"

	"github.com/syncthing/syncthing/lib/relay/protocol"
)

var (
	outboxesMut    = sync.RWMutex{}
	outboxes       = make(map[syncthingprotocol.DeviceID]chan interface{})
	numConnections atomic.Int64
)

func listener(_, addr string, config *tls.Config, token string, messageTimeout, networkTimeout, pingInterval time.Duration, networkBufferSize int) {
	tcpListener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalln(err)
	}

	listener := tlsutil.DowngradingListener{
		Listener: tcpListener,
	}

	for {
		conn, isTLS, err := listener.AcceptNoWrapTLS()
		if err != nil {
			if debug {
				log.Println("Listener failed to accept connection from", conn.RemoteAddr(), ". Possibly a TCP Ping.")
			}
			continue
		}

		setTCPOptions(conn, networkTimeout)

		if debug {
			log.Println("Listener accepted connection from", conn.RemoteAddr(), "tls", isTLS)
		}

		if isTLS {
			go protocolConnectionHandler(conn, config, token, messageTimeout, networkTimeout, pingInterval, networkBufferSize)
		} else {
			go sessionConnectionHandler(conn, messageTimeout)
		}

	}
}

func protocolConnectionHandler(tcpConn net.Conn, config *tls.Config, token string, messageTimeout, networkTimeout, pingInterval time.Duration, networkBufferSize int) {
	conn := tls.Server(tcpConn, config)
	if err := conn.SetDeadline(time.Now().Add(messageTimeout)); err != nil {
		if debug {
			log.Println("Weird error setting deadline:", err, "on", conn.RemoteAddr())
		}
		conn.Close()
		return
	}
	err := conn.Handshake()
	if err != nil {
		if debug {
			log.Println("Protocol connection TLS handshake:", conn.RemoteAddr(), err)
		}
		conn.Close()
		return
	}

	state := conn.ConnectionState()
	if debug && state.NegotiatedProtocol != protocol.ProtocolName {
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
	conn.SetDeadline(time.Time{})

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
	defer pingTicker.Stop()
	timeoutTicker := time.NewTimer(networkTimeout)
	defer timeoutTicker.Stop()
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
				if token != "" && msg.Token != token {
					if debug {
						log.Printf("invalid token %s\n", msg.Token)
					}
					protocol.WriteMessage(conn, protocol.ResponseWrongToken)
					conn.Close()
					continue
				}

				if overLimit.Load() {
					protocol.WriteMessage(conn, protocol.RelayFull{})
					if debug {
						log.Println("Refusing join request from", id, "due to being over limits")
					}
					conn.Close()
					limitCheckTimer.Reset(time.Second)
					continue
				}

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
				requestedPeer, err := syncthingprotocol.DeviceIDFromBytes(msg.ID)
				if err != nil {
					if debug {
						log.Println(id, "is looking for an invalid peer ID")
					}
					protocol.WriteMessage(conn, protocol.ResponseNotFound)
					conn.Close()
					continue
				}
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
				// requestedPeer is the server, id is the client
				ses := newSession(requestedPeer, id, sessionLimiter, globalLimiter, messageTimeout, networkTimeout, networkBufferSize)

				go ses.Serve()

				clientInvitation := ses.GetClientInvitationMessage()
				serverInvitation := ses.GetServerInvitationMessage()

				if err := protocol.WriteMessage(conn, clientInvitation); err != nil {
					if debug {
						log.Printf("Error sending invitation from %s to client: %s", id, err)
					}
					conn.Close()
					continue
				}

				select {
				case peerOutbox <- serverInvitation:
					if debug {
						log.Println("Sent invitation from", id, "to", requestedPeer)
					}
				case <-time.After(time.Second):
					if debug {
						log.Println("Could not send invitation from", id, "to", requestedPeer, "as peer disconnected")
					}

				}
				conn.Close()

			case protocol.Ping:
				if err := protocol.WriteMessage(conn, protocol.Pong{}); err != nil {
					if debug {
						log.Println("Error writing pong:", err)
					}
					conn.Close()
					continue
				}

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

			// Potentially closing a second time.
			conn.Close()

			if joined {
				// Only delete the outbox if the client is joined, as it might be
				// a lookup request coming from the same client.
				outboxesMut.Lock()
				delete(outboxes, id)
				outboxesMut.Unlock()
				// Also, kill all sessions related to this node, as it probably
				// went offline. This is for the other end to realize the client
				// is no longer there faster. This also helps resolve
				// 'already connected' errors when one of the sides is
				// restarting, and connecting to the other peer before the other
				// peer even realised that the node has gone away.
				dropSessions(id)
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

			if overLimit.Load() && !hasSessions(id) {
				if debug {
					log.Println("Dropping", id, "as it has no sessions and we are over our limits")
				}
				protocol.WriteMessage(conn, protocol.RelayFull{})
				conn.Close()

				limitCheckTimer.Reset(time.Second)
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

func sessionConnectionHandler(conn net.Conn, messageTimeout time.Duration) {
	if err := conn.SetDeadline(time.Now().Add(messageTimeout)); err != nil {
		if debug {
			log.Println("Weird error setting deadline:", err, "on", conn.RemoteAddr())
		}
		conn.Close()
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
			log.Println(conn.RemoteAddr(), "session lookup", ses, hex.EncodeToString(msg.Key)[:5])
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
	numConnections.Add(1)
	defer numConnections.Add(-1)

	for {
		msg, err := protocol.ReadMessage(conn)
		if err != nil {
			errors <- err
			return
		}
		messages <- msg
	}
}
