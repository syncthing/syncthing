// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// +build ignore

package main

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"os/exec"
	"regexp"
)

var (
	subjectIssues = regexp.MustCompile(`^([^(]+)\s+\((?:fixes|ref) ([^)]+)\)(?:[^\w])?$`)
	issueNumbers  = regexp.MustCompile(`(#\d+)`)
)

func main() {
	flag.Parse()

	// Display changelog since the version given on the command line, or
	// figure out the last release if there were no arguments.
	var prevRel string
	if flag.NArg() > 0 {
		prevRel = flag.Arg(0)
	} else {
		bs, err := runError("git", "describe", "--abbrev=0", "HEAD^")
		if err != nil {
			log.Fatal(err)
		}
		prevRel = string(bs)
	}

	// Get the git log with subject and author nickname
	bs, err := runError("git", "log", "--reverse", "--pretty=format:%s|%aN", prevRel+"..")
	if err != nil {
		log.Fatal(err)
	}

	// Split into lines
	for _, line := range bytes.Split(bs, []byte{'\n'}) {
		// Split into subject and author
		fields := bytes.Split(line, []byte{'|'})
		subj := fields[0]
		author := fields[1]

		// Check if subject contains a "(fixes ...)" or "(ref ...)""
		if m := subjectIssues.FindSubmatch(subj); len(m) > 0 {
			// Find all issue numbers
			issues := issueNumbers.FindAll(m[2], -1)

			// Format a changelog entry
			fmt.Printf("* %s (%s, @%s)\n", m[1], bytes.Join(issues, []byte(", ")), author)
		}
	}
}

func runError(cmd string, args ...string) ([]byte, error) {
	ecmd := exec.Command(cmd, args...)
	bs, err := ecmd.CombinedOutput()
	if err != nil {
		return nil, err
	}
	return bytes.TrimSpace(bs), nil
}
