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
	"path/filepath"

	"github.com/syncthing/protocol"
	"github.com/syncthing/syncthing/internal/scanner"
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
	log.Printf("  Time: %v (%d)", fi.ModTime(), fi.ModTime().Unix())
	log.Println()

	if !fi.Mode().IsDir() && !fi.Mode().IsRegular() {
		fi, err = os.Stat(path)
		if err != nil {
			log.Fatal(err)
		}

		log.Println("Stat:")
		log.Printf("  Size: %d bytes", fi.Size())
		log.Printf("  Mode: 0%o", fi.Mode())
		log.Printf("  Time: %v (%d)", fi.ModTime(), fi.ModTime().Unix())
		log.Println()
	}

	if fi.Mode().IsRegular() {
		log.Println("Blocks:")

		fd, err := os.Open(path)
		if err != nil {
			log.Fatal(err)
		}

		blockSize := int(fi.Size())
		if *standardBlocks || blockSize < protocol.BlockSize {
			blockSize = protocol.BlockSize
		}
		bs, err := scanner.Blocks(fd, blockSize, fi.Size())
		if err != nil {
			log.Fatal(err)
		}

		for _, b := range bs {
			log.Println(" ", b)
		}
	}
}
