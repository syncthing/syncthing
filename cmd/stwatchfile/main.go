// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package main

import (
	"bytes"
	"crypto/md5"
	"flag"
	"fmt"
	"io"
	"os"
	"time"
)

func getmd5(filePath string) ([]byte, error) {
	var result []byte
	file, err := os.Open(filePath)
	if err != nil {
		return result, err
	}
	defer file.Close()

	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		return result, err
	}

	return hash.Sum(result), nil
}

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
	hash := []byte{}

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
			hash = []byte{}
			continue
		}

		if fi.IsDir() {
			fmt.Println(file, "is directory")
			return
		}
		newSize := fi.Size()
		newMtime := fi.ModTime()

		newHash, err := getmd5(file)
		if err != nil {
			fmt.Println("getmd5:", err)
		}

		if newSize != size || newMtime != mtime || !bytes.Equal(newHash, hash) {
			fmt.Println(file, "Size:", newSize, "Mtime:", newMtime, "Hash:", fmt.Sprintf("%x", newHash))
			hash = newHash
			size = newSize
			mtime = newMtime
		}
	}
}
