// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// +build ignore

package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

var trans = make(map[string]string)
var attrRe = regexp.MustCompile(`\{\{'([^']+)'\s+\|\s+translate\}\}`)

// .desktop entries that can be localestrings
var groupRe = regexp.MustCompile(`^\[Desktop Entry\]$`)
var locRe = regexp.MustCompile(`^(Name|GenericName|Comment|Keywords)=.*\S*.*`)

// exceptions to the untranslated text warning
var noStringRe = regexp.MustCompile(
	`^((\W*\{\{.*?\}\} ?.?\/?.?(bps)?\W*)+(\.stignore)?|[^a-zA-Z]+.?[^a-zA-Z]*|[kMGT]?B|Twitter|JS\W?|DEV|https?://\S+)$`)

// exceptions to the untranslated text warning specific to aboutModalView.html
var aboutRe = regexp.MustCompile(`^([^/]+/[^/]+|(The Go Pro|Font Awesome ).+)$`)

func generalNode(n *html.Node, filename string) {
	translate := false
	if n.Type == html.ElementNode {
		if n.Data == "translate" { // for <translate>Text</translate>
			translate = true
		} else if n.Data == "style" {
			return
		} else {
			for _, a := range n.Attr {
				if a.Key == "translate" {
					translate = true
				} else if a.Key == "id" && (a.Val == "contributor-list" ||
					a.Val == "copyright-notices") {
					// Don't translate a list of names and
					// copyright notices of other projects
					return
				} else {
					if matches := attrRe.FindStringSubmatch(a.Val); len(matches) == 2 {
						translation(matches[1])
					}
					if a.Key == "data-content" &&
						!noStringRe.MatchString(a.Val) {
						log.Println("Untranslated data-content string (" + filename + "):")
						log.Print("\t" + a.Val)
					}
				}
			}
		}
	} else if n.Type == html.TextNode {
		v := strings.TrimSpace(n.Data)
		if len(v) > 1 && !noStringRe.MatchString(v) &&
			!(filename == "aboutModalView.html" && aboutRe.MatchString(v)) &&
			!(filename == "logbar.html" && (v == "warn" || v == "errors")) {
			log.Println("Untranslated text node (" + filename + "):")
			log.Print("\t" + v)
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if translate {
			inTranslate(c, filename)
		} else {
			generalNode(c, filename)
		}
	}
}

func inTranslate(n *html.Node, filename string) {
	if n.Type == html.TextNode {
		translation(n.Data)
	} else {
		log.Println("translate node with non-text child < (" + filename + ")")
		log.Println(n)
	}
	if n.FirstChild != nil {
		log.Println("translate node has children (" + filename + "):")
		log.Println(n.Data)
	}
}

func parseDesktop(filename string) {
	fd, err := os.Open(filename)
	if err != nil {
		log.Fatal(err)
	}
	defer fd.Close()

	bs, err := ioutil.ReadAll(fd)
	if err != nil {
		log.Fatal(err)
	}

	lines := strings.Split(string(bs), "\n")

	in := false

	for _, line := range lines {
		if groupRe.MatchString(line) {
			in = true
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			in = false
			continue
		}

		if !in {
			continue
		}

		if !(locRe.MatchString(line)) {
			continue
		}

		switch value := strings.SplitN(line, "=", 2); value[0] {
		case "Name", "GenericName", "Comment":
			if len(value[1]) == 0 {
				continue
			}
			translation(value[1])
		case "Keywords":
			words := strings.Split(value[1], ";")
			for _, word := range words {
				if len(word) == 0 {
					continue
				}
				translation(word)
			}
		}
	}
}

func translation(v string) {
	v = strings.TrimSpace(v)
	if _, ok := trans[v]; !ok {
		av := strings.Replace(v, "{%", "{{", -1)
		av = strings.Replace(av, "%}", "}}", -1)
		trans[v] = av
	}
}

func walkerFor(basePath string) filepath.WalkFunc {
	return func(name string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if filepath.Ext(name) == ".html" && info.Mode().IsRegular() {
			fd, err := os.Open(name)
			if err != nil {
				log.Fatal(err)
			}
			doc, err := html.Parse(fd)
			if err != nil {
				log.Fatal(err)
			}
			fd.Close()
			generalNode(doc, filepath.Base(name))
		}

		if filepath.Ext(name) == ".desktop" && info.Mode().IsRegular() {
			parseDesktop(filepath.Clean(name))
		}

		return nil
	}
}

func main() {
	fd, err := os.Open(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	err = json.NewDecoder(fd).Decode(&trans)
	if err != nil {
		log.Fatal(err)
	}
	fd.Close()

	var guiDir = os.Args[2]

	filepath.Walk(guiDir, walkerFor(guiDir))

	var desktopDir = guiDir + "../etc/linux-desktop"
	filepath.Walk(desktopDir, walkerFor(desktopDir))

	bs, err := json.MarshalIndent(trans, "", "   ")
	if err != nil {
		log.Fatal(err)
	}
	os.Stdout.Write(bs)
	os.Stdout.WriteString("\n")
}
