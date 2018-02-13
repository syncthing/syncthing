// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// Checks for authors that are not mentioned in AUTHORS
package meta

import (
	"bytes"
	"io/ioutil"
	"os/exec"
	"regexp"
	"strings"
	"testing"
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
	"342036408e65bd25bb6afbcc705e2e2c013bb01f",
	"e37cefdbee1c1cd95ad095b5da6d1252723f103b",
	"bcc5d7c00f52552303b463d43a636f27b7f7e19b",
	"bc7639b0ffcea52b2197efb1c0bb68b338d1c915",
})

func TestCheckAuthors(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test")
	}

	actual, hashes := actualAuthorEmails(t, ".", "../cmd/", "../lib/", "../gui/", "../test/", "../script/")
	listed := listedAuthorEmails(t)
	missing := actual.except(listed)
	for author := range missing {
		t.Logf("Missing author: %s", author)
		for _, hash := range hashes[author] {
			t.Logf("  in hash: %s", hash)
		}
	}
	if len(missing) > 0 {
		t.Errorf("Missing %d author(s)", len(missing))
	}
}

// actualAuthorEmails returns the set of author emails found in the actual git
// commit log, except those in excluded commits.
func actualAuthorEmails(t *testing.T, paths ...string) (stringSet, map[string][]string) {
	args := append([]string{"log", "--format=%H %ae"}, paths...)
	cmd := exec.Command("git", args...)
	bs, err := cmd.Output()
	if err != nil {
		t.Fatal("authorEmails:", err)
	}

	hashes := make(map[string][]string)
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

		if strings.Contains(strings.ToLower(body(t, hash)), "skip-check: authors") {
			continue
		}

		authors.add(author)
		hashes[author] = append(hashes[author], hash)
	}

	return authors, hashes
}

// listedAuthorEmails returns the set of author emails mentioned in AUTHORS
func listedAuthorEmails(t *testing.T) stringSet {
	bs, err := ioutil.ReadFile("../AUTHORS")
	if err != nil {
		t.Fatal("listedAuthorEmails:", err)
	}

	emailRe := regexp.MustCompile(`<([^>]+)>`)
	matches := emailRe.FindAllStringSubmatch(string(bs), -1)

	authors := newStringSet()
	for _, match := range matches {
		authors.add(match[1])
	}
	return authors
}

func body(t *testing.T, hash string) string {
	cmd := exec.Command("git", "show", "--pretty=format:%b", "-s", hash)
	bs, err := cmd.Output()
	if err != nil {
		t.Fatal("body:", err)
	}
	return string(bs)
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
