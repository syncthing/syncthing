// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// +build ignore

// Checks for authors that are not mentioned in AUTHORS
package main

import (
	"bytes"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// list of commits that we don't include in our checks; because they are
// legacy things that don't check code, are committed with incorrect address,
// or for other reasons.
var excludeCommits = stringSetFromStrings([]string{
	"63bd0136fb40a91efaa279cb4b4159d82e8e6904",
	"4e2feb6fbc791bb8a2daf0ab8efb10775d66343e",
	"f2459ef3319b2f060dbcdacd0c35a1788a94b8bd",
	"b61f418bf2d1f7d5a9d7088a20a2a448e5e66801",
	"a9339d0627fff439879d157c75077f02c9fac61b",
	"254c63763a3ad42fd82259f1767db526cff94a14",
	"4b76ec40c07078beaa2c5e250ed7d9bd6276a718",
	"32a76901a91ff0f663db6f0830e0aedec946e4d0",
	"3626003f680bad3e63677982576d3a05421e88e9",
})

func init() {
	log.SetOutput(os.Stdout)
	log.SetFlags(0)
}

func main() {
	actual := actualAuthorEmails()
	listed := listedAuthorEmails()
	missing := actual.except(listed)
	if len(missing) > 0 {
		log.Println("Missing authors:")
		for author := range missing {
			log.Println(" ", author)
		}
		os.Exit(1)
	}
}

// actualAuthorEmails returns the set of author emails found in the actual git
// commit log, except those in excluded commits.
func actualAuthorEmails() stringSet {
	cmd := exec.Command("git", "log", "--format=%H %ae")
	bs, err := cmd.Output()
	if err != nil {
		log.Fatal("authorEmails:", err)
	}

	authors := newStringSet()
	for _, line := range bytes.Split(bs, []byte{'\n'}) {
		fields := strings.Fields(string(line))
		if len(fields) != 2 {
			continue
		}

		hash, author := fields[0], fields[1]
		if excludeCommits.has(hash) {
			continue
		}

		authors.add(author)
	}

	return authors
}

// listedAuthorEmails returns the set of author emails mentioned in AUTHORS
func listedAuthorEmails() stringSet {
	bs, err := ioutil.ReadFile("AUTHORS")
	if err != nil {
		log.Fatal("listedAuthorEmails:", err)
	}

	emailRe := regexp.MustCompile(`<([^>]+)>`)
	matches := emailRe.FindAllStringSubmatch(string(bs), -1)

	authors := newStringSet()
	for _, match := range matches {
		authors.add(match[1])
	}
	return authors
}

// A simple string set type

type stringSet map[string]struct{}

func newStringSet() stringSet {
	return make(stringSet)
}

func stringSetFromStrings(ss []string) stringSet {
	s := newStringSet()
	for _, e := range ss {
		s.add(e)
	}
	return s
}

func (s stringSet) add(e string) {
	s[e] = struct{}{}
}

func (s stringSet) has(e string) bool {
	_, ok := s[e]
	return ok
}

func (s stringSet) except(other stringSet) stringSet {
	diff := newStringSet()
	for e := range s {
		if !other.has(e) {
			diff.add(e)
		}
	}
	return diff
}
