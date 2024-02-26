// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"flag"
	"log"
	"strings"
	"time"

	"github.com/syncthing/syncthing/lib/beacon"
	"github.com/syncthing/syncthing/lib/discover"
	"github.com/syncthing/syncthing/lib/protocol"
	_ "go.uber.org/automaxprocs"
)

var (
	all  = false // print all packets, not just first from each device/source
	fake = false // send fake packets to lure out other devices faster
	mc   = "[ff12::8384]:21027"
	bc   = 21027
)

var (
	// Static prefix that we use when generating fake device IDs, so that we
	// can recognize them ourselves. Also makes the device ID start with
	// "STPROBE-" which is humanly recognizable.
	randomPrefix = []byte{148, 223, 23, 4, 148}

	// Our random, fake, device ID that we use when sending announcements.
	myID = randomDeviceID()
)

func main() {
	flag.BoolVar(&all, "all", all, "Print all received announcements (not only first)")
	flag.BoolVar(&fake, "fake", fake, "Send fake announcements")
	flag.StringVar(&mc, "mc", mc, "IPv6 multicast address")
	flag.IntVar(&bc, "bc", bc, "IPv4 broadcast port number")
	flag.Parse()

	if fake {
		log.Println("My ID:", myID)
	}

	ctx := context.Background()

	runbeacon(ctx, beacon.NewMulticast(mc), fake)
	runbeacon(ctx, beacon.NewBroadcast(bc), fake)

	select {}
}

func runbeacon(ctx context.Context, bc beacon.Interface, fake bool) {
	go bc.Serve(ctx)
	go recv(bc)
	if fake {
		go send(bc)
	}
}

// receives and prints discovery announcements
func recv(bc beacon.Interface) {
	seen := make(map[string]bool)
	for {
		data, src := bc.Recv()
		if m := binary.BigEndian.Uint32(data); m != discover.Magic {
			log.Printf("Incorrect magic %x in announcement from %v", m, src)
			continue
		}

		var ann discover.Announce
		ann.Unmarshal(data[4:])

		if ann.ID == myID {
			// This is one of our own fake packets, don't print it.
			continue
		}

		// Print announcement details for the first packet from a given
		// device ID and source address, or if -all was given.
		key := ann.ID.String() + src.String()
		if all || !seen[key] {
			log.Printf("Announcement from %v\n", src)
			log.Printf(" %v at %s\n", ann.ID, strings.Join(ann.Addresses, ", "))
			seen[key] = true
		}
	}
}

// sends fake discovery announcements once every second
func send(bc beacon.Interface) {
	ann := discover.Announce{
		ID:        myID,
		Addresses: []string{"tcp://fake.example.com:12345"},
	}
	bs, _ := ann.Marshal()

	for {
		bc.Send(bs)
		time.Sleep(time.Second)
	}
}

// returns a random but recognizable device ID
func randomDeviceID() protocol.DeviceID {
	var id protocol.DeviceID
	copy(id[:], randomPrefix)
	rand.Read(id[len(randomPrefix):])
	return id
}
