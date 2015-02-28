// Copyright (C) 2014 The Syncthing Authors.
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

package main

import (
	"flag"
	"log"
	"os"

	"github.com/syncthing/protocol"
	"github.com/syncthing/syncthing/internal/discover"
)

func main() {
	log.SetFlags(0)
	log.SetOutput(os.Stdout)

	var server string

	flag.StringVar(&server, "server", "udp4://announce.syncthing.net:22026", "Announce server")
	flag.Parse()

	if len(flag.Args()) != 1 || server == "" {
		log.Printf("Usage: %s [-server=\"udp4://announce.syncthing.net:22026\"] <device>", os.Args[0])
		os.Exit(64)
	}

	id, err := protocol.DeviceIDFromString(flag.Args()[0])
	if err != nil {
		log.Println(err)
		os.Exit(1)
	}

	discoverer := discover.NewDiscoverer(protocol.LocalDeviceID, nil)
	discoverer.StartGlobal([]string{server}, 1)
	for _, addr := range discoverer.Lookup(id) {
		log.Println(addr)
	}
}
