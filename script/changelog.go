// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// +build ignore

package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/mitchellh/go-wordwrap"
)

var (
	subjectIssues = regexp.MustCompile(`^([^(]+)\s+\((?:fixes|ref) ([^)]+)\)(?:[^\w])?$`)
	issueNumbers  = regexp.MustCompile(`(#\d+)`)
)

func main() {
	flag.Parse()

	fmt.Printf("Resolved issues:\n\n")

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
	bs, err := runError("git", "log", "--reverse", "--pretty=format:%s|%aN|%cN", prevRel+"..")
	if err != nil {
		log.Fatal(err)
	}

	// Split into lines
	for _, line := range bytes.Split(bs, []byte{'\n'}) {
		// Split into subject and author
		fields := bytes.Split(line, []byte{'|'})
		subj := fields[0]
		author := fields[1]
		committer := fields[2]

		// Check if subject contains a "(fixes ...)" or "(ref ...)""
		if m := subjectIssues.FindSubmatch(subj); len(m) > 0 {
			subj := m[1]
			issues := issueNumbers.FindAll(m[2], -1)
			for _, issue := range issues {
				n, err := strconv.Atoi(string(issue[1:]))
				if err != nil {
					continue
				}
				title, err := githubIssueTitle(n)
				if err != nil {
					fmt.Println(err)
					continue
				}

				// Format a changelog entry
				reviewed := ""
				if !bytes.Equal(committer, author) {
					reviewed = fmt.Sprintf(", reviewed by @%s", committer)
				}

				message := fmt.Sprintf("%s: %s\n\n%s (by @%s%s)\n", issue, title, subj, author, reviewed)
				para := wordwrap.WrapString(message, 74)
				for i, line := range strings.Split(para, "\n") {
					if i == 0 {
						fmt.Println("*", line)
					} else {
						fmt.Println(" ", line)
					}
				}
			}
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

func githubIssueTitle(n int) (string, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("https://api.github.com/repos/syncthing/syncthing/issues/%d", n), nil)
	if err != nil {
		return "", err
	}

	user, token := os.Getenv("GITHUB_USERNAME"), os.Getenv("GITHUB_TOKEN")
	if user != "" && token != "" {
		req.SetBasicAuth(user, token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	bs, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var res struct {
		Title string
	}
	err = json.Unmarshal(bs, &res)
	if err != nil {
		return "", err
	}

	return res.Title, nil
}
