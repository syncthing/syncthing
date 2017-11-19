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
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var (
	subjectIssues = regexp.MustCompile(`^([^(]+)\s+\((?:fixes|ref) ([^)]+)\)(?:[^\w])?$`)
	issueNumbers  = regexp.MustCompile(`(#\d+)`)
)

type issue struct {
	number  int
	subject string
	labels  []string
}

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
	bs, err := runError("git", "log", "--reverse", "--pretty=format:%s", prevRel+"..")
	if err != nil {
		log.Fatal(err)
	}

	var resolved []issue

	// Split into lines
	for _, line := range bytes.Split(bs, []byte{'\n'}) {
		// Check if subject contains a "(fixes ...)" or "(ref ...)""
		if m := subjectIssues.FindSubmatch(line); len(m) > 0 {
			issues := issueNumbers.FindAll(m[2], -1)
			for _, i := range issues {
				n, err := strconv.Atoi(string(i[1:]))
				if err != nil {
					continue
				}
				title, labels, err := githubIssueTitleLabels(n)
				if err != nil {
					continue
				}

				resolved = append(resolved, issue{n, title, labels})
			}
		}
	}

	sort.Slice(resolved, func(a, b int) bool {
		return resolved[a].number < resolved[b].number
	})

	var bugs, enhancements, other []issue
	var prev int
	for _, i := range resolved {
		if i.number == prev {
			continue
		}
		prev = i.number
		switch {
		case contains("unreleased", i.labels):
			continue
		case contains("bug", i.labels):
			bugs = append(bugs, i)
		case contains("enhancement", i.labels):
			enhancements = append(enhancements, i)
		default:
			other = append(other, i)
		}
	}

	fmt.Printf("--- markdown ---\n\n")
	markdown(prevRel, bugs, enhancements, other)
	fmt.Printf("\n--- text ---\n\n")
	text(prevRel, bugs, enhancements, other)
}

func markdown(version string, bugs, enhancements, other []issue) {
	fmt.Printf("## Resolved issues since %s\n\n", version)
	if len(bugs) > 0 {
		fmt.Printf("### Bugs\n\n")
		for _, issue := range bugs {
			fmt.Printf("* [#%d](https://github.com/syncthing/syncthing/issues/%d): %s\n", issue.number, issue.number, issue.subject)
		}
		fmt.Println()
	}
	if len(enhancements) > 0 {
		fmt.Printf("### Enhancements\n\n")
		for _, issue := range enhancements {
			fmt.Printf("* [#%d](https://github.com/syncthing/syncthing/issues/%d): %s\n", issue.number, issue.number, issue.subject)
		}
		fmt.Println()
	}
	if len(other) > 0 {
		fmt.Printf("### Unclassified\n\n")
		for _, issue := range other {
			fmt.Printf("* [#%d](https://github.com/syncthing/syncthing/issues/%d): %s\n", issue.number, issue.number, issue.subject)
		}
		fmt.Println()
	}
}

func text(version string, bugs, enhancements, other []issue) {
	fmt.Println(underline(fmt.Sprintf("Resolved issues since %s", version), "="))
	fmt.Println()
	if len(bugs) > 0 {
		fmt.Println(underline("Bugs", "-"))
		fmt.Println()
		for _, issue := range bugs {
			fmt.Printf("* #%d: %s\n", issue.number, issue.subject)
		}
		fmt.Println()
	}
	if len(enhancements) > 0 {
		fmt.Println(underline("Enhancements", "-"))
		fmt.Println()
		for _, issue := range enhancements {
			fmt.Printf("* #%d: %s\n", issue.number, issue.subject)
		}
		fmt.Println()
	}
	if len(other) > 0 {
		fmt.Println(underline("Unclassified", "-"))
		fmt.Println()
		for _, issue := range other {
			fmt.Printf("* #%d: %s\n", issue.number, issue.subject)
		}
		fmt.Println()
	}
}

func underline(s, c string) string {
	return fmt.Sprintf("%s\n%s", s, strings.Repeat(c, len(s)))
}

func runError(cmd string, args ...string) ([]byte, error) {
	ecmd := exec.Command(cmd, args...)
	bs, err := ecmd.CombinedOutput()
	if err != nil {
		return nil, err
	}
	return bytes.TrimSpace(bs), nil
}

func githubIssueTitleLabels(n int) (string, []string, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("https://api.github.com/repos/syncthing/syncthing/issues/%d", n), nil)
	if err != nil {
		return "", nil, err
	}

	user, token := os.Getenv("GITHUB_USERNAME"), os.Getenv("GITHUB_TOKEN")
	if user != "" && token != "" {
		req.SetBasicAuth(user, token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()

	bs, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", nil, err
	}

	var res struct {
		Title  string
		Labels []struct {
			Name string
		}
		PR struct {
			URL string
		} `json:"pull_request"`
	}
	err = json.Unmarshal(bs, &res)
	if err != nil {
		return "", nil, err
	}

	if res.PR.URL != "" {
		return "", nil, errors.New("pull request")
	}

	var labels []string
	for _, l := range res.Labels {
		labels = append(labels, l.Name)
	}

	return res.Title, labels, nil
}

func contains(s string, ss []string) bool {
	for _, x := range ss {
		if s == x {
			return true
		}
	}
	return false
}
