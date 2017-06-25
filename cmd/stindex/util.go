// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/syncthing/syncthing/lib/fs"
)

func nulString(bs []byte) string {
	for i := range bs {
		if bs[i] == 0 {
			return string(bs[:i])
		}
	}
	return string(bs)
}

func defaultConfigDir() string {
	switch runtime.GOOS {
	case "windows":
		if p := os.Getenv("LocalAppData"); p != "" {
			return filepath.Join(p, "Syncthing")
		}
		return filepath.Join(os.Getenv("AppData"), "Syncthing")

	case "darwin":
		dir, err := fs.ExpandTilde("~/Library/Application Support/Syncthing")
		if err != nil {
			log.Fatal(err)
		}
		return dir

	default:
		if xdgCfg := os.Getenv("XDG_CONFIG_HOME"); xdgCfg != "" {
			return filepath.Join(xdgCfg, "syncthing")
		}
		dir, err := fs.ExpandTilde("~/.config/syncthing")
		if err != nil {
			log.Fatal(err)
		}
		return dir
	}
}
