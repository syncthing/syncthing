// Copyright (C) 2015 Audrius Butkevicius and Contributors (see the CONTRIBUTORS file).

package main

import (
	"io"
	"log"
	"net"
	"time"
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

		if debug {
			log.Println("Session listener accepted connection from", conn.RemoteAddr())
		}

		go sessionConnectionHandler(conn)
	}
}

func sessionConnectionHandler(conn net.Conn) {
	conn.SetReadDeadline(time.Now().Add(messageTimeout))
	key := make([]byte, 32)

	_, err := io.ReadFull(conn, key)
	if err != nil {
		if debug {
			log.Println("Failed to read key", err, conn.RemoteAddr())
		}
		conn.Close()
		return
	}

	ses := findSession(string(key))
	if debug {
		log.Println("Key", key, "by", conn.RemoteAddr(), "session", ses)
	}

	if ses != nil {
		ses.AddConnection(conn)
	} else {
		conn.Close()
		return
	}
}
