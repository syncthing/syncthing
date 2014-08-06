// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// +build ignore

package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
)

type stat struct {
	Translated   int `json:"translated_entities"`
	Untranslated int `json:"untranslated_entities"`
}

type translation struct {
	Content string
}

func main() {
	log.SetFlags(log.Lshortfile)

	if u, p := userPass(); u == "" || p == "" {
		log.Fatal("Need environment variables TRANSIFEX_USER and TRANSIFEX_PASS")
	}

	curValidLangs := map[string]bool{}
	for _, lang := range loadValidLangs() {
		curValidLangs[lang] = true
	}
	log.Println(curValidLangs)

	resp := req("https://www.transifex.com/api/2/project/syncthing/resource/gui/stats")

	var stats map[string]stat
	err := json.NewDecoder(resp.Body).Decode(&stats)
	if err != nil {
		log.Fatal(err)
	}
	resp.Body.Close()

	var langs []string
	for code, stat := range stats {
		shortCode := code[:2]
		if !curValidLangs[shortCode] {
			if pct := 100 * stat.Translated / (stat.Translated + stat.Untranslated); pct < 95 {
				log.Printf("Skipping language %q (too low completion ratio %d%%)", shortCode, pct)
				os.Remove("lang-" + shortCode + ".json")
				continue
			}
		}

		langs = append(langs, shortCode)
		if shortCode == "en" {
			continue
		}

		log.Printf("Updating language %q", shortCode)

		resp := req("https://www.transifex.com/api/2/project/syncthing/resource/gui/translation/" + code)
		var t translation
		err := json.NewDecoder(resp.Body).Decode(&t)
		if err != nil {
			log.Fatal(err)
		}
		resp.Body.Close()

		fd, err := os.Create("lang-" + shortCode + ".json")
		if err != nil {
			log.Fatal(err)
		}
		fd.WriteString(t.Content)
		fd.Close()
	}

	saveValidLangs(langs)
}

func saveValidLangs(langs []string) {
	sort.Strings(langs)
	fd, err := os.Create("valid-langs.js")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Fprint(fd, "var validLangs = ")
	json.NewEncoder(fd).Encode(langs)
	fd.Close()
}

func userPass() (string, string) {
	user := os.Getenv("TRANSIFEX_USER")
	pass := os.Getenv("TRANSIFEX_PASS")
	return user, pass
}

func req(url string) *http.Response {
	user, pass := userPass()

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Fatal(err)
	}
	req.SetBasicAuth(user, pass)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	return resp
}

func loadValidLangs() []string {
	fd, err := os.Open("valid-langs.js")
	if err != nil {
		log.Fatal(err)
	}
	defer fd.Close()
	bs, err := ioutil.ReadAll(fd)
	if err != nil {
		log.Fatal(err)
	}

	var langs []string
	exp := regexp.MustCompile(`\[([a-z",]+)\]`)
	if matches := exp.FindSubmatch(bs); len(matches) == 2 {
		langs = strings.Split(string(matches[1]), ",")
		for i := range langs {
			// Remove quotes
			langs[i] = langs[i][1 : len(langs[i])-1]
		}
	}

	return langs
}
