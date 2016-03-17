// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// +build ignore

// Generates the list of contributors in gui/index.html based on contents of
// AUTHORS.

package main

import (
	"bytes"
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

type author struct {
	name         string
	emails       []string
	commits      int
	log10commits int
}

func main() {
	authors := getAuthors()
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
}

func getAuthors() []author {
	bs := readAll("AUTHORS")
	lines := strings.Split(string(bs), "\n")
	var authors []author
	nameRe := regexp.MustCompile(`(.+?)\s+<`)
	authorRe := regexp.MustCompile(`<([^>]+)>`)
	for _, line := range lines {
		m := nameRe.FindStringSubmatch(line)
		if len(m) < 2 {
			continue
		}
		name := m[1]

		ms := authorRe.FindAllStringSubmatch(line, -1)
		if len(ms) == 0 {
			continue
		}

		var emails []string
		for i := range ms {
			emails = append(emails, ms[i][1])
		}

		a := author{
			name:   name,
			emails: emails,
		}
		authors = append(authors, a)
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
