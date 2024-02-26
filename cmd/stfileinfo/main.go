// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"context"
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/scanner"
	_ "go.uber.org/automaxprocs"
)

func main() {
	log.SetFlags(0)
	log.SetOutput(os.Stdout)

	standardBlocks := flag.Bool("s", false, "Use standard block size")
	flag.Parse()

	path := flag.Arg(0)
	if path == "" {
		log.Fatal("Need one argument: path to check")
	}

	log.Println("File:")
	log.Println(" ", filepath.Clean(path))
	log.Println()

	fi, err := os.Lstat(path)
	if err != nil {
		log.Fatal(err)
	}

	log.Println("Lstat:")
	log.Printf("  Size: %d bytes", fi.Size())
	log.Printf("  Mode: 0%o", fi.Mode())
	log.Printf("  Time: %v", fi.ModTime())
	log.Printf("        %d.%09d", fi.ModTime().Unix(), fi.ModTime().Nanosecond())
	log.Println()

	if !fi.Mode().IsDir() && !fi.Mode().IsRegular() {
		fi, err = os.Stat(path)
		if err != nil {
			log.Fatal(err)
		}

		log.Println("Stat:")
		log.Printf("  Size: %d bytes", fi.Size())
		log.Printf("  Mode: 0%o", fi.Mode())
		log.Printf("  Time: %v", fi.ModTime())
		log.Printf("        %d.%09d", fi.ModTime().Unix(), fi.ModTime().Nanosecond())
		log.Println()
	}

	if fi.Mode().IsRegular() {
		log.Println("Blocks:")

		fd, err := os.Open(path)
		if err != nil {
			log.Fatal(err)
		}

		blockSize := int(fi.Size())
		if *standardBlocks || blockSize < protocol.MinBlockSize {
			blockSize = protocol.BlockSize(fi.Size())
		}
		bs, err := scanner.Blocks(context.TODO(), fd, blockSize, fi.Size(), nil, true)
		if err != nil {
			log.Fatal(err)
		}

		for _, b := range bs {
			log.Println(" ", b)
		}
	}
}
