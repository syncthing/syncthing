// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build ignore
// +build ignore

package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

const (
	exitSuccess = 0
	exitError   = 1
)

var subject = regexp.MustCompile(`^[\w/,\. ]+: \w`)

func main() {
	if len(os.Args) != 2 {
		fmt.Printf("Usage: %s <file>\n", filepath.Base(os.Args[0]))
		os.Exit(exitError)
	}

	bs, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Println("Reading input:", err)
		os.Exit(exitError)
	}

	lines := bytes.Split(bs, []byte{'\n'})
	if !subject.Match(lines[0]) {
		fmt.Printf(`Commit message subject:

    %s

doesn't look like "tag: One sentence description". Specifically, it doesn't
match this pattern:

    %s
`, lines[0], subject)
		os.Exit(exitError)
	}

	os.Exit(exitSuccess)
}
