// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/syncthing/protocol"
	"github.com/syncthing/syncthing/internal/db"
	"github.com/syndtr/goleveldb/leveldb"
)

func main() {
	log.SetFlags(0)
	log.SetOutput(os.Stdout)

	folder := flag.String("folder", "default", "Folder ID")
	device := flag.String("device", "", "Device ID (blank for global)")
	flag.Parse()

	ldb, err := leveldb.OpenFile(flag.Arg(0), nil)
	if err != nil {
		log.Fatal(err)
	}

	fs := db.NewFileSet(*folder, ldb)

	if *device == "" {
		log.Printf("*** Global index for folder %q", *folder)
		fs.WithGlobalTruncated(func(fi db.FileIntf) bool {
			f := fi.(db.FileInfoTruncated)
			fmt.Println(f)
			fmt.Println("\t", fs.Availability(f.Name))
			return true
		})
	} else {
		n, err := protocol.DeviceIDFromString(*device)
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("*** Have index for folder %q device %q", *folder, n)
		fs.WithHaveTruncated(n, func(fi db.FileIntf) bool {
			f := fi.(db.FileInfoTruncated)
			fmt.Println(f)
			return true
		})
	}
}
