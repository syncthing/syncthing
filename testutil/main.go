// Copyright (C) 2015 Audrius Butkevicius and Contributors (see the CONTRIBUTORS file).

package main

import (
	"bufio"
	"crypto/tls"
	"flag"
	"log"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"time"

	syncthingprotocol "github.com/syncthing/protocol"
	"github.com/syncthing/relaysrv/client"
	"github.com/syncthing/relaysrv/protocol"
)

func main() {
	log.SetOutput(os.Stdout)
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	var connect, relay, dir string
	var join bool

	flag.StringVar(&connect, "connect", "", "Device ID to which to connect to")
	flag.BoolVar(&join, "join", false, "Join relay")
	flag.StringVar(&relay, "relay", "relay://127.0.0.1:22067", "Relay address")
	flag.StringVar(&dir, "keys", ".", "Directory where cert.pem and key.pem is stored")

	flag.Parse()

	certFile, keyFile := filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		log.Fatalln("Failed to load X509 key pair:", err)
	}

	id := syncthingprotocol.NewDeviceID(cert.Certificate[0])
	log.Println("ID:", id)

	uri, err := url.Parse(relay)
	if err != nil {
		log.Fatal(err)
	}

	stdin := make(chan string)

	go stdinReader(stdin)

	if join {
		log.Printf("Creating client")
		relay := client.NewProtocolClient(uri, []tls.Certificate{cert}, nil)
		log.Printf("Created client")

		go relay.Serve()

		recv := make(chan protocol.SessionInvitation)

		go func() {
			log.Println("Starting invitation receiver")
			for invite := range relay.Invitations {
				select {
				case recv <- invite:
					log.Printf("Received invitation from %s on %s:%d", syncthingprotocol.DeviceIDFromBytes(invite.From), net.IP(invite.Address), invite.Port)
				default:
					log.Printf("Discarding invitation", invite)
				}
			}
		}()

		for {
			conn, err := client.JoinSession(<-recv)
			if err != nil {
				log.Fatalln("Failed to join", err)
			}
			log.Println("Joined", conn.RemoteAddr(), conn.LocalAddr())
			connectToStdio(stdin, conn)
			log.Println("Finished", conn.RemoteAddr(), conn.LocalAddr())
		}
	} else if connect != "" {
		id, err := syncthingprotocol.DeviceIDFromString(connect)
		if err != nil {
			log.Fatal(err)
		}

		invite, err := client.GetInvitationFromRelay(uri, id, []tls.Certificate{cert})
		if err != nil {
			log.Fatal(err)
		}

		log.Printf("Received invitation from %s on %s:%d", syncthingprotocol.DeviceIDFromBytes(invite.From), net.IP(invite.Address), invite.Port)
		conn, err := client.JoinSession(invite)
		if err != nil {
			log.Fatalln("Failed to join", err)
		}
		log.Println("Joined", conn.RemoteAddr(), conn.LocalAddr())
		connectToStdio(stdin, conn)
		log.Println("Finished", conn.RemoteAddr(), conn.LocalAddr())
	} else {
		log.Fatal("Requires either join or connect")
	}
}

func stdinReader(c chan<- string) {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		c <- scanner.Text()
		c <- "\n"
	}
}

func connectToStdio(stdin <-chan string, conn net.Conn) {
	go func() {

	}()

	buf := make([]byte, 1024)
	for {
		conn.SetReadDeadline(time.Now().Add(time.Millisecond))
		n, err := conn.Read(buf[0:])
		if err != nil {
			nerr, ok := err.(net.Error)
			if !ok || !nerr.Timeout() {
				log.Println(err)
				return
			}
		}
		os.Stdout.Write(buf[:n])

		select {
		case msg := <-stdin:
			_, err := conn.Write([]byte(msg))
			if err != nil {
				return
			}
		default:
		}
	}
}
