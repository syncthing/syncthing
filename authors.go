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
	"io/ioutil"
	"log"
	"os"
	"regexp"
	"sort"
	"strings"
)

const htmlFile = "gui/scripts/syncthing/core/views/directives/aboutModalView.html"

func main() {
	bs := readAll("AUTHORS")
	lines := strings.Split(string(bs), "\n")
	nameRe := regexp.MustCompile(`(.+?)\s+<`)
	authors := make([]string, 0, len(lines))
	for _, line := range lines {
		if m := nameRe.FindStringSubmatch(line); len(m) == 2 {
			authors = append(authors, "        <li class=\"auto-generated\">"+m[1]+"</li>")
		}
	}
	sort.Strings(authors)
	replacement := strings.Join(authors, "\n")

	authorsRe := regexp.MustCompile(`(?s)id="contributor-list">.*?</ul>`)
	bs = readAll(htmlFile)
	bs = authorsRe.ReplaceAll(bs, []byte("id=\"contributor-list\">\n"+replacement+"\n      </ul>"))

	if err := ioutil.WriteFile(htmlFile, bs, 0644); err != nil {
		log.Fatal(err)
	}
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
