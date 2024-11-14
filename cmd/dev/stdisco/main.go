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

	"google.golang.org/protobuf/proto"

	"github.com/syncthing/syncthing/internal/gen/discoproto"
	_ "github.com/syncthing/syncthing/lib/automaxprocs"
	"github.com/syncthing/syncthing/lib/beacon"
	"github.com/syncthing/syncthing/lib/discover"
	"github.com/syncthing/syncthing/lib/protocol"
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

		var ann discoproto.Announce
		proto.Unmarshal(data[4:], &ann)

		id, _ := protocol.DeviceIDFromBytes(ann.Id)
		if id == myID {
			// This is one of our own fake packets, don't print it.
			continue
		}

		// Print announcement details for the first packet from a given
		// device ID and source address, or if -all was given.
		key := id.String() + src.String()
		if all || !seen[key] {
			log.Printf("Announcement from %v\n", src)
			log.Printf(" %v at %s\n", id, strings.Join(ann.Addresses, ", "))
			seen[key] = true
		}
	}
}

// sends fake discovery announcements once every second
func send(bc beacon.Interface) {
	ann := &discoproto.Announce{
		Id:        myID[:],
		Addresses: []string{"tcp://fake.example.com:12345"},
	}
	bs, _ := proto.Marshal(ann)

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
