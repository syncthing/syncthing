// Copyright (C) 2015 Audrius Butkevicius and Contributors (see the CONTRIBUTORS file).

package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"flag"
	"log"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"time"

	_ "github.com/syncthing/syncthing/lib/automaxprocs"
	syncthingprotocol "github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/relay/client"
	"github.com/syncthing/syncthing/lib/relay/protocol"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	log.SetOutput(os.Stdout)
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	var connect, relay, dir string
	var join, test bool

	flag.StringVar(&connect, "connect", "", "Device ID to which to connect to")
	flag.BoolVar(&join, "join", false, "Join relay")
	flag.BoolVar(&test, "test", false, "Generic relay test")
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
		log.Println("Creating client")
		relay, err := client.NewClient(uri, []tls.Certificate{cert}, 10*time.Second)
		if err != nil {
			log.Fatal(err)
		}
		log.Println("Created client")

		go relay.Serve(ctx)

		recv := make(chan protocol.SessionInvitation)

		go func() {
			log.Println("Starting invitation receiver")
			for invite := range relay.Invitations() {
				select {
				case recv <- invite:
					log.Println("Received invitation", invite)
				default:
					log.Println("Discarding invitation", invite)
				}
			}
		}()

		for {
			conn, err := client.JoinSession(ctx, <-recv)
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

		invite, err := client.GetInvitationFromRelay(ctx, uri, id, []tls.Certificate{cert}, 10*time.Second)
		if err != nil {
			log.Fatal(err)
		}

		log.Println("Received invitation", invite)
		conn, err := client.JoinSession(ctx, invite)
		if err != nil {
			log.Fatalln("Failed to join", err)
		}
		log.Println("Joined", conn.RemoteAddr(), conn.LocalAddr())
		connectToStdio(stdin, conn)
		log.Println("Finished", conn.RemoteAddr(), conn.LocalAddr())
	} else if test {
		if err := client.TestRelay(ctx, uri, []tls.Certificate{cert}, time.Second, 2*time.Second, 4); err == nil {
			log.Println("OK")
		} else {
			log.Println("FAIL:", err)
		}
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
