// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build ignore
// +build ignore

// Generates the list of contributors in gui/index.html based on contents of
// AUTHORS.

package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
)

const htmlFile = "gui/default/syncthing/core/aboutModalView.html"

var (
	nicknameRe = regexp.MustCompile(`\(([^\s]*)\)`)
	emailRe    = regexp.MustCompile(`<([^\s]*)>`)
)

const authorsHeader = `# This is the official list of Syncthing authors for copyright purposes.
#
# THIS FILE IS MOSTLY AUTO GENERATED. IF YOU'VE MADE A COMMIT TO THE
# REPOSITORY YOU WILL BE ADDED HERE AUTOMATICALLY WITHOUT THE NEED FOR
# ANY MANUAL ACTION.
#
# That said, you are welcome to correct your name or add a nickname / GitHub
# user name as appropriate. The format is:
#
#    Name Name Name (nickname) <email1@example.com> <email2@example.com>
#
# The in-GUI authors list is periodically automatically updated from the
# contents of this file.
#
`

type author struct {
	name         string
	nickname     string
	emails       []string
	commits      int
	log10commits int
}

func main() {
	// Read authors from the AUTHORS file
	authors := getAuthors()

	// Grab the set of thus known email addresses
	listed := make(stringSet)
	names := make(map[string]int)
	for i, a := range authors {
		names[a.name] = i
		for _, e := range a.emails {
			listed.add(e)
		}
	}

	// Grab the set of all known authors based on the git log, and add any
	// missing ones to the authors list.
	all := allAuthors()
	for email, name := range all {
		if listed.has(email) {
			continue
		}

		if _, ok := names[name]; ok && name != "" {
			// We found a match on name
			authors[names[name]].emails = append(authors[names[name]].emails, email)
			listed.add(email)
			continue
		}

		authors = append(authors, author{
			name:   name,
			emails: []string{email},
		})
		names[name] = len(authors) - 1
		listed.add(email)
	}

	// Write author names in GUI about modal

	getContributions(authors)
	sort.Sort(byContributions(authors))

	var lines []string
	for _, author := range authors {
		lines = append(lines, author.name)
	}
	replacement := strings.Join(lines, ", ")

	authorsRe := regexp.MustCompile(`(?s)id="contributor-list">.*?</div>`)
	bs := readAll(htmlFile)
	bs = authorsRe.ReplaceAll(bs, []byte("id=\"contributor-list\">\n"+replacement+"\n    </div>"))

	if err := ioutil.WriteFile(htmlFile, bs, 0644); err != nil {
		log.Fatal(err)
	}

	// Write AUTHORS file

	sort.Sort(byName(authors))

	out, err := os.Create("AUTHORS")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Fprintf(out, "%s\n", authorsHeader)
	for _, author := range authors {
		fmt.Fprintf(out, "%s", author.name)
		if author.nickname != "" {
			fmt.Fprintf(out, " (%s)", author.nickname)
		}
		for _, email := range author.emails {
			fmt.Fprintf(out, " <%s>", email)
		}
		fmt.Fprintf(out, "\n")
	}
	out.Close()
}

func getAuthors() []author {
	bs := readAll("AUTHORS")
	lines := strings.Split(string(bs), "\n")
	var authors []author

	for _, line := range lines {
		if len(line) == 0 || line[0] == '#' {
			continue
		}

		fields := strings.Fields(line)
		var author author
		for _, field := range fields {
			if m := nicknameRe.FindStringSubmatch(field); len(m) > 1 {
				author.nickname = m[1]
			} else if m := emailRe.FindStringSubmatch(field); len(m) > 1 {
				author.emails = append(author.emails, m[1])
			} else {
				if author.name == "" {
					author.name = field
				} else {
					author.name = author.name + " " + field
				}
			}
		}

		authors = append(authors, author)
	}
	return authors
}

func readAll(path string) []byte {
	fd, err := os.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	defer fd.Close()

	bs, err := ioutil.ReadAll(fd)
	if err != nil {
		log.Fatal(err)
	}

	return bs
}

// Add number of commits per author to the author list.
func getContributions(authors []author) {
	buf := new(bytes.Buffer)
	cmd := exec.Command("git", "log", "--pretty=format:%ae")
	cmd.Stdout = buf
	err := cmd.Run()
	if err != nil {
		log.Fatal(err)
	}

next:
	for _, line := range strings.Split(buf.String(), "\n") {
		for i := range authors {
			for _, email := range authors[i].emails {
				if email == line {
					authors[i].commits++
					continue next
				}
			}
		}
	}

	for i := range authors {
		authors[i].log10commits = int(math.Log10(float64(authors[i].commits + 1)))
	}
}

// list of commits that we don't include in our author file; because they
// are legacy things that don't affect code, are committed with incorrect
// address, or for other reasons.
var excludeCommits = stringSetFromStrings([]string{
	"a9339d0627fff439879d157c75077f02c9fac61b",
	"254c63763a3ad42fd82259f1767db526cff94a14",
	"32a76901a91ff0f663db6f0830e0aedec946e4d0",
	"bc7639b0ffcea52b2197efb1c0bb68b338d1c915",
	"9bdcadf6345aba3a939e9e58d85b89dbe9d44bc9",
	"b933e9666abdfcd22919dd458c930d944e1e1b7f",
	"b84d960a81c1282a79e2b9477558de4f1af6faae",
})

// allAuthors returns the set of authors in the git commit log, except those
// in excluded commits.
func allAuthors() map[string]string {
	args := append([]string{"log", "--format=%H %ae %an"})
	cmd := exec.Command("git", args...)
	bs, err := cmd.Output()
	if err != nil {
		log.Fatal("git:", err)
	}

	names := make(map[string]string)
	for _, line := range bytes.Split(bs, []byte{'\n'}) {
		fields := strings.SplitN(string(line), " ", 3)
		if len(fields) != 3 {
			continue
		}
		hash, email, name := fields[0], fields[1], fields[2]

		if excludeCommits.has(hash) {
			continue
		}

		if names[email] == "" {
			names[email] = name
		}
	}

	return names
}

type byContributions []author

func (l byContributions) Len() int { return len(l) }

// Sort first by log10(commits), then by name. This means that we first get
// an alphabetic list of people with >= 1000 commits, then a list of people
// with >= 100 commits, and so on.
func (l byContributions) Less(a, b int) bool {
	if l[a].log10commits != l[b].log10commits {
		return l[a].log10commits > l[b].log10commits
	}
	return l[a].name < l[b].name
}

func (l byContributions) Swap(a, b int) { l[a], l[b] = l[b], l[a] }

type byName []author

func (l byName) Len() int { return len(l) }

func (l byName) Less(a, b int) bool {
	aname := strings.ToLower(l[a].name)
	bname := strings.ToLower(l[b].name)
	return aname < bname
}

func (l byName) Swap(a, b int) { l[a], l[b] = l[b], l[a] }

// A simple string set type

type stringSet map[string]struct{}

func stringSetFromStrings(ss []string) stringSet {
	s := make(stringSet)
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
	diff := make(stringSet)
	for e := range s {
		if !other.has(e) {
			diff.add(e)
		}
	}
	return diff
}
