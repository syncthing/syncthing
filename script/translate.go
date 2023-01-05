// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build ignore
// +build ignore

package main

import (
	"bufio"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

var trans = make(map[string]string)
var attrRe = regexp.MustCompile(`\{\{\s*'([^']+)'\s+\|\s+translate\s*\}\}`)
var attrReCond = regexp.MustCompile(`\{\{.+\s+\?\s+'([^']+)'\s+:\s+'([^']+)'\s+\|\s+translate\s*\}\}`)

// Find both $translate.instant("…") and $translate.instant("…",…) in JS.
// Consider single quote variants too.
var jsRe = []*regexp.Regexp{
	regexp.MustCompile(`\$translate\.instant\(\s*"(.+?)"(,.*|\s*)\)`),
	regexp.MustCompile(`\$translate\.instant\(\s*'(.+?)'(,.*|\s*)\)`),
}

// exceptions to the untranslated text warning
var noStringRe = regexp.MustCompile(
	`^((\W*\{\{.*?\}\} ?.?\/?.?(bps)?\W*)+(\.stignore)?|[^a-zA-Z]+.?[^a-zA-Z]*|[kMGT]?B|Twitter|JS\W?|DEV|https?://\S+|TechUi)$`)

// exceptions to the untranslated text warning specific to aboutModalView.html
var aboutRe = regexp.MustCompile(`^([^/]+/[^/]+|(The Go Pro|Font Awesome ).+|Build \{\{.+\}\}|Copyright .+ the Syncthing Authors\.)$`)

func generalNode(n *html.Node, filename string) {
	translate := false
	if n.Type == html.ElementNode {
		if n.Data == "translate" { // for <translate>Text</translate>
			translate = true
		} else if n.Data == "style" || n.Data == "noscript" {
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
					for _, matches := range attrRe.FindAllStringSubmatch(a.Val, -1) {
						translation(matches[1])
					}
					for _, matches := range attrReCond.FindAllStringSubmatch(a.Val, -1) {
						translation(matches[1])
						translation(matches[2])
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

		if !info.Mode().IsRegular() {
			return nil
		}
		fd, err := os.Open(name)
		if err != nil {
			log.Fatal(err)
		}
		defer fd.Close()
		switch filepath.Ext(name) {
		case ".html":
			doc, err := html.Parse(fd)
			if err != nil {
				log.Fatal(err)
			}
			generalNode(doc, filepath.Base(name))
		case ".js":
			for s := bufio.NewScanner(fd); s.Scan(); {
				for _, re := range jsRe {
					for _, matches := range re.FindAllStringSubmatch(s.Text(), -1) {
						translation(matches[1])
					}
				}
			}
		}

		return nil
	}
}

func collectThemes(basePath string) {
	files, err := os.ReadDir(basePath)
	if err != nil {
		log.Fatal(err)
	}
	for _, f := range files {
		if f.IsDir() {
			key := "theme-name-" + f.Name()
			if _, ok := trans[key]; !ok {
				name := strings.Title(f.Name())
				trans[key] = name
			}
		}
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
	collectThemes(guiDir)

	bs, err := json.MarshalIndent(trans, "", "    ")
	if err != nil {
		log.Fatal(err)
	}
	os.Stdout.Write(bs)
	os.Stdout.WriteString("\n")
}
