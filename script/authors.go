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
	"cmp"
	"fmt"
	"log"
	"math"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"
)

const htmlFile = "gui/default/syncthing/core/aboutModalView.html"

var (
	nicknameRe        = regexp.MustCompile(`\(([^\s]*)\)`)
	emailRe           = regexp.MustCompile(`<([^\s]*)>`)
	authorBotsRegexps = []string{
		`\[bot\]`,
		`Syncthing.*Automation`,
	}
)

var authorBotsRe = regexp.MustCompile(strings.Join(authorBotsRegexps, "|"))

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
	authorSet := getAuthors()

	// Grab the set of all known authors based on the git log, and add any
	// missing ones to the authors list.
	addAuthors(authorSet)

	authors := authorSet.filteredAuthors()

	// Write authors to the about dialog

	slices.SortFunc(authors, func(a, b author) int {
		return cmp.Or(
			-cmp.Compare(a.log10commits, b.log10commits),
			cmp.Compare(strings.ToLower(a.name), strings.ToLower(b.name)))
	})

	var lines []string
	for _, author := range authors {
		lines = append(lines, author.name)
	}
	replacement := strings.Join(lines, ", ")

	authorsRe := regexp.MustCompile(`(?s)id="contributor-list">.*?</div>`)
	bs, err := os.ReadFile(htmlFile)
	if err != nil {
		log.Fatal(err)
	}
	bs = authorsRe.ReplaceAll(bs, []byte("id=\"contributor-list\">\n"+replacement+"\n          </div>"))

	if err := os.WriteFile(htmlFile, bs, 0o644); err != nil {
		log.Fatal(err)
	}

	// Write AUTHORS file

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
		fmt.Fprint(out, "\n")
	}
	out.Close()
}

func getAuthors() *authorSet {
	bs, err := os.ReadFile("AUTHORS")
	if err != nil {
		log.Fatal(err)
	}

	lines := strings.Split(string(bs), "\n")
	authors := &authorSet{
		emails:  make(map[string]int),
		commits: make(map[string]stringSet),
	}
	for _, line := range lines {
		if len(line) == 0 || line[0] == '#' {
			continue
		}

		fields := strings.Fields(line)
		var author author
		for _, field := range fields {
			if field == "#" {
				break
			} else if m := nicknameRe.FindStringSubmatch(field); len(m) > 1 {
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

		authors.add(author)
	}
	return authors
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
	"4dfb9d7c83ed172f12ae19408517961f4a49beeb",
})

func addAuthors(authors *authorSet) {
	// All existing source-tracked files
	bs, err := exec.Command("git", "ls-tree", "-r", "HEAD", "--name-only").CombinedOutput()
	if err != nil {
		fmt.Println(string(bs))
		log.Fatal("git ls-tree:", err)
	}
	files := strings.Split(string(bs), "\n")
	files = slices.DeleteFunc(files, func(s string) bool {
		return !(strings.HasPrefix(s, "assets/") ||
			strings.HasPrefix(s, "cmd/") ||
			strings.HasPrefix(s, "etc/") ||
			strings.HasPrefix(s, "gui/") ||
			strings.HasPrefix(s, "internal/") ||
			strings.HasPrefix(s, "lib/") ||
			strings.HasPrefix(s, "proto/") ||
			strings.HasPrefix(s, "script/") ||
			strings.HasPrefix(s, "test/") ||
			strings.HasPrefix(s, "Dockerfile") ||
			s == "build.go")
	})

	coAuthoredPrefix := "Co-authored-by: "
	for _, file := range files {
		// All commits affecting those files, following any renames to their
		// origin. Format is hash, email, name, newline, body. The body is
		// indented with one space, to differentiate from the hash lines.
		args := []string{"log", "--format=%H %ae %an%n%w(,1,1)%b", "--follow", "--", file}
		bs, err = exec.Command("git", args...).CombinedOutput()
		if err != nil {
			fmt.Println(string(bs))
			log.Fatal("git log:", err)
		}

		skipCommit := false
		var hash, email, name string
		for _, line := range bytes.Split(bs, []byte{'\n'}) {
			if len(line) == 0 {
				continue
			}

			switch line[0] {
			case ' ':
				// Look for Co-authored-by: lines in the commit body.
				if skipCommit {
					continue
				}

				line = line[1:]
				if bytes.HasPrefix(line, []byte(coAuthoredPrefix)) {
					// Co-authored-by: Name Name <email@example.com>
					line = line[len(coAuthoredPrefix):]
					if name, email, ok := strings.Cut(string(line), "<"); ok {
						name = strings.TrimSpace(name)
						email = strings.Trim(strings.TrimSpace(email), "<>")
						if email == "@" {
							// GitHub special for users who hide their email.
							continue
						}
						authors.setName(email, name)
						authors.addCommit(email, hash)
					}
				}

			default: // hash email name
				fields := strings.SplitN(string(line), " ", 3)
				if len(fields) != 3 {
					continue
				}
				hash, email, name = fields[0], fields[1], fields[2]

				if excludeCommits.has(hash) {
					skipCommit = true
					continue
				}
				skipCommit = false

				authors.setName(email, name)
				authors.addCommit(email, hash)
			}
		}
	}
}

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

// A set of authors

type authorSet struct {
	authors []author
	emails  map[string]int       // email to author index
	commits map[string]stringSet // email to commit hashes
}

func (a *authorSet) add(author author) {
	for _, e := range author.emails {
		if idx, ok := a.emails[e]; ok {
			emails := append(author.emails, a.authors[idx].emails...)
			slices.Sort(emails)
			emails = slices.Compact(emails)
			a.authors[idx].name = author.name
			a.authors[idx].emails = emails

			for _, e := range emails {
				a.emails[e] = idx
			}
			return
		}
	}

	for _, e := range author.emails {
		a.emails[e] = len(a.authors)
	}
	a.authors = append(a.authors, author)
}

func (a *authorSet) setName(email, name string) {
	idx, ok := a.emails[email]
	if !ok {
		a.emails[email] = len(a.authors)
		a.authors = append(a.authors, author{name: name, emails: []string{email}})
	} else if a.authors[idx].name == "" {
		a.authors[idx].name = name
	}
}

func (a *authorSet) addCommit(email, hash string) {
	ss, ok := a.commits[email]
	if !ok {
		ss = make(stringSet)
		a.commits[email] = ss
	}
	ss.add(hash)
}

func (a *authorSet) filteredAuthors() []author {
	authors := make([]author, len(a.authors))
	copy(authors, a.authors)
	for i, author := range authors {
		for _, e := range author.emails {
			authors[i].commits += len(a.commits[e])
		}
	}
	authors = slices.DeleteFunc(authors, func(a author) bool {
		return a.commits == 0 || authorBotsRe.MatchString(a.name)
	})
	for i := range authors {
		authors[i].log10commits = int(math.Log10(float64(authors[i].commits)))
	}
	return authors
}
