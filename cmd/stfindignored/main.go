// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// Command stfindignored lists ignored files under a given folder root.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/ignore"
	_ "go.uber.org/automaxprocs"
)

func main() {
	flag.Parse()
	root := flag.Arg(0)
	if root == "" {
		root = "."
	}

	vfs := fs.NewWalkFilesystem(fs.NewFilesystem(fs.FilesystemTypeBasic, root))

	ign := ignore.New(vfs)
	if err := ign.Load(".stignore"); err != nil {
		fmt.Fprintf(os.Stderr, "Fatal: loading ignores: %v\n", err)
		os.Exit(1)
	}

	vfs.Walk(".", func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: %s: %v\n", path, err)
			return fs.SkipDir
		}
		if ign.Match(path).IsIgnored() {
			fmt.Println(path)
		}
		return nil
	})
}
