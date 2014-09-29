// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

// +build ignore

package main

import (
	"encoding/json"
	"log"
	"os"
	"regexp"
	"strings"

	"code.google.com/p/go.net/html"
)

var trans = make(map[string]string)
var attrRe = regexp.MustCompile(`\{\{'([^']+)'\s+\|\s+translate\}\}`)

func generalNode(n *html.Node) {
	translate := false
	if n.Type == html.ElementNode {
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
	} else if n.Type == html.TextNode {
		v := strings.TrimSpace(n.Data)
		if len(v) > 1 && !(strings.HasPrefix(v, "{{") && strings.HasSuffix(v, "}}")) {
			log.Println("Untranslated text node:")
			log.Print("\t" + v)
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if translate {
			inTranslate(c)
		} else {
			generalNode(c)
		}
	}
}

func inTranslate(n *html.Node) {
	if n.Type == html.TextNode {
		translation(n.Data)
	} else {
		log.Println("translate node with non-text child <")
		log.Println(n)
	}
	if n.FirstChild != nil {
		log.Println("translate node has children:")
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

	fd, err = os.Open(os.Args[2])
	if err != nil {
		log.Fatal(err)
	}
	doc, err := html.Parse(fd)
	if err != nil {
		log.Fatal(err)
	}
	fd.Close()

	generalNode(doc)
	bs, err := json.MarshalIndent(trans, "", "   ")
	if err != nil {
		log.Fatal(err)
	}
	os.Stdout.Write(bs)
	os.Stdout.WriteString("\n")
}
