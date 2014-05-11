package main

import (
	"encoding/binary"
	"log"
	"time"

	"github.com/calmh/syncthing/mc"
)

func main() {
	b := mc.NewBeacon("239.21.0.25", 21025)
	go func() {
		for {
			bs, addr := b.Recv()
			log.Printf("Received %d bytes from %s: %x %x", len(bs), addr, bs[:8], bs[8:])
		}
	}()
	go func() {
		bs := [16]byte{}
		binary.BigEndian.PutUint64(bs[:], uint64(time.Now().UnixNano()))
		log.Printf("My ID: %x", bs[:8])
		for {
			binary.BigEndian.PutUint64(bs[8:], uint64(time.Now().UnixNano()))
			b.Send(bs[:])
			log.Printf("Sent %d bytes", len(bs[:]))
			time.Sleep(10 * time.Second)
		}
	}()
	select {}
}
