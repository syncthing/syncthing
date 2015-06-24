// Copyright (C) 2015 Audrius Butkevicius and Contributors (see the CONTRIBUTORS file).

package main

import (
	"crypto/tls"
	"io"
	"log"
	"net"
	"time"

	syncthingprotocol "github.com/syncthing/protocol"

	"github.com/syncthing/relaysrv/protocol"
)

type message struct {
	header  protocol.Header
	payload []byte
}

func protocolListener(addr string, config *tls.Config) {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalln(err)
	}

	for {
		conn, err := listener.Accept()
		if err != nil {
			if debug {
				log.Println(err)
			}
			continue
		}

		if debug {
			log.Println("Protocol listener accepted connection from", conn.RemoteAddr())
		}

		go protocolConnectionHandler(conn, config)
	}
}

func protocolConnectionHandler(tcpConn net.Conn, config *tls.Config) {
	err := setTCPOptions(tcpConn)
	if err != nil && debug {
		log.Println("Failed to set TCP options on protocol connection", tcpConn.RemoteAddr(), err)
	}

	conn := tls.Server(tcpConn, config)
	err = conn.Handshake()
	if err != nil {
		log.Println("Protocol connection TLS handshake:", conn.RemoteAddr(), err)
		conn.Close()
		return
	}

	state := conn.ConnectionState()
	if (!state.NegotiatedProtocolIsMutual || state.NegotiatedProtocol != protocol.ProtocolName) && debug {
		log.Println("Protocol negotiation error")
	}

	certs := state.PeerCertificates
	if len(certs) != 1 {
		log.Println("Certificate list error")
		conn.Close()
		return
	}

	deviceId := syncthingprotocol.NewDeviceID(certs[0].Raw)

	mut.RLock()
	_, ok := outbox[deviceId]
	mut.RUnlock()
	if ok {
		log.Println("Already have a peer with the same ID", deviceId, conn.RemoteAddr())
		conn.Close()
		return
	}

	errorChannel := make(chan error)
	messageChannel := make(chan message)
	outboxChannel := make(chan message)

	go readerLoop(conn, messageChannel, errorChannel)

	pingTicker := time.NewTicker(pingInterval)
	timeoutTicker := time.NewTimer(messageTimeout * 2)
	joined := false

	for {
		select {
		case msg := <-messageChannel:
			switch msg.header.MessageType {
			case protocol.MessageTypeJoinRequest:
				mut.Lock()
				outbox[deviceId] = outboxChannel
				mut.Unlock()
				joined = true
			case protocol.MessageTypeConnectRequest:
				// We will disconnect after this message, no matter what,
				// because, we've either sent out an invitation, or we don't
				// have the peer available.
				var fmsg protocol.ConnectRequest
				err := fmsg.UnmarshalXDR(msg.payload)
				if err != nil {
					log.Println(err)
					conn.Close()
					continue
				}

				requestedPeer := syncthingprotocol.DeviceIDFromBytes(fmsg.ID)
				mut.RLock()
				peerOutbox, ok := outbox[requestedPeer]
				mut.RUnlock()
				if !ok {
					if debug {
						log.Println("Do not have", requestedPeer)
					}
					conn.Close()
					continue
				}

				ses := newSession()

				smsg, err := ses.GetServerInvitationMessage()
				if err != nil {
					log.Println("Error getting server invitation", requestedPeer)
					conn.Close()
					continue
				}
				cmsg, err := ses.GetClientInvitationMessage()
				if err != nil {
					log.Println("Error getting client invitation", requestedPeer)
					conn.Close()
					continue
				}

				go ses.Serve()

				if err := sendMessage(cmsg, conn); err != nil {
					log.Println("Failed to send invitation message", err)
				} else {
					peerOutbox <- smsg
					if debug {
						log.Println("Sent invitation from", deviceId, "to", requestedPeer)
					}
				}
				conn.Close()
			case protocol.MessageTypePong:
				timeoutTicker.Reset(messageTimeout)
			}
		case err := <-errorChannel:
			log.Println("Closing connection:", err)
			return
		case <-pingTicker.C:
			if !joined {
				log.Println(deviceId, "didn't join within", messageTimeout)
				conn.Close()
				continue
			}

			if err := sendMessage(pingMessage, conn); err != nil {
				log.Println(err)
				conn.Close()
				continue
			}
		case <-timeoutTicker.C:
			// We should receive a error, which will cause us to quit the
			// loop.
			conn.Close()
		case msg := <-outboxChannel:
			if debug {
				log.Println("Sending message to", deviceId, msg)
			}
			if err := sendMessage(msg, conn); err == nil {
				log.Println(err)
				conn.Close()
				continue
			}
		}
	}
}

func readerLoop(conn *tls.Conn, messages chan<- message, errors chan<- error) {
	header := make([]byte, protocol.HeaderSize)
	data := make([]byte, 0, 0)
	for {
		_, err := io.ReadFull(conn, header)
		if err != nil {
			errors <- err
			conn.Close()
			return
		}

		var hdr protocol.Header
		err = hdr.UnmarshalXDR(header)
		if err != nil {
			conn.Close()
			return
		}

		if hdr.Magic != protocol.Magic {
			conn.Close()
			return
		}

		if hdr.MessageLength > int32(cap(data)) {
			data = make([]byte, 0, hdr.MessageLength)
		} else {
			data = data[:hdr.MessageLength]
		}

		_, err = io.ReadFull(conn, data)
		if err != nil {
			errors <- err
			conn.Close()
			return
		}

		msg := message{
			header:  hdr,
			payload: make([]byte, hdr.MessageLength),
		}
		copy(msg.payload, data[:hdr.MessageLength])

		messages <- msg
	}
}
