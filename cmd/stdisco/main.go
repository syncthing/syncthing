// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package main

import (
	"bytes"
	"crypto/rand"
	"flag"
	"log"
	"strings"
	"time"

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
		log.Println("My ID:", protocol.DeviceIDFromBytes(myID))
	}

	runbeacon(beacon.NewMulticast(mc), fake)
	runbeacon(beacon.NewBroadcast(bc), fake)

	select {}
}

func runbeacon(bc beacon.Interface, fake bool) {
	go bc.Serve()
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
		var ann discover.Announce
		ann.UnmarshalXDR(data)

		if bytes.Equal(ann.This.ID, myID) {
			// This is one of our own fake packets, don't print it.
			continue
		}

		// Print announcement details for the first packet from a given
		// device ID and source address, or if -all was given.
		key := string(ann.This.ID) + src.String()
		if all || !seen[key] {
			log.Printf("Announcement from %v\n", src)
			log.Printf(" %v at %s\n", protocol.DeviceIDFromBytes(ann.This.ID), strings.Join(addrStrs(ann.This), ", "))

			for _, dev := range ann.Extra {
				log.Printf(" %v at %s\n", protocol.DeviceIDFromBytes(dev.ID), strings.Join(addrStrs(dev), ", "))
			}
			seen[key] = true
		}
	}
}

// sends fake discovery announcements once every second
func send(bc beacon.Interface) {
	ann := discover.Announce{
		Magic: discover.AnnouncementMagic,
		This: discover.Device{
			ID: myID,
			Addresses: []discover.Address{
				{URL: "tcp://fake.example.com:12345"},
			},
		},
	}
	bs, _ := ann.MarshalXDR()

	for {
		bc.Send(bs)
		time.Sleep(time.Second)
	}
}

// returns the list of address URLs
func addrStrs(dev discover.Device) []string {
	ss := make([]string, len(dev.Addresses))
	for i, addr := range dev.Addresses {
		ss[i] = addr.URL
	}
	return ss
}

// returns a random but recognizable device ID
func randomDeviceID() []byte {
	var id [32]byte
	copy(id[:], randomPrefix)
	rand.Read(id[len(randomPrefix):])
	return id[:]
}
