// Copyright (C) 2020 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build ignore
// +build ignore

package main

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
)

//go:generate go run scripts/protofmt.go .

// First generate extensions using standard proto compiler.
//go:generate protoc -I ../ -I . --gogofast_out=Mgoogle/protobuf/descriptor.proto=github.com/gogo/protobuf/protoc-gen-gogo/descriptor,paths=source_relative:ext ext.proto

// Then build our vanity compiler that uses the new extensions
//go:generate go build -o scripts/protoc-gen-gosyncthing scripts/protoc_plugin.go

// Inception, go generate calls the script itself that then deals with generation.
// This is only done because go:generate does not support wildcards in paths.
//go:generate go run generate.go lib/protocol lib/config lib/fs lib/db lib/discover lib/api lib/ur

func main() {
	for _, path := range os.Args[1:] {
		matches, err := filepath.Glob(filepath.Join(path, "*proto"))
		if err != nil {
			log.Fatal(err)
		}
		log.Println(path, "returned:", matches)
		args := []string{
			"-I", "..",
			"-I", ".",
			"--plugin=protoc-gen-gosyncthing=scripts/protoc-gen-gosyncthing",
			"--gosyncthing_out=paths=source_relative:..",
		}
		args = append(args, matches...)
		cmd := exec.Command("protoc", args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			log.Fatal("Failed generating", path)
		}
	}
}
