// Copyright (C) 2015 Audrius Butkevicius and Contributors (see the CONTRIBUTORS file).

package main

import (
	"log"
	"net"
	"time"

	"github.com/syncthing/relaysrv/protocol"
)

func sessionListener(addr string) {
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

		setTCPOptions(conn)

		if debug {
			log.Println("Session listener accepted connection from", conn.RemoteAddr())
		}

		go sessionConnectionHandler(conn)
	}
}

func sessionConnectionHandler(conn net.Conn) {
	defer conn.Close()

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
			return
		}

		if !ses.AddConnection(conn) {
			if debug {
				log.Println("Failed to add", conn.RemoteAddr(), "to session", ses)
			}
			protocol.WriteMessage(conn, protocol.ResponseAlreadyConnected)
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
			return
		}
	default:
		if debug {
			log.Println("Unexpected message from", conn.RemoteAddr(), message)
		}
		protocol.WriteMessage(conn, protocol.ResponseUnexpectedMessage)
	}
}
