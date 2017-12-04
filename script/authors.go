// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

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
# The format is:
#
#    Name Name Name (nickname) <email1@example.com> <email2@example.com>
#
# After changing this list, run "go run script/authors.go" to sort and update
# the GUI HTML.
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
	authors := getAuthors()

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
