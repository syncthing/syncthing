// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	_ "github.com/syncthing/syncthing/lib/automaxprocs"
	"github.com/syncthing/syncthing/lib/sha256"
)

func main() {
	period := flag.Duration("period", 200*time.Millisecond, "Sleep period between checks")
	flag.Parse()

	file := flag.Arg(0)

	if file == "" {
		fmt.Println("Expects a path as an argument")
		return
	}

	exists := true
	size := int64(0)
	mtime := time.Time{}
	var hash [sha256.Size]byte

	for {
		time.Sleep(*period)

		newExists := true
		fi, err := os.Stat(file)
		if err != nil && os.IsNotExist(err) {
			newExists = false
		} else if err != nil {
			fmt.Println("stat:", err)
			return
		}

		if newExists != exists {
			exists = newExists
			if !newExists {
				fmt.Println(file, "does not exist")
			} else {
				fmt.Println(file, "appeared")
			}
		}

		if !exists {
			size = 0
			mtime = time.Time{}
			hash = [sha256.Size]byte{}
			continue
		}

		if fi.IsDir() {
			fmt.Println(file, "is directory")
			return
		}
		newSize := fi.Size()
		newMtime := fi.ModTime()

		newHash, err := sha256file(file)
		if err != nil {
			fmt.Println("sha256file:", err)
		}

		if newSize != size || newMtime != mtime || newHash != hash {
			fmt.Println(file, "Size:", newSize, "Mtime:", newMtime, "Hash:", fmt.Sprintf("%x", newHash))
			hash = newHash
			size = newSize
			mtime = newMtime
		}
	}
}

func sha256file(fname string) (hash [sha256.Size]byte, err error) {
	f, err := os.Open(fname)
	if err != nil {
		return
	}
	defer f.Close()

	h := sha256.New()
	io.Copy(h, f)
	hb := h.Sum(nil)
	copy(hash[:], hb)

	return
}
