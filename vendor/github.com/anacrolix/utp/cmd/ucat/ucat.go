package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"

	"github.com/anacrolix/envpprof"

	"github.com/anacrolix/utp"
)

func main() {
	defer envpprof.Stop()
	listen := flag.Bool("l", false, "listen")
	port := flag.Int("p", 0, "port to listen on")
	flag.Parse()
	var (
		conn net.Conn
		err  error
	)
	if *listen {
		s, err := utp.NewSocket("udp", fmt.Sprintf(":%d", *port))
		if err != nil {
			log.Fatal(err)
		}
		defer s.Close()
		conn, err = s.Accept()
		if err != nil {
			log.Fatal(err)
		}
	} else {
		conn, err = utp.Dial(net.JoinHostPort(flag.Arg(0), flag.Arg(1)))
		if err != nil {
			log.Fatal(err)
		}
	}
	defer conn.Close()
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt)
		<-sig
		conn.Close()
	}()
	writerDone := make(chan struct{})
	readerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		written, err := io.Copy(conn, os.Stdin)
		if err != nil {
			conn.Close()
			log.Fatalf("error after writing %d bytes: %s", written, err)
		}
		log.Printf("wrote %d bytes", written)
		conn.Close()
	}()
	go func() {
		defer close(readerDone)
		n, err := io.Copy(os.Stdout, conn)
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("received %d bytes", n)
	}()
	// Technically we should wait until both reading and writing are done. But
	// ucat-style binaries terminate abrubtly when read or write is completed,
	// and no state remains to clean-up the peer neatly.
	select {
	case <-writerDone:
	case <-readerDone:
	}
}
