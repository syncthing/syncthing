// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/syncthing/syncthing/internal/files"
	"github.com/syncthing/syncthing/internal/protocol"
	"github.com/syndtr/goleveldb/leveldb"
)

func main() {
	log.SetFlags(0)
	log.SetOutput(os.Stdout)

	folder := flag.String("folder", "default", "Folder ID")
	device := flag.String("device", "", "Device ID (blank for global)")
	flag.Parse()

	db, err := leveldb.OpenFile(flag.Arg(0), nil)
	if err != nil {
		log.Fatal(err)
	}

	fs := files.NewSet(*folder, db)

	if *device == "" {
		log.Printf("*** Global index for folder %q", *folder)
		fs.WithGlobalTruncated(func(fi protocol.FileIntf) bool {
			f := fi.(protocol.FileInfoTruncated)
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
		fs.WithHaveTruncated(n, func(fi protocol.FileIntf) bool {
			f := fi.(protocol.FileInfoTruncated)
			fmt.Println(f)
			return true
		})
	}
}
