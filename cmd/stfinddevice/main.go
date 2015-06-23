// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package main

import (
	"flag"
	"log"
	"os"

	"github.com/syncthing/protocol"
	"github.com/syncthing/syncthing/lib/discover"
)

func main() {
	log.SetFlags(0)
	log.SetOutput(os.Stdout)

	var server string

	flag.StringVar(&server, "server", "udp4://announce.syncthing.net:22027", "Announce server")
	flag.Parse()

	if len(flag.Args()) != 1 || server == "" {
		log.Printf("Usage: %s [-server=\"udp4://announce.syncthing.net:22027\"] <device>", os.Args[0])
		os.Exit(64)
	}

	id, err := protocol.DeviceIDFromString(flag.Args()[0])
	if err != nil {
		log.Println(err)
		os.Exit(1)
	}

	discoverer := discover.NewDiscoverer(protocol.LocalDeviceID, nil, nil)
	discoverer.StartGlobal([]string{server}, 1)
	addresses, relays := discoverer.Lookup(id)
	for _, addr := range addresses {
		log.Println("address:", addr)
	}
	for _, addr := range relays {
		log.Println("relay:", addr)
	}
}
