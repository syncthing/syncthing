// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/syncthing/syncthing/lib/db/backend"
)

func main() {
	var mode string
	log.SetFlags(0)
	log.SetOutput(os.Stdout)

	flag.StringVar(&mode, "mode", "dump", "Mode of operation: dump, dumpsize, idxck")

	flag.Parse()

	path := flag.Arg(0)
	if path == "" {
		path = filepath.Join(defaultConfigDir(), "index-v0.14.0.db")
	}

	ldb, err := backend.OpenLevelDBRO(path)
	if err != nil {
		log.Fatal(err)
	}

	switch mode {
	case "dump":
		dump(ldb)
	case "dumpsize":
		dumpsize(ldb)
	case "idxck":
		if !idxck(ldb) {
			os.Exit(1)
		}
	case "account":
		account(ldb)
	default:
		fmt.Println("Unknown mode")
	}
}
