// Copyright (C) 2021 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// +build ignore

package main

import (
	"bytes"
	"flag"
	"log"
	"os"
	"os/exec"
	"path/filepath"
)

func main() {
	var path string
	flag.StringVar(&path, "t", "", "Name of file to prune")
	flag.Parse()

	filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Fatal(err)
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		err = pruneInterfaceCheck(path, info.Size())
		if err != nil {
			log.Fatal(err)
		}
		err = exec.Command("goimports", "-w", path).Run()
		if err != nil {
			log.Fatal(err)
		}
		return nil
	})
}

func pruneInterfaceCheck(path string, size int64) error {
	fd, err := os.OpenFile(path, os.O_RDWR, 0666)
	if err != nil {
		return err
	}
	defer fd.Close()

	var chunk int64 = 100
	buf := make([]byte, chunk)
	searched := []byte("var _ ")
	pos := size - chunk
	for {
		_, err = fd.ReadAt(buf, pos)
		if err != nil {
			return err
		}
		if i := bytes.LastIndex(buf, searched); i != -1 {
			pos += int64(i)
			break
		}
		pos -= chunk
	}
	return fd.Truncate(pos)
}
