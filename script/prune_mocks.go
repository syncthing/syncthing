// Copyright (C) 2021 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build ignore
// +build ignore

package main

import (
	"bufio"
	"flag"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
		err = exec.Command("go", "tool", "goimports", "-w", path).Run()
		if err != nil {
			log.Fatal(err)
		}
		return nil
	})
}

func pruneInterfaceCheck(path string, size int64) error {
	fd, err := os.Open(path)
	if err != nil {
		return err
	}
	defer fd.Close()

	tmp, err := os.CreateTemp(".", "")
	if err != nil {
		return err
	}

	scanner := bufio.NewScanner(fd)

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(strings.TrimSpace(line), "var _ ") {
			continue
		}
		if _, err := tmp.WriteString(line + "\n"); err != nil {
			os.Remove(tmp.Name())
			return err
		}
	}

	if err := fd.Close(); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
