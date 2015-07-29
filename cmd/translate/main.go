// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// +build ignore

package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

var trans = make(map[string]string)
var attrRe = regexp.MustCompile(`\{\{'([^']+)'\s+\|\s+translate\}\}`)

func generalNode(n *html.Node, filename string) {
	translate := false
	if n.Type == html.ElementNode {
		if n.Data == "translate" { // for <translate>Text</translate>
			translate = true
		} else {
			for _, a := range n.Attr {
				if a.Key == "translate" {
					translate = true
					break
				} else {
					if matches := attrRe.FindStringSubmatch(a.Val); len(matches) == 2 {
						translation(matches[1])
					}
				}
			}
		}
	} else if n.Type == html.TextNode {
		v := strings.TrimSpace(n.Data)
		if len(v) > 1 && !(strings.HasPrefix(v, "{{") && strings.HasSuffix(v, "}}")) {
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

	bs, err := json.MarshalIndent(trans, "", "   ")
	if err != nil {
		log.Fatal(err)
	}
	os.Stdout.Write(bs)
	os.Stdout.WriteString("\n")
}
