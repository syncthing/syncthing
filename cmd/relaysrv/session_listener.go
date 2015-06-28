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
		setTCPOptions(conn)
		if err != nil {
			if debug {
				log.Println(err)
			}
			continue
		}

		if debug {
			log.Println("Session listener accepted connection from", conn.RemoteAddr())
		}

		go sessionConnectionHandler(conn)
	}
}

func sessionConnectionHandler(conn net.Conn) {
	conn.SetDeadline(time.Now().Add(messageTimeout))
	message, err := protocol.ReadMessage(conn)
	if err != nil {
		conn.Close()
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

		err := protocol.WriteMessage(conn, protocol.ResponseSuccess)
		if err != nil {
			if debug {
				log.Println("Failed to send session join response to ", conn.RemoteAddr(), "for", ses)
			}
			conn.Close()
			return
		}
		conn.SetDeadline(time.Time{})
	default:
		if debug {
			log.Println("Unexpected message from", conn.RemoteAddr(), message)
		}
		protocol.WriteMessage(conn, protocol.ResponseUnexpectedMessage)
		conn.Close()
	}
}
